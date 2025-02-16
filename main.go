package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"wallet-tracker/config"
	"wallet-tracker/internal/tracker"

	"github.com/joho/godotenv"
)

func main() {
	// 解析命令行参数
	var (
		walletAddr string
		configFile string
		processAll bool
	)
	flag.StringVar(&walletAddr, "wallet", "", "要分析的钱包地址")
	flag.StringVar(&configFile, "config", "config/wallets.yaml", "钱包配置文件路径")
	flag.BoolVar(&processAll, "all", false, "是否处理配置文件中的所有钱包")
	flag.Parse()

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

	// 加载配置文件
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatal("加载配置文件失败:", err)
	}

	var walletAddrs []string
	if processAll {
		// 使用配置文件中的所有钱包
		walletAddrs = cfg.GetWalletAddresses()
		log.Printf("从配置文件加载了 %d 个钱包地址", len(walletAddrs))
	} else if walletAddr != "" {
		// 使用命令行指定的钱包
		walletAddrs = []string{walletAddr}
		log.Printf("使用命令行指定的钱包地址: %s", walletAddr)
	} else {
		log.Fatal("请使用 -wallet 指定钱包地址或使用 -all 处理所有配置的钱包")
	}

	// 创建上下文，设置超时
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 获取所有钱包的代币
	tokens, err := fetchTokens(ctx, walletAddrs, cfg)
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

func fetchTokens(ctx context.Context, walletAddrs []string, cfg *config.Config) (map[string][]*tracker.TokenData, error) {
	log.Printf("开始处理 %d 个钱包地址...", len(walletAddrs))

	// 使用 FetchMultipleWalletsTokens 获取所有钱包的代币
	return tracker.FetchMultipleWalletsTokens(ctx, walletAddrs, nil, cfg)
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
