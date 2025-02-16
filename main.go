package main

import (
	"fmt"
	"log"
	"os"

	"wallet-tracker/internal/tracker"

	"github.com/joho/godotenv"
)

const targetWalletAddr = "MfDuWeqSHEqTFVYZ7LoexgAK9dxk7cy4DFJWjWMGVWa"

func main() {
	// 配置日志输出到文件
	logFile, err := os.OpenFile("wallet-tracker.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatal("无法创建日志文件:", err)
	}
	defer logFile.Close()

	// 设置日志输出格式
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	log.Println("开始执行程序...")
	log.Println("----------------------------------------")

	if err := initEnv(); err != nil {
		log.Fatal(err)
	}

	tokens, err := fetchTokens()
	if err != nil {
		log.Fatal(err)
	}

	// 更新代币价格并获取已排序的有效代币列表
	validTokens, err := updateTokenPrices(tokens)
	if err != nil {
		log.Fatal(err)
	}

	// 将报告同时输出到控制台和日志文件
	report := generateReport(validTokens)
	fmt.Print(report) // 输出到控制台
	log.Print(report) // 输出到日志文件

	log.Println("----------------------------------------")
	log.Println("程序执行完成")
}

func initEnv() error {
	if err := godotenv.Load(); err != nil {
		return fmt.Errorf("加载环境变量失败: %v", err)
	}
	return nil
}

func fetchTokens() (map[string][]*tracker.TokenData, error) {
	log.Printf("处理钱包地址: %s", targetWalletAddr)

	// 获取原始代币列表
	tokenList, err := tracker.FetchWalletTokens(targetWalletAddr, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("获取代币列表失败: %v", err)
	}

	// 转换为 map[string][]*TokenData
	tokenMap := make(map[string][]*tracker.TokenData)
	for _, token := range tokenList {
		tokenMap[token.MintAddr] = append(tokenMap[token.MintAddr], token)
	}

	return tokenMap, nil
}

func updateTokenPrices(tokens map[string][]*tracker.TokenData) ([]*tracker.TokenData, error) {
	validTokens, err := tracker.UpdateTokenPrices(tokens)
	if err != nil {
		return nil, fmt.Errorf("更新价格信息失败: %v", err)
	}
	return validTokens, nil
}

func generateReport(tokens []*tracker.TokenData) string {
	// 直接使用已排序的代币列表生成报告
	return tracker.GenerateReport(tokens)
}
