package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
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
	logFile, err := os.OpenFile("wallet-tracker.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("无法创建日志文件:", err)
	}
	defer logFile.Close()

	// 设置日志输出格式
	log.SetOutput(logFile)
	log.SetFlags(log.Ltime) // 只显示时间，不显示日期

	// 检查日志级别
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO" // 默认日志级别
	}

	// 移除不必要的日志输出
	if logLevel == "DEBUG" {
		log.Printf("日志级别: %s", logLevel)
		log.Println("----------------------------------------")
		log.Println("开始执行程序...")
		log.Println("----------------------------------------")
	}

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
		if logLevel == "DEBUG" {
			log.Printf("从配置文件加载了 %d 个钱包地址", len(walletAddrs))
		}
	} else if walletAddr != "" {
		// 使用命令行指定的钱包
		walletAddrs = []string{walletAddr}
		if logLevel == "DEBUG" {
			log.Printf("使用命令行指定的钱包地址: %s", walletAddr)
		}
	} else {
		log.Fatal("请使用 -wallet 指定钱包地址或使用 -all 处理所有配置的钱包")
	}

	// 创建上下文以便优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 获取最新数据
	tokens, err := fetchTokens(ctx, walletAddrs, cfg)
	if err != nil {
		log.Fatal("获取代币数据失败:", err)
	}

	// 更新价格
	validTokens, err := updateTokenPrices(tokens, nil)
	if err != nil {
		log.Fatal("更新价格失败:", err)
	}

	// 生成初始报告
	printReport(validTokens)

	// 创建中断信号通道
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 创建并启动监控器
	monitor := tracker.NewTokenMonitor(20*time.Second, func(tokens []*tracker.TokenData) {
		printReport(tokens)
	})

	// 更新监控器数据
	monitor.UpdateTokens(validTokens)

	// 启动监控
	monitor.Start()

	// 创建定时更新代币列表的goroutine
	go func() {
		// 等待一段时间后再开始定时更新
		time.Sleep(5 * time.Minute)
		log.Println("开始定时更新代币列表...")

		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		updateData := func() {
			log.Println("执行定时更新...")
			// 获取最新数据
			tokens, err := fetchTokens(ctx, walletAddrs, cfg)
			if err != nil {
				log.Printf("更新代币数据失败: %v", err)
				return
			}

			validTokens, err := updateTokenPrices(tokens, monitor)
			if err != nil {
				log.Printf("更新价格失败: %v", err)
				return
			}

			// 更新监控器数据
			monitor.UpdateTokens(validTokens)
			log.Println("定时更新完成")
		}

		for {
			select {
			case <-ctx.Done():
				log.Println("停止定时更新")
				return
			case <-ticker.C:
				updateData()
			}
		}
	}()

	// 等待中断信号
	<-sigChan

	// 优雅退出
	monitor.Stop()

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
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		log.Printf("开始处理 %d 个钱包地址...", len(walletAddrs))
		return tracker.FetchMultipleWalletsTokens(ctx, walletAddrs, nil, cfg)
	}
}

func updateTokenPrices(tokens map[string][]*tracker.TokenData, monitor *tracker.TokenMonitor) ([]*tracker.TokenData, error) {
	validTokens, err := tracker.UpdateTokenPrices(tokens, monitor)
	if err != nil {
		return nil, fmt.Errorf("更新价格信息失败: %v", err)
	}
	return validTokens, nil
}

func printReport(tokens []*tracker.TokenData) {
	logLevel := os.Getenv("LOG_LEVEL")

	// 生成报告
	report := tracker.GenerateReport(tokens)

	// 根据日志级别决定输出内容
	switch logLevel {
	case "DEBUG":
		// Debug模式：输出完整报告
		fmt.Println(report)
		log.Println(report)
	case "WARN", "ALERT":
		// 警告和报警模式：不输出报告
	default:
		// INFO模式：只输出到控制台
		fmt.Println(report)
	}
}
