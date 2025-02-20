package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxRetries         = 5
	jupiterAPIEndpoint = "https://api.jup.ag/price/v2"
	batchSize          = 100                 // Jupiter API批量处理大小
	maxPriceUSD        = 1_000_000_000_000.0 // 最大价格阈值
	minPriceUSD        = 0.000000001         // 最小价格阈值
)

// PriceSource 价格数据源
type PriceSource int

const (
	PriceSourceJupiter PriceSource = iota
)

// TokenPrice 代币价格信息
type TokenPrice struct {
	Price           float64
	Source          PriceSource
	Timestamp       time.Time
	ConfidenceLevel string // 价格可信度
}

// TokenDepth 代币深度信息
type TokenDepth struct {
	BuyDepth  float64 // 买入深度
	SellDepth float64 // 卖出深度
}

// JupiterPriceService Jupiter价格服务
type JupiterPriceService struct {
	client *http.Client
}

func NewJupiterPriceService() *JupiterPriceService {
	return &JupiterPriceService{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// 添加格式化函数
func formatPrice(price float64) string {
	return fmt.Sprintf("$%.0f", price)
}

// GetTokenPrices 批量获取代币价格
func (s *JupiterPriceService) GetTokenPrices(ctx context.Context, mintAddrs []string) (map[string]*TokenPrice, error) {
	if len(mintAddrs) == 0 {
		return make(map[string]*TokenPrice), nil
	}

	prices := make(map[string]*TokenPrice)

	// 按批次处理mint地址
	for i := 0; i < len(mintAddrs); i += batchSize {
		select {
		case <-ctx.Done():
			return prices, ctx.Err()
		default:
			end := i + batchSize
			if end > len(mintAddrs) {
				end = len(mintAddrs)
			}

			batch := mintAddrs[i:end]
			log.Printf("处理Jupiter价格批次 %d-%d，共 %d 个代币", i+1, end, len(batch))

			// 构建请求URL
			url := fmt.Sprintf("%s?ids=%s&vsToken=EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v&showExtraInfo=true",
				jupiterAPIEndpoint, strings.Join(batch, ","))

			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				log.Printf("创建请求失败: %v", err)
				continue
			}

			var lastErr error
			for retry := 0; retry < maxRetries; retry++ {
				if retry > 0 {
					backoff := time.Duration(2<<uint(retry-1)) * time.Second
					log.Printf("重试获取价格 (第 %d 次)，等待 %v...", retry+1, backoff)
					time.Sleep(backoff)
				}

				resp, err := s.client.Do(req)
				if err != nil {
					lastErr = fmt.Errorf("请求失败: %v", err)
					continue
				}

				var result struct {
					Data map[string]struct {
						Price     string `json:"price"`
						ExtraInfo struct {
							ConfidenceLevel string `json:"confidenceLevel"`
						} `json:"extraInfo"`
					} `json:"data"`
				}

				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					resp.Body.Close()
					lastErr = fmt.Errorf("解析响应失败: %v", err)
					continue
				}
				resp.Body.Close()

				for mintAddr, data := range result.Data {
					price, err := strconv.ParseFloat(data.Price, 64)
					if err != nil {
						log.Printf("解析价格失败: %v", err)
						continue
					}

					// 验证价格是否在合理范围内
					if price < minPriceUSD || price > maxPriceUSD {
						log.Printf("mint地址 %s: 价格 %f 超出合理范围", mintAddr, price)
						continue
					}

					prices[mintAddr] = &TokenPrice{
						Price:           price,
						Source:          PriceSourceJupiter,
						Timestamp:       time.Now(),
						ConfidenceLevel: data.ExtraInfo.ConfidenceLevel,
					}
					log.Printf("获取到代币 %s 的价格: %s (可信度: %s)",
						mintAddr, formatPrice(price), data.ExtraInfo.ConfidenceLevel)
				}

				// 如果成功获取了数据，跳出重试循环
				if len(result.Data) > 0 {
					break
				}
			}

			if lastErr != nil {
				log.Printf("批次处理失败: %v", lastErr)
			}

			// 添加短暂延迟避免请求过快
			time.Sleep(100 * time.Millisecond)
		}
	}

	log.Printf("成功从Jupiter获取 %d/%d 个代币的价格信息",
		len(prices), len(mintAddrs))
	return prices, nil
}

