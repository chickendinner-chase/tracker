package tracker

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// TokenReport 代币报告数据
type TokenReport struct {
	Symbol    string
	Amount    float64
	Value     float64
	Change    float64 // 变化率
	Timestamp time.Time
}

// GenerateReport 生成代币持仓报告
func GenerateReport(tokens []*TokenData) string {
	// 按价值排序
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Value > tokens[j].Value
	})

	// 获取日志级别
	logLevel := os.Getenv("LOG_LEVEL")

	// 根据日志级别生成不同格式的报告
	switch logLevel {
	case "DEBUG":
		return generateDebugReport(tokens)
	case "WARN", "ALERT":
		return "" // 警告和报警模式不生成报告
	default:
		return generateSimpleReport(tokens)
	}
}

// generateSimpleReport 生成简单报告（默认模式）
func generateSimpleReport(tokens []*TokenData) string {
	var sb strings.Builder
	var totalValue float64

	// 显示前50个代币
	maxTokens := 50
	if len(tokens) < maxTokens {
		maxTokens = len(tokens)
	}

	// 生成表格
	sb.WriteString(fmt.Sprintf("\n%-4s %-16s %16s %16s %10s\n",
		"#", "代币", "价格", "价值", "占比"))
	sb.WriteString(strings.Repeat("-", 66) + "\n")

	// 先计算总值用于计算占比
	for _, token := range tokens[:maxTokens] {
		totalValue += token.Value
	}

	for i, token := range tokens[:maxTokens] {
		symbol := token.Symbol
		if symbol == "" || symbol == "UNKNOWN" {
			if token.Name != "" {
				if len(token.Name) > 16 {
					symbol = token.Name[:16]
				} else {
					symbol = token.Name
				}
			} else {
				symbol = "Unknown"
			}
		}

		// 计算该代币占总值的百分比
		percentage := (token.Value / totalValue) * 100

		sb.WriteString(fmt.Sprintf("%-4d %-16s %16.4f %16.2f %9.2f%%\n",
			i+1,
			symbol,
			token.Price,
			token.Value,
			percentage))
	}

	sb.WriteString(fmt.Sprintf("总值: $%.2f [%s]\n",
		totalValue,
		time.Now().Format("15:04:05")))

	return sb.String()
}

// generateDebugReport 生成详细报告（调试模式）
func generateDebugReport(tokens []*TokenData) string {
	var sb strings.Builder
	var totalValue float64

	sb.WriteString("\n详细代币报告\n")
	sb.WriteString("时间: " + time.Now().Format("15:04:05") + "\n")
	sb.WriteString(strings.Repeat("-", 80) + "\n")

	// 显示所有代币的详细信息
	for i, token := range tokens {
		sb.WriteString(fmt.Sprintf("代币 #%d: %s\n", i+1, token.Symbol))
		sb.WriteString(fmt.Sprintf("  Mint地址: %s\n", token.MintAddr))
		sb.WriteString(fmt.Sprintf("  价格: $%.8f\n", token.Price))
		sb.WriteString(fmt.Sprintf("  数量: %.8f\n", token.Amount))
		sb.WriteString(fmt.Sprintf("  价值: $%.2f\n", token.Value))
		sb.WriteString(fmt.Sprintf("  可信度: %s\n", token.ConfidenceLevel))
		sb.WriteString(strings.Repeat("-", 80) + "\n")

		totalValue += token.Value
	}

	sb.WriteString(fmt.Sprintf("总资产价值: $%.2f\n", totalValue))
	return sb.String()
}

// GenerateCSVReport 生成CSV格式的报告
func GenerateCSVReport(tokens []*TokenData) string {
	var sb strings.Builder

	// 复制tokens切片以避免修改原始数据
	sortedTokens := make([]*TokenData, len(tokens))
	copy(sortedTokens, tokens)

	// 按价值排序
	sort.Slice(sortedTokens, func(i, j int) bool {
		return sortedTokens[i].Value > sortedTokens[j].Value
	})

	// 写入CSV头部（如果文件为空的话）
	if sb.Len() == 0 {
		sb.WriteString("Mint地址,价格(USD),价值(USD),变化率(%/s),时间戳\n")
	}

	// 写入数据行
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	for _, token := range sortedTokens {
		// 计算变化率 (使用token.Change，这个值需要在更新价值时计算)
		changeRate := token.Change

		// 写入CSV行
		sb.WriteString(fmt.Sprintf("%s,%.8f,%.2f,%.2f,%s\n",
			token.MintAddr,
			token.Price,
			token.Value,
			changeRate,
			timestamp))
	}

	return sb.String()
}
