package tracker

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// AggregateTokens 聚合代币数据
func AggregateTokens(tokens []*TokenData) []*AggregatedToken {
	// 按代币符号聚合
	tokenMap := make(map[string]*AggregatedToken)

	for _, token := range tokens {
		if token.Symbol == "" {
			continue
		}

		if agg, exists := tokenMap[token.Symbol]; exists {
			agg.TotalAmt += token.Amount
			agg.TotalVal += token.Value
		} else {
			tokenMap[token.Symbol] = &AggregatedToken{
				Symbol:   token.Symbol,
				TotalAmt: token.Amount,
				TotalVal: token.Value,
			}
		}
	}

	// 转换为切片并按总价值排序
	var result []*AggregatedToken
	for _, agg := range tokenMap {
		result = append(result, agg)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalVal > result[j].TotalVal
	})

	return result
}

// GenerateReport 生成代币持仓报告
func GenerateReport(tokens []*TokenData) string {
	var sb strings.Builder

	// 添加报告头部
	sb.WriteString("\n代币持仓报告\n")
	sb.WriteString("生成时间: " + time.Now().Format("2006-01-02 15:04:05") + "\n")
	sb.WriteString("----------------------------------------\n\n")

	// 表头
	sb.WriteString(fmt.Sprintf("%-6s|%-12s|%20s|%20s|%10s|\n",
		"排名", "代币", "总持有量", "总价值 (USD)", "可信度"))
	sb.WriteString(strings.Repeat("-", 74) + "\n")

	// 直接使用传入的已排序代币列表
	var totalValue float64
	for i, token := range tokens {
		// 格式化数值
		amountStr := formatAmount(token)
		valueStr := formatValue(token.Value)

		// 确保代币符号不为空
		symbol := token.Symbol
		if symbol == "" || symbol == "UNKNOWN" {
			if token.Name != "" {
				if len(token.Name) > 12 {
					symbol = token.Name[:12]
				} else {
					symbol = token.Name
				}
			} else {
				symbol = "Unknown"
			}
		}

		// 写入行数据
		confidenceLevel := token.ConfidenceLevel
		if confidenceLevel == "" {
			confidenceLevel = "unknown"
		}

		sb.WriteString(fmt.Sprintf("%-6d|%-12s|%20s|%20s|%10s|\n",
			i+1, symbol, amountStr, valueStr, confidenceLevel))

		// 添加详细计算过程
		sb.WriteString(fmt.Sprintf("      |%-66s|\n",
			fmt.Sprintf("Mint: %s", token.MintAddr)))
		sb.WriteString(fmt.Sprintf("      |%-66s|\n",
			fmt.Sprintf("数量: %.8f", token.Amount)))
		sb.WriteString(fmt.Sprintf("      |%-66s|\n",
			fmt.Sprintf("价格: $%.8f", token.Price)))
		sb.WriteString(fmt.Sprintf("      |%-66s|\n",
			fmt.Sprintf("计算: %.8f * $%.8f = $%.6f", token.Amount, token.Price, token.Value)))
		sb.WriteString(strings.Repeat("-", 74) + "\n")

		totalValue += token.Value
	}

	// 添加总计行
	sb.WriteString(fmt.Sprintf("%-6s|%-12s|%20s|%20s|%10s|\n",
		"总计", "-", "-", formatValue(totalValue), "-"))

	// 添加报告尾部
	sb.WriteString("3. 价格来源: Jupiter API\n")
	sb.WriteString("4. 可信度等级说明:\n")
	sb.WriteString("   - high: 高可信度，价格数据可靠\n")
	sb.WriteString("   - medium: 中等可信度，价格数据相对可靠\n")
	sb.WriteString("   - unknown: 未知可信度，价格数据来源不明\n")
	sb.WriteString("5. 注意: 可信度为low的代币已被过滤\n")
	sb.WriteString(fmt.Sprintf("6. 显示范围: 按价值排序的前%d名代币\n", len(tokens)))

	return sb.String()
}

// formatAmount 格式化代币数量
func formatAmount(token *TokenData) string {
	return fmt.Sprintf("%.0f", token.Amount)
}

// formatValue 格式化代币价值
func formatValue(value float64) string {
	return fmt.Sprintf("$%.0f", value)
}

// GenerateTable 生成报告表格
func GenerateTable() {
	// 创建表格写入器
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.Debug)

	// 写入表头
	fmt.Fprintln(w, "排名\t代币\t总持有量\t总价值 (USD)\t")
	fmt.Fprintln(w, "----\t----\t--------\t------------\t")

	// 获取聚合数据
	aggregated := AggregateTokens(nil) // 这里应该传入实际的tokens数据

	// 写入数据行
	for i, token := range aggregated {
		fmt.Fprintf(w, "%d\t%s\t%.0f\t$%.0f\t\n",
			i+1, token.Symbol, token.TotalAmt, token.TotalVal)
	}

	// 计算总价值
	var totalValue float64
	for _, token := range aggregated {
		totalValue += token.TotalVal
	}

	// 写入总计
	fmt.Fprintln(w, "----\t----\t--------\t------------\t")
	fmt.Fprintf(w, "总计\t-\t-\t$%.0f\t\n", totalValue)

	// 刷新输出
	if err := w.Flush(); err != nil {
		log.Printf("生成表格时发生错误: %v", err)
	}
}