func UpdateTokenPrices(tokens map[string][]*TokenData, monitor *TokenMonitor) ([]*TokenData, error) {
	log.Println("\n开始更新所有代币价格...")

	// 获取上一次的价值数据（如果monitor存在）
	var lastTokenValues map[string]float64
	var lastUpdateTime time.Time
	if monitor != nil {
		lastTokenValues = make(map[string]float64)
		for _, token := range monitor.tokens {
			lastTokenValues[token.MintAddr] = token.Value
		}
		lastUpdateTime = monitor.lastUpdateTime
	}

	// 收集所有唯一的mint地址
	mintMap := make(map[string]*TokenData)
	validTokens := make([]*TokenData, 0)
	for _, walletTokens := range tokens {
		for _, token := range walletTokens {
			if existing, ok := mintMap[token.MintAddr]; ok {
				// 如果mint已存在，累加数量
				existing.Amount += token.Amount
			} else {
				// 新的mint，复制token数据
				mintMap[token.MintAddr] = &TokenData{
					MintAddr: token.MintAddr,
					Amount:   token.Amount,
					Decimals: token.Decimals,
					Symbol:   token.Symbol,
					Name:     token.Name,
				}
			}
		}
	}

	// 获取所有mint地址
	mintAddrs := make([]string, 0, len(mintMap))
	for mintAddr := range mintMap {
		mintAddrs = append(mintAddrs, mintAddr)
	}

	// 从Jupiter获取价格
	jupiterService := NewJupiterPriceService()
	jupiterPrices, err := jupiterService.GetTokenPrices(context.Background(), mintAddrs)
	if err != nil {
		log.Printf("从Jupiter获取价格失败: %v", err)
	}

	var totalValue float64
	var updatedCount int
	currentTime := time.Now()

	// 处理每个mint的代币
	for mintAddr, token := range mintMap {
		log.Printf("\n代币计算详情: %s (%s)", token.Symbol, token.Name)
		log.Printf("1. 基本信息:")
		log.Printf("   - Mint地址: %s", mintAddr)
		log.Printf("   - 原始数量: %.8f", token.Amount)
		log.Printf("   - 小数位数: %d", token.Decimals)

		if price, ok := jupiterPrices[mintAddr]; ok {
			if price.Price <= 0 || price.ConfidenceLevel == "low" {
				log.Printf("2. Jupiter价格: 无效 (价格: %.8f, 可信度: %s)",
					price.Price, price.ConfidenceLevel)
				continue
			}

			log.Printf("2. Jupiter价格数据:")
			log.Printf("   - 当前价格: $%.8f", price.Price)
			log.Printf("   - 可信度: %s", price.ConfidenceLevel)

			token.Price = price.Price
			token.Value = token.Amount * price.Price
			token.ConfidenceLevel = price.ConfidenceLevel

			// 计算变化率
			if lastValue, ok := lastTokenValues[mintAddr]; ok && !lastUpdateTime.IsZero() {
				timeDiff := currentTime.Sub(lastUpdateTime).Seconds()
				if timeDiff > 0 {
					valueChange := ((token.Value - lastValue) / lastValue) * 100
					token.Change = valueChange / timeDiff
				}
			}

			log.Printf("3. 价值计算:")
			log.Printf("   - 计算公式: %.8f * $%.8f", token.Amount, price.Price)
			log.Printf("   - 计算结果: %s", formatPrice(token.Value))
			log.Printf("   - 变化率: %.2f%%/s", token.Change)

			validTokens = append(validTokens, token)
			totalValue += token.Value
			updatedCount++
		} else {
			log.Printf("2. Jupiter价格: 未找到")
		}
	}

	// 按价值排序
	sort.Slice(validTokens, func(i, j int) bool {
		return validTokens[i].Value > validTokens[j].Value
	})

	// 只保留前50个
	if len(validTokens) > 50 {
		validTokens = validTokens[:50]
	}

	log.Printf("\nJupiter更新汇总:")
	log.Printf("- 成功: %d个", updatedCount)
	log.Printf("- 总代币数: %d个", len(mintMap))
	log.Printf("- 当前总价值: %s", formatPrice(totalValue))
	log.Println("----------------------------------------")

	// 更新监控器的代币列表
	if monitor != nil {
		monitor.UpdateTokens(validTokens)
		monitor.lastUpdateTime = currentTime
	}

	return validTokens, nil
}

// FilterTopTokensByValue 筛选价值最高的代币
func FilterTopTokensByValue(tokens []*TokenData, limit int) []*TokenData {
	// 首先过滤掉无效的代币
	validTokens := make([]*TokenData, 0)
	for _, token := range tokens {
		if token.Price > 0 && token.Value > 0 {
			validTokens = append(validTokens, token)
		}
	}

	// 按价值排序
	sort.Slice(validTokens, func(i, j int) bool {
		return validTokens[i].Value > validTokens[j].Value
	})

	if len(validTokens) > limit {
		return validTokens[:limit]
	}
	return validTokens
}
