package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"wallet-tracker/config"

	"github.com/portto/solana-go-sdk/client"
)

// HeliusService Helius API服务
type HeliusService struct {
	client   *http.Client
	endpoint string
	apiKey   string
}

func NewHeliusService() (*HeliusService, error) {
	endpoint := os.Getenv("HELIUS_RPC_ENDPOINT")
	apiKey := os.Getenv("HELIUS_API_KEY")
	if endpoint == "" || apiKey == "" {
		return nil, fmt.Errorf("缺少 Helius API 配置")
	}

	return &HeliusService{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		endpoint: endpoint,
		apiKey:   apiKey,
	}, nil
}

// TokenAccount 代表一个代币账户
type TokenAccount struct {
	Mint     string
	Balance  uint64
	Decimals uint8
}

// TokenResult 代表一个数据源的结果
type TokenResult struct {
	Tokens []*TokenData
	Error  error
}

// FetchWalletTokens 获取钱包下所有 token 列表
func FetchWalletTokens(walletAddr string, rpcClient *client.Client, cfg *config.Config) ([]*TokenData, error) {
	log.Printf("开始获取钱包 %s 的代币列表...", walletAddr)
	log.Println("----------------------------------------")

	// 创建 Helius 服务实例
	helius, err := NewHeliusService()
	if err != nil {
		return nil, err
	}

	// 创建通道用于接收结果
	rpcChan := make(chan []*TokenAccount)
	dasChan := make(chan struct {
		tokens  []*TokenData
		balance uint64
		err     error
	})

	// 并发获取 RPC 和 DAS 数据
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 启动 RPC 获取 goroutine
	go func() {
		accounts, err := fetchTokenAccountsByRPC(ctx, walletAddr, helius)
		if err != nil {
			log.Printf("RPC获取失败: %v", err)
			rpcChan <- nil
			return
		}
		rpcChan <- accounts
	}()

	// 启动 DAS API 获取 goroutine
	go func() {
		tokens, balance, err := helius.fetchTokensWithDAS(ctx, walletAddr)
		dasChan <- struct {
			tokens  []*TokenData
			balance uint64
			err     error
		}{tokens, balance, err}
	}()

	// 等待两个数据源的结果
	var rpcTokens []*TokenAccount
	var dasTokens []*TokenData
	var nativeBalance uint64

	// 使用 select 处理超时
	select {
	case rpcTokens = <-rpcChan:
		log.Printf("RPC获取到 %d 个代币账户", len(rpcTokens))
	case <-ctx.Done():
		return nil, fmt.Errorf("RPC获取超时")
	}

	select {
	case dasResult := <-dasChan:
		if dasResult.err != nil {
			log.Printf("警告: DAS API获取失败: %v, 将使用RPC数据作为备选", dasResult.err)
		} else {
			dasTokens = dasResult.tokens
			nativeBalance = dasResult.balance
			log.Printf("DAS API获取到 %d 个代币", len(dasTokens))
		}
	case <-ctx.Done():
		log.Printf("警告: DAS API获取超时，将使用RPC数据作为备选")
	}

	// 合并数据
	log.Println("合并RPC和DAS API数据")
	mergedTokens := mergeTokenData(rpcTokens, dasTokens)

	// 添加原生 SOL 余额
	if nativeBalance > 0 {
		solAmount := float64(nativeBalance) / 1e9
		log.Printf("添加SOL余额: %.0f SOL", solAmount)
		mergedTokens = append(mergedTokens, &TokenData{
			MintAddr: "So11111111111111111111111111111111111111111",
			Amount:   solAmount,
			Decimals: 9,
			Symbol:   "SOL",
			Name:     "Solana",
		})
	}

	// 移除过滤规则相关代码，让价格更新后再过滤
	return mergedTokens, nil
}

