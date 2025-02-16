package tracker

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

// TokenMonitor 代币监控器
type TokenMonitor struct {
	tokens         []*TokenData  // 当前监控的代币列表
	interval       time.Duration // 监控间隔
	ctx            context.Context
	cancel         context.CancelFunc
	onUpdate       func([]*TokenData) // 更新回调函数
	csvFile        *os.File           // CSV文件句柄
	lastTotalValue float64            // 上次更新时的总价值
	lastUpdateTime time.Time          // 上次更新时间
}

// NewTokenMonitor 创建新的代币监控器
func NewTokenMonitor(interval time.Duration, onUpdate func([]*TokenData)) *TokenMonitor {
	// 创建reports目录
	if err := os.MkdirAll("reports", 0755); err != nil {
		log.Printf("创建reports目录失败: %v", err)
	}

	// 使用固定的CSV文件名
	csvPath := "reports/monitor.csv"
	csvFile, err := os.OpenFile(
		csvPath,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0666,
	)
	if err != nil {
		log.Printf("创建CSV文件失败: %v", err)
	} else {
		log.Printf("CSV报告将保存到: %s", csvPath)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TokenMonitor{
		tokens:         make([]*TokenData, 0),
		interval:       interval,
		ctx:            ctx,
		cancel:         cancel,
		onUpdate:       onUpdate,
		csvFile:        csvFile,
		lastTotalValue: 0,
		lastUpdateTime: time.Time{},
	}
}

// UpdateTokens 更新监控的代币列表
func (m *TokenMonitor) UpdateTokens(tokens []*TokenData) {
	m.tokens = tokens
}

// Start 开始监控
func (m *TokenMonitor) Start() {
	ticker := time.NewTicker(m.interval)
	go func() {
		for {
			select {
			case <-m.ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				m.takeSnapshot()
			}
		}
	}()
}

// Stop 停止监控
func (m *TokenMonitor) Stop() {
	m.cancel()
	if m.csvFile != nil {
		m.csvFile.Close()
	}
}

// takeSnapshot 获取当前代币状态快照
func (m *TokenMonitor) takeSnapshot() {
	// 将 []*TokenData 转换为 map[string][]*TokenData
	tokenMap := make(map[string][]*TokenData)
	tokenMap["default"] = m.tokens

	// 获取最新价格
	validTokens, err := UpdateTokenPrices(tokenMap, m)
	if err != nil {
		log.Printf("更新价格失败: %v", err)
		return
	}

	// 计算当前总值和变化率
	now := time.Now()
	currentTotalValue := 0.0
	for _, token := range validTokens {
		currentTotalValue += token.Value
	}

	// 生成状态消息
	var statusMsg string
	if !m.lastUpdateTime.IsZero() {
		timeDiff := now.Sub(m.lastUpdateTime).Seconds()
		if timeDiff > 0 {
			valueDiff := ((currentTotalValue - m.lastTotalValue) / m.lastTotalValue) * 100
			changePerSecond := valueDiff / timeDiff
			statusMsg = fmt.Sprintf("$%.2f (%.2f%%/s) [%s]",
				currentTotalValue,
				changePerSecond,
				now.Format("15:04:05"))
		}
	} else {
		statusMsg = fmt.Sprintf("$%.2f [%s]",
			currentTotalValue,
			now.Format("15:04:05"))
	}

	// 输出状态
	fmt.Println(statusMsg)

	// 更新状态
	m.lastTotalValue = currentTotalValue
	m.lastUpdateTime = now

	// 写入CSV文件
	if m.csvFile != nil {
		csvReport := GenerateCSVReport(validTokens)
		if _, err := m.csvFile.WriteString(csvReport); err != nil {
			log.Printf("写入CSV报告失败: %v", err)
		}
	}

	// 触发更新回调
	if m.onUpdate != nil {
		m.onUpdate(validTokens)
	}
}
