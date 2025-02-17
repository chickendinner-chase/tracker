package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
	PriceSourceCMC PriceSource = iota
	PriceSourceJupiter
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

// CMCPriceService CoinMarketCap价格服务实现
type CMCPriceService struct {
	client    *http.Client
	baseURL   string
	apiKey    string
	solanaMap map[string]string
}

func NewCMCPriceService() (*CMCPriceService, error) {
	apiKey := os.Getenv("COINMARKETCAP_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("缺少 COINMARKETCAP_API_KEY 配置")
	}

	return &CMCPriceService{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:   "https://pro-api.coinmarketcap.com/v1",
		apiKey:    apiKey,
		solanaMap: make(map[string]string),
	}, nil
}

// GetTokenPrices 从CMC获取价格
func (s *CMCPriceService) GetTokenPrices(ctx context.Context, mintAddrs []string) (map[string]float64, error) {
	if len(mintAddrs) == 0 {
		return make(map[string]float64), nil
	}

	log.Printf("开始从CMC获取 %d 个代币的价格信息...", len(mintAddrs))

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		if err := s.updateSolanaMapping(); err != nil {
			return nil, fmt.Errorf("更新Solana映射失败: %v", err)
		}
	}

	prices := make(map[string]float64)
	var symbols []string
	mintToSymbol := make(map[string]string)

	// 处理mint地址
	for _, mintAddr := range mintAddrs {
		mintAddrLower := strings.ToLower(mintAddr)
		if symbol, ok := s.solanaMap[mintAddrLower]; ok {
			symbols = append(symbols, symbol)
			mintToSymbol[mintAddr] = symbol
			log.Printf("找到mint地址 %s 的符号: %s", mintAddr, symbol)
		} else {
			log.Printf("mint地址 %s 未在CMC上市", mintAddr)
		}
	}

	if len(symbols) == 0 {
		log.Printf("警告: 没有找到任何mint地址的映射")
		return prices, nil
	}

	log.Printf("找到 %d/%d 个mint地址的映射", len(symbols), len(mintAddrs))

	const maxSymbolsPerRequest = 50
	for i := 0; i < len(symbols); i += maxSymbolsPerRequest {
		select {
		case <-ctx.Done():
			return prices, ctx.Err()
		default:
			end := i + maxSymbolsPerRequest
			if end > len(symbols) {
				end = len(symbols)
			}
			batch := symbols[i:end]
			log.Printf("处理批次 %d-%d，共 %d 个代币", i+1, end, len(batch))

			batchPrices, err := s.getBatchPrices(batch)
			if err != nil {
				log.Printf("获取批次价格失败 [%d-%d]: %v", i, end, err)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			// 将符号的价格映射回mint地址
			for mintAddr, symbol := range mintToSymbol {
				if price, ok := batchPrices[symbol]; ok {
					prices[mintAddr] = price
					log.Printf("更新mint地址 %s (符号: %s) 的价格: %s",
						mintAddr, symbol, formatPrice(price))
				}
			}

			if end < len(symbols) {
				time.Sleep(200 * time.Millisecond)
			}
		}
	}

	log.Printf("成功从CMC获取 %d/%d 个代币的价格信息",
		len(prices), len(mintAddrs))
	return prices, nil
}