// fetchTokenAccountsByRPC 使用RPC获取代币账户列表
func fetchTokenAccountsByRPC(ctx context.Context, walletAddr string, helius *HeliusService) ([]*TokenAccount, error) {
	// 发送 RPC 请求并获取响应
	var result struct {
		Result struct {
			Value []struct {
				Account struct {
					Data struct {
						Parsed struct {
							Info struct {
								Mint        string `json:"mint"`
								TokenAmount struct {
									Amount   string `json:"amount"`
									Decimals int    `json:"decimals"`
								} `json:"tokenAmount"`
							} `json:"info"`
						} `json:"parsed"`
					} `json:"data"`
				} `json:"account"`
			} `json:"value"`
		} `json:"result"`
	}

	url := fmt.Sprintf("%s/?api-key=%s", helius.endpoint, helius.apiKey)
	jsonData, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("rpc-query-%d", rand.Int()),
		"method":  "getTokenAccountsByOwner",
		"params": []interface{}{
			walletAddr,
			map[string]interface{}{
				"programId": "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA",
			},
			map[string]interface{}{
				"encoding": "jsonParsed",
			},
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := helius.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	var tokenAccounts []*TokenAccount
	for _, acc := range result.Result.Value {
		info := acc.Account.Data.Parsed.Info
		amount, err := strconv.ParseUint(info.TokenAmount.Amount, 10, 64)
		if err != nil {
			log.Printf("警告: 无法解析代币数量 %s: %v", info.TokenAmount.Amount, err)
			continue
		}

		log.Printf("RPC代币数据: Mint=%s, Amount=%s, Decimals=%d",
			info.Mint, info.TokenAmount.Amount, info.TokenAmount.Decimals)

		tokenAccounts = append(tokenAccounts, &TokenAccount{
			Mint:     info.Mint,
			Balance:  amount,
			Decimals: uint8(info.TokenAmount.Decimals),
		})
	}

	return tokenAccounts, nil
}

// fetchTokensWithDAS 使用DAS API获取代币列表
func (s *HeliusService) fetchTokensWithDAS(ctx context.Context, walletAddr string) ([]*TokenData, uint64, error) {
	var dasResponse struct {
		Result struct {
			Total         int `json:"total"`
			NativeBalance struct {
				Lamports uint64 `json:"lamports"`
			} `json:"nativeBalance"`
			Items []struct {
				Interface string `json:"interface"`
				ID        string `json:"id"`
				Content   struct {
					Metadata struct {
						Symbol string `json:"symbol"`
						Name   string `json:"name"`
					} `json:"metadata"`
				} `json:"content"`
				TokenInfo struct {
					Balance  string `json:"balance"`
					Decimals int    `json:"decimals"`
					Symbol   string `json:"symbol"`
					Name     string `json:"name"`
				} `json:"token_info"`
			} `json:"items"`
		} `json:"result"`
	}

	url := fmt.Sprintf("%s/?api-key=%s", s.endpoint, s.apiKey)
	jsonData, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("helius-query-%d", rand.Int()),
		"method":  "searchAssets",
		"params": map[string]interface{}{
			"ownerAddress": walletAddr,
			"tokenType":    "fungible",
			"displayOptions": map[string]interface{}{
				"showNativeBalance": true,
			},
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&dasResponse); err != nil {
		return nil, 0, fmt.Errorf("解析响应失败: %v", err)
	}

	var tokens []*TokenData
	for _, item := range dasResponse.Result.Items {
		symbol := item.TokenInfo.Symbol
		name := item.TokenInfo.Name
		if symbol == "" {
			symbol = item.Content.Metadata.Symbol
		}
		if name == "" {
			name = item.Content.Metadata.Name
		}

		// 直接解析为float64，因为DAS API返回的balance可能包含小数点
		balance, err := strconv.ParseFloat(item.TokenInfo.Balance, 64)
		if err != nil {
			log.Printf("警告: 无法解析代币余额 %s: %v", item.TokenInfo.Balance, err)
			continue
		}

		log.Printf("处理DAS代币数据: Mint=%s, RawBalance=%s", item.ID, item.TokenInfo.Balance)

		td := &TokenData{
			MintAddr: item.ID,
			Amount:   balance, // 直接使用解析后的float64值
			Decimals: uint8(item.TokenInfo.Decimals),
			Symbol:   symbol,
			Name:     name,
		}
		tokens = append(tokens, td)
	}

	return tokens, dasResponse.Result.NativeBalance.Lamports, nil
}

// mergeTokenData 合并RPC和DAS API的数据
func mergeTokenData(rpcTokens []*TokenAccount, dasTokens []*TokenData) []*TokenData {
	// 创建mint地址到DAS token的映射
	dasTokenMap := make(map[string]*TokenData)
	for _, token := range dasTokens {
		dasTokenMap[token.MintAddr] = token
	}

	// 合并结果
	var mergedTokens []*TokenData
	processedMints := make(map[string]bool)

	// 首先处理RPC数据
	for _, rpcToken := range rpcTokens {
		if dasToken, ok := dasTokenMap[rpcToken.Mint]; ok {
			// 如果DAS API中有对应的token，使用DAS的数据
			mergedTokens = append(mergedTokens, dasToken)
		} else {
			// 如果DAS API中没有，从RPC数据创建token数据
			log.Printf("处理RPC代币数据: Mint=%s, Balance=%d, Decimals=%d",
				rpcToken.Mint, rpcToken.Balance, rpcToken.Decimals)

			actualBalance := float64(rpcToken.Balance)
			if rpcToken.Decimals > 0 {
				actualBalance = actualBalance / math.Pow10(int(rpcToken.Decimals))
			}

			mergedTokens = append(mergedTokens, &TokenData{
				MintAddr: rpcToken.Mint,
				Amount:   actualBalance,
				Decimals: rpcToken.Decimals,
				Symbol:   "UNKNOWN",
				Name:     "Unknown Token",
			})
			log.Printf("创建RPC代币数据: Mint=%s, ActualBalance=%.8f, Decimals=%d",
				rpcToken.Mint, actualBalance, rpcToken.Decimals)
		}
		processedMints[rpcToken.Mint] = true
	}

	// 添加仅在DAS API中存在的token
	for mint, dasToken := range dasTokenMap {
		if !processedMints[mint] {
			mergedTokens = append(mergedTokens, dasToken)
		}
	}

	return mergedTokens
}

// FetchMultipleWalletsTokens 并发获取多个钱包的代币信息
func FetchMultipleWalletsTokens(ctx context.Context, walletAddrs []string, c *client.Client, cfg *config.Config) (map[string][]*TokenData, error) {
	log.Printf("开始并发获取 %d 个钱包的代币信息...", len(walletAddrs))

	// 创建结果通道
	type walletResult struct {
		address string
		tokens  []*TokenData
		err     error
	}
	resultChan := make(chan walletResult, len(walletAddrs))

	// 创建信号量来限制并发请求数
	const maxConcurrent = 2
	sem := make(chan struct{}, maxConcurrent)

	for _, addr := range walletAddrs {
		select {
		case sem <- struct{}{}: // 获取信号量
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		go func(walletAddr string) {
			defer func() {
				<-sem // 释放信号量
				if r := recover(); r != nil {
					log.Printf("处理钱包 %s 时发生panic: %v", walletAddr, r)
					resultChan <- walletResult{
						address: walletAddr,
						err:     fmt.Errorf("panic: %v", r),
					}
				}
			}()

			// 添加随机延迟，避免同时发起请求
			time.Sleep(time.Duration(500+rand.Intn(1000)) * time.Millisecond)

			select {
			case <-ctx.Done():
				resultChan <- walletResult{
					address: walletAddr,
					err:     ctx.Err(),
				}
				return
			default:
				tokens, err := FetchWalletTokens(walletAddr, c, cfg)
				resultChan <- walletResult{
					address: walletAddr,
					tokens:  tokens,
					err:     err,
				}
			}
		}(addr)
	}

	// 收集结果
	results := make(map[string][]*TokenData)
	var firstErr error
	for i := 0; i < len(walletAddrs); i++ {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		case result := <-resultChan:
			if result.err != nil {
				log.Printf("获取钱包 %s 代币失败: %v", result.address, result.err)
				if firstErr == nil {
					firstErr = result.err
				}
				continue
			}
			results[result.address] = result.tokens
		}
	}

	// 如果所有钱包都失败了，返回错误
	if len(results) == 0 && firstErr != nil {
		return nil, fmt.Errorf("所有钱包处理失败: %v", firstErr)
	}

	log.Printf("完成处理 %d 个钱包的代币信息", len(results))
	return results, nil
}
