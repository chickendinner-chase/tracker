package config

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// TokenMetadataCache 代币元数据缓存
type TokenMetadataCache struct {
	data  map[string]*TokenMetadata
	mutex sync.RWMutex
}

// TokenMetadata 代币元数据
type TokenMetadata struct {
	Symbol    string
	Name      string
	Decimals  int
	Price     float64
	UpdatedAt time.Time
}

// WalletConfig 存储单个钱包的配置
type WalletConfig struct {
	Address string `yaml:"address"`
	Label   string `yaml:"label"`
}

// TokenConfig 存储代币配置
type TokenConfig struct {
	Address string `yaml:"address"`
	Symbol  string `yaml:"symbol"`
	Name    string `yaml:"name"`
	Decimal int    `yaml:"decimal"`
}

// Config 存储所有配置
type Config struct {
	Wallets []WalletConfig `yaml:"wallets"`
	Tokens  []TokenConfig  `yaml:"tokens"`
	cache   *TokenMetadataCache
}

// NewTokenMetadataCache 创建新的代币元数据缓存
func NewTokenMetadataCache() *TokenMetadataCache {
	return &TokenMetadataCache{
		data: make(map[string]*TokenMetadata),
	}
}

// Get 获取缓存的代币元数据
func (c *TokenMetadataCache) Get(mint string) (*TokenMetadata, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	metadata, ok := c.data[mint]
	if !ok {
		return nil, false
	}
	// 检查缓存是否过期（1分钟）
	if time.Since(metadata.UpdatedAt) > 1*time.Minute {
		return nil, false
	}
	return metadata, true
}

// Set 设置代币元数据缓存
func (c *TokenMetadataCache) Set(mint string, metadata *TokenMetadata) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	metadata.UpdatedAt = time.Now()
	c.data[mint] = metadata
}

// LoadConfig 从YAML文件加载配置
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	config.cache = NewTokenMetadataCache()
	return &config, nil
}

// SaveConfig 保存配置到YAML文件
func SaveConfig(filename string, config *Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("保存配置文件失败: %v", err)
	}

	return nil
}

// GetTokenMetadata 获取代币元数据（先从缓存获取，如果没有则返回配置中的信息）
func (c *Config) GetTokenMetadata(mint string) *TokenMetadata {
	// 先从缓存中查找
	if metadata, ok := c.cache.Get(mint); ok {
		return metadata
	}

	// 如果缓存中没有，从配置中查找
	for _, token := range c.Tokens {
		if token.Address == mint {
			metadata := &TokenMetadata{
				Symbol:   token.Symbol,
				Name:     token.Name,
				Decimals: token.Decimal,
			}
			c.cache.Set(mint, metadata)
			return metadata
		}
	}

	return nil
}

// SetTokenMetadata 设置代币元数据到缓存
func (c *Config) SetTokenMetadata(mint string, metadata *TokenMetadata) {
	c.cache.Set(mint, metadata)
}

// GetWalletAddresses 获取所有钱包地址
func (c *Config) GetWalletAddresses() []string {
	addresses := make([]string, len(c.Wallets))
	for i, w := range c.Wallets {
		addresses[i] = w.Address
	}
	return addresses
}

// GetToken 获取代币配置（兼容旧方法）
func (c *Config) GetToken(address string) *TokenConfig {
	for _, token := range c.Tokens {
		if token.Address == address {
			return &token
		}
	}
	return nil
}

// AddWallet 添加新的钱包配置
func (c *Config) AddWallet(address, label string) {
	c.Wallets = append(c.Wallets, WalletConfig{
		Address: address,
		Label:   label,
	})
}

// AddToken 添加新的代币配置
func (c *Config) AddToken(address, symbol, name string, decimal int) {
	c.Tokens = append(c.Tokens, TokenConfig{
		Address: address,
		Symbol:  symbol,
		Name:    name,
		Decimal: decimal,
	})
}

// RemoveWallet 移除钱包配置
func (c *Config) RemoveWallet(address string) {
	var newWallets []WalletConfig
	for _, w := range c.Wallets {
		if w.Address != address {
			newWallets = append(newWallets, w)
		}
	}
	c.Wallets = newWallets
}

// UpdateWallet 更新钱包配置
func (c *Config) UpdateWallet(address, newLabel string) bool {
	for i, w := range c.Wallets {
		if w.Address == address {
			c.Wallets[i].Label = newLabel
			return true
		}
	}
	return false
}