func (s *CMCPriceService) updateSolanaMapping() error {
	url := fmt.Sprintf("%s/cryptocurrency/quotes/latest", s.baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	q := req.URL.Query()
	q.Add("aux", "platform,contract_address")
	q.Add("skip_invalid", "true")
	q.Add("tag", "solana-ecosystem") // 添加 Solana 生态系统标签
	req.URL.RawQuery = q.Encode()

	req.Header.Set("X-CMC_PRO_API_KEY", s.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	log.Printf("API响应状态码: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		log.Printf("API响应内容: %s", string(body))
		return fmt.Errorf("API返回非200状态码: %d", resp.StatusCode)
	}

	var result struct {
		Status struct {
			ErrorCode    int    `json:"error_code"`
			ErrorMessage string `json:"error_message"`
		} `json:"status"`
		Data map[string]struct {
			Symbol   string `json:"symbol"`
			Platform struct {
				TokenAddress string `json:"token_address"`
			} `json:"platform"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %v, body: %s", err, string(body))
	}

	if result.Status.ErrorCode != 0 {
		return fmt.Errorf("API错误: %d - %s",
			result.Status.ErrorCode,
			result.Status.ErrorMessage)
	}

	oldCount := len(s.solanaMap)
	for _, data := range result.Data {
		if data.Platform.TokenAddress != "" {
			addr := strings.ToLower(data.Platform.TokenAddress)
			s.solanaMap[addr] = data.Symbol
			log.Printf("添加Solana代币映射: %s -> %s", addr, data.Symbol)
		}
	}

	log.Printf("更新Solana映射完成: 原有 %d 个映射，现有 %d 个映射",
		oldCount, len(s.solanaMap))
	return nil
}

func (s *CMCPriceService) getBatchPrices(symbols []string) (map[string]float64, error) {
	if len(symbols) == 0 {
		return nil, fmt.Errorf("没有需要查询的代币符号")
	}

	log.Printf("开始获取 %d 个代币的价格...", len(symbols))
	url := fmt.Sprintf("%s/cryptocurrency/quotes/latest", s.baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	q := req.URL.Query()
	q.Add("symbol", strings.Join(symbols, ","))
	q.Add("convert", "USD")
	q.Add("aux", "price") // 只获取价格信息
	req.URL.RawQuery = q.Encode()

	req.Header.Set("X-CMC_PRO_API_KEY", s.apiKey)
	req.Header.Set("Accept", "application/json")

	var lastErr error
	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			backoff := time.Duration(5*(2<<uint(retry-1))) * time.Second
			log.Printf("重试获取价格 (第 %d 次)，等待 %v...", retry+1, backoff)
			time.Sleep(backoff)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("请求失败: %v", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("读取响应失败: %v", err)
			continue
		}

		var result struct {
			Status struct {
				ErrorCode    int    `json:"error_code"`
				ErrorMessage string `json:"error_message"`
				Credit_count int    `json:"credit_count"`
			} `json:"status"`
			Data map[string]struct {
				Symbol string `json:"symbol"`
				Quote  struct {
					USD struct {
						Price float64 `json:"price"`
					} `json:"USD"`
				} `json:"quote"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			lastErr = fmt.Errorf("解析数据失败: %v, body: %s", err, string(body))
			continue
		}

		// 检查API响应状态
		if result.Status.ErrorCode != 0 {
			lastErr = fmt.Errorf("API错误: %d - %s",
				result.Status.ErrorCode,
				result.Status.ErrorMessage)
			continue
		}

		log.Printf("API调用消耗积分: %d", result.Status.Credit_count)

		prices := make(map[string]float64)
		for symbol, data := range result.Data {
			price := data.Quote.USD.Price
			if price >= 0 {
				prices[symbol] = price
				log.Printf("获取到代币 %s 的价格: %s", symbol, formatPrice(price))
			}
		}

		if len(prices) > 0 {
			log.Printf("成功获取 %d/%d 个代币的价格", len(prices), len(symbols))
			return prices, nil
		}

		lastErr = fmt.Errorf("没有获取到任何有效价格")
		continue
	}

	return nil, fmt.Errorf("在%d次尝试后仍然失败: %v", maxRetries, lastErr)
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
func (s *JupiterPriceService) GetTokenPrices(mintAddrs []string) (map[string]*TokenPrice, error) {
	if len(mintAddrs) == 0 {
		return make(map[string]*TokenPrice), nil
	}

	prices := make(map[string]*TokenPrice)

	// 按批次处理mint地址
	for i := 0; i < len(mintAddrs); i += batchSize {
		end := i + batchSize
		if end > len(mintAddrs) {
			end = len(mintAddrs)
		}

		batch := mintAddrs[i:end]
		log.Printf("处理Jupiter价格批次 %d-%d，共 %d 个代币", i+1, end, len(batch))

		// 构建请求URL，添加showExtraInfo=true参数
		url := fmt.Sprintf("%s?ids=%s&vsToken=EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v&showExtraInfo=true",
			jupiterAPIEndpoint, strings.Join(batch, ","))

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Printf("创建请求失败: %v", err)
			continue
		}

		resp, err := s.client.Do(req)
		if err != nil {
			log.Printf("发送请求失败: %v", err)
			continue
		}

		var result struct {
			Data map[string]struct {
				ID        string `json:"id"`
				Type      string `json:"type"`
				Price     string `json:"price"`
				ExtraInfo struct {
					ConfidenceLevel string `json:"confidenceLevel"`
				} `json:"extraInfo"`
			} `json:"data"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			log.Printf("解析响应失败: %v", err)
			continue
		}
		resp.Body.Close()

		// 处理批次结果
		for mintAddr, data := range result.Data {
			// 检查可信度
			if data.ExtraInfo.ConfidenceLevel == "low" || data.ExtraInfo.ConfidenceLevel == "unknown" {
				log.Printf("mint地址 %s: 可信度为 %s，跳过", mintAddr, data.ExtraInfo.ConfidenceLevel)
				continue
			}

			// 检查价格字符串是否为空
			if data.Price == "" {
				log.Printf("mint地址 %s: 价格为空", mintAddr)
				continue
			}

			// 解析价格
			price, err := strconv.ParseFloat(data.Price, 64)
			if err != nil {
				log.Printf("mint地址 %s: 价格解析失败 (%s): %v", mintAddr, data.Price, err)
				continue
			}

			// 价格范围检查
			if price <= minPriceUSD || price >= maxPriceUSD {
				log.Printf("mint地址 %s: 价格超出范围 (%v)", mintAddr, price)
				continue
			}

			tokenPrice := &TokenPrice{
				Price:           price,
				Source:          PriceSourceJupiter,
				Timestamp:       time.Now(),
				ConfidenceLevel: data.ExtraInfo.ConfidenceLevel,
			}

			prices[mintAddr] = tokenPrice
			log.Printf("成功获取mint地址 %s 的价格数据: %s (可信度: %s)",
				mintAddr, formatPrice(price), tokenPrice.ConfidenceLevel)
		}

		// 添加短暂延迟避免请求过快
		if end < len(mintAddrs) {
			time.Sleep(100 * time.Millisecond)
		}
	}

	log.Printf("完成Jupiter价格获取: 成功获取 %d/%d 个代币的价格",
		len(prices), len(mintAddrs))

	// 如果没有获取到任何价格，返回错误
	if len(prices) == 0 {
		return nil, fmt.Errorf("未能获取到任何有效价格")
	}

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
	jupiterPrices, err := jupiterService.GetTokenPrices(mintAddrs)
	if err != nil {
		log.Printf("从Jupiter获取价格失败: %v", err)
	}

	var totalValue float64
	var updatedCount int
	remainingMints := make([]string, 0)
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
				continue
			}

			log.Printf("2. Jupiter价格数据:")
			log.Printf("   - 当前价格: $%.8f", price.Price)
			log.Printf("   - 可信度: %s", price.ConfidenceLevel)

			token.Price = price.Price
			// 使用原始数量计算价值
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
			remainingMints = append(remainingMints, mintAddr)
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

	// 显示Jupiter更新结果
	log.Printf("\nJupiter更新汇总:")
	log.Printf("- 成功: %d个", updatedCount)
	log.Printf("- 待处理: %d个", len(remainingMints))
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
