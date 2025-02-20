package tracker

import (
	"container/ring"
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

// PriceSnapshot 价格快照
type PriceSnapshot struct {
	Timestamp time.Time
	Value     float64
	TokenData map[string]*TokenData // mint地址 -> 代币数据
}

// TokenMonitor 代币监控器
type TokenMonitor struct {
	tokens         []*TokenData  // 当前监控的代币列表
	interval       time.Duration // 监控间隔
	ctx            context.Context
	cancel         context.CancelFunc
	onUpdate       func([]*TokenData) // 更新回调函数
	csvFile        *os.File           // CSV文件句柄
	alertFile      *os.File           // 报警日志文件句柄
	lastTotalValue float64            // 上次更新时的总价值
	lastUpdateTime time.Time          // 上次更新时间
	priceHistory   *ring.Ring         // 价格历史环形缓冲区
	alertThreshold float64            // 报警阈值（百分比）
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

	// 创建报警日志文件
	alertPath := "reports/alert.log"
	alertFile, err := os.OpenFile(
		alertPath,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0666,
	)
	if err != nil {
		log.Printf("创建报警日志文件失败: %v", err)
	} else {
		log.Printf("报警日志将保存到: %s", alertPath)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 创建环形缓冲区，存储最近300个数据点（假设interval为1秒，则存储5分钟数据）
	priceHistory := ring.New(300)

	return &TokenMonitor{
		tokens:         make([]*TokenData, 0),
		interval:       interval,
		ctx:            ctx,
		cancel:         cancel,
		onUpdate:       onUpdate,
		csvFile:        csvFile,
		alertFile:      alertFile,
		lastTotalValue: 0,
		lastUpdateTime: time.Time{},
		priceHistory:   priceHistory,
		alertThreshold: 5.0, // 5%的报警阈值
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
	if m.alertFile != nil {
		m.alertFile.Close()
	}
}

// checkPriceAlert 检查价格变化并生成报警
func (m *TokenMonitor) checkPriceAlert(currentSnapshot *PriceSnapshot) {
	timeWindows := []time.Duration{
		30 * time.Second, // 短期
		1 * time.Minute,  // 中期
		5 * time.Minute,  // 长期
	}

	// 遍历每个代币
	for mintAddr, currentToken := range currentSnapshot.TokenData {
		if currentToken.Value <= 0 {
			continue
		}

		log.Printf("检查代币 %s (%s) 的价格变化, 当前价值: $%.2f",
			currentToken.Symbol, mintAddr, currentToken.Value)

		// 对每个时间窗口检查价格变化
		for _, window := range timeWindows {
			// 从当前位置开始遍历整个环形缓冲区
			r := m.priceHistory
			var oldSnapshot *PriceSnapshot
			found := false

			// 遍历环形缓冲区查找合适的历史快照
			for i := 0; i < m.priceHistory.Len(); i++ {
				if r.Value != nil {
					snapshot := r.Value.(*PriceSnapshot)
					timeDiff := currentSnapshot.Timestamp.Sub(snapshot.Timestamp)

					// 放宽时间窗口匹配条件
					if timeDiff >= window && timeDiff <= window+(5*time.Second) {
						oldSnapshot = snapshot
						found = true
						break
					}
				}
				r = r.Next()
			}

			if found && oldSnapshot != nil {
				// 检查历史快照中是否存在该代币
				if oldToken, exists := oldSnapshot.TokenData[mintAddr]; exists && oldToken.Value > 0 {
					// 计算价格变化
					priceChange := ((currentToken.Price - oldToken.Price) / oldToken.Price) * 100
					// 计算价值变化（价格 * 数量的变化）
					valueChange := ((currentToken.Value - oldToken.Value) / oldToken.Value) * 100

					// 记录显著的价格变化
					if abs(priceChange) > 1.0 || abs(valueChange) > 1.0 {
						log.Printf("代币 %s 在 %s 内的变化: 价格变化率: %.2f%%, 价值变化率: %.2f%%",
							currentToken.Symbol, window, priceChange, valueChange)
					}

					// 在检查价格变化时添加详细日志
					log.Printf("检查价格变化 - 代币: %s, 窗口: %s, 当前价格: $%.8f, 历史价格: $%.8f, 变化率: %.2f%%, 阈值: %.2f%%",
						currentToken.Symbol,
						window.String(),
						currentToken.Price,
						oldToken.Price,
						priceChange,
						m.alertThreshold)

					// 如果价格变化超过阈值，生成报警
					if abs(priceChange) >= m.alertThreshold {
						alertMsg := fmt.Sprintf("⚠️ 代币价格报警 - %s (%s)\n"+
							"时间窗口: %s\n"+
							"价格变化: %.2f%%\n"+
							"当前价格: $%.8f\n"+
							"历史价格: $%.8f\n"+
							"当前价值: $%.2f",
							currentToken.Symbol,
							mintAddr,
							window.String(),
							priceChange,
							currentToken.Price,
							oldToken.Price,
							currentToken.Value)

						// 立即写入报警日志并打印
						m.writeAlertLog(alertMsg)
						log.Print(alertMsg)
					}

					// 如果价值变化超过阈值，生成报警
					if abs(valueChange) >= m.alertThreshold {
						alertMsg := fmt.Sprintf("代币价值报警 - %s (%s) %s内价值变化率: %.2f%% (从 $%.2f 到 $%.2f)",
							currentToken.Symbol,
							mintAddr,
							window.String(),
							valueChange,
							oldToken.Value,
							currentToken.Value)

						m.writeAlertLog(alertMsg)
						log.Print("⚠️ " + alertMsg)
					}
				}
			}
		}
	}
}

// abs 返回浮点数的绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
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

	// 创建当前快照
	now := time.Now()
	tokenDataMap := make(map[string]*TokenData)

	for _, token := range validTokens {
		if token.Price > 0 {
			tokenDataMap[token.MintAddr] = token
		}
	}

	currentSnapshot := &PriceSnapshot{
		Timestamp: now,
		TokenData: tokenDataMap,
	}

	// 计算总值（仅用于日志显示）
	var totalValue float64
	for _, token := range tokenDataMap {
		totalValue += token.Value
	}
	currentSnapshot.Value = totalValue

	// 将当前快照添加到环形缓冲区
	if len(tokenDataMap) > 0 {
		// 先移动到下一个位置，再设置值
		m.priceHistory = m.priceHistory.Next()
		m.priceHistory.Value = currentSnapshot

		// 添加调试日志
		log.Printf("添加新的价格快照: 时间=%s, 代币数=%d, 总价值=$%.2f",
			now.Format("15:04:05"),
			len(tokenDataMap),
			totalValue)
	}

	// 检查价格报警
	m.checkPriceAlert(currentSnapshot)

	// 生成状态消息
	var statusMsg string

	// 查找上一个快照用于计算变化率
	var previousSnapshot *PriceSnapshot
	r := m.priceHistory.Prev()
	if r.Value != nil {
		previousSnapshot = r.Value.(*PriceSnapshot)
	}

	if previousSnapshot != nil {
		// 使用快照时间计算时间差
		timeDiff := currentSnapshot.Timestamp.Sub(previousSnapshot.Timestamp).Seconds()
		if timeDiff > 0 {
			// 计算变化率
			absoluteChange := currentSnapshot.Value - previousSnapshot.Value
			percentageChange := (absoluteChange / previousSnapshot.Value) * 100
			changePerSecond := percentageChange / timeDiff

			statusMsg = fmt.Sprintf("$%.2f (%.4f%%/s | 总变化: %.4f%% | 间隔: %.1fs) [%s]",
				totalValue,
				changePerSecond,
				percentageChange,
				timeDiff,
				now.Format("15:04:05"))
		}
	} else {
		statusMsg = fmt.Sprintf("$%.2f [%s]",
			totalValue,
			now.Format("15:04:05"))
	}

	// 输出状态
	fmt.Println(statusMsg)

	// 更新状态
	m.lastTotalValue = totalValue
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

// writeAlertLog 写入报警日志
func (m *TokenMonitor) writeAlertLog(msg string) {
	if m.alertFile == nil {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	alertMsg := fmt.Sprintf("[%s] %s\n", timestamp, msg)

	if _, err := m.alertFile.WriteString(alertMsg); err != nil {
		log.Printf("写入报警日志失败: %v", err)
	}
}
