package tracker

import (
	"context"

	"github.com/portto/solana-go-sdk/program/token"
)

// TokenData 保存单个 token 数据
type TokenData struct {
	MintAddr        string // mint address (唯一标识)
	Symbol          string
	Amount          float64
	Value           float64
	Decimals        uint8
	Name            string
	Raw             *token.TokenAccount
	Price           float64
	Liquidity       float64 // 代币流动性（美元）
	ConfidenceLevel string  // 价格可信度: high/medium/low
}

// TokenMap 用于存储 mint address 到 TokenData 的映射
type TokenMap map[string]*TokenData

// WalletTokens 用于存储钱包地址到代币列表的映射
type WalletTokens map[string][]*TokenData

// AggregatedToken 聚合多个钱包中同一 token 的数据
type AggregatedToken struct {
	Symbol   string
	TotalAmt float64
	TotalVal float64
}

// PriceService 价格服务接口
type PriceService interface {
	// GetTokenPrices 批量获取代币价格
	GetTokenPrices(ctx context.Context, mintAddrs []string) (map[string]float64, error)
}
