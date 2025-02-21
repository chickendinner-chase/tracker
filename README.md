# Solana 钱包资产分析工具 v0.8

一个专注于 Solana 链上资产分析的命令行工具，支持多钱包资产追踪、实时价格更新、变化率监控和智能报警功能。

## 最新特性 (v0.8)
$env:LOG_LEVEL="DEBUG"; go run main.go -all
### 1. 扩展监控范围
- 支持 TOP 50 代币实时监控
- 单个代币实时变化率显示
- 灵活的更新频率设置（5-60秒可调）
- 智能报警系统

### 2. 智能报警功能
- 价格波动报警
- 资产变动报警
- 自定义报警阈值
- 多渠道报警通知

### 3. 优化的数据结构
- 统一的日志记录系统
- 简化的数据存储方式
- 清晰的模块职责划分

### 4. 增强的终端显示
```bash
# 终端输出示例
总资产: $44467536.89 (-0.01%/s) [07:39:40]

# 详细资产列表
Mint地址	价格(USD)	价值(USD)	变化率(%/s)	时间戳
EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v	1	7792244.14	0	2/17/2025 14:28
7GCihgDB8fe6KNjn2MYtkzZcRjQy3t9GHdC8uHYmW2hr	0.26768155	633945.19	-0.01	2/17/2025 14:28 [警报：下跌]
CZcdoP3hEDd8sVKqaeipXS1acxivMeC7WdDHuCADpump	0.65557969	5440725.41	0	2/17/2025 14:28
5mbK36SZ7J19An8jFochhQS4of8g6BwUjbeCSxBSoWdp	0.03293429	144418.87	0.05	2/17/2025 14:28 [警报：上涨]
6p6xgHyF7AeE6TZkSmFsko444wqoP15icUSqi2jfGiPN	17.26540647	1813915.21	0	2/17/2025 14:28

# 简要概览
SOL: $234.5 (+0.02%/s) [警报：大幅上涨]
BONK: $1234.5 (-0.03%/s)
JTO: $567.8 (+0.01%/s)
```

### 5. 性能提升
- 优化的内存使用
- 减少重复计算
- 提高数据处理速度

## 系统架构

### 1. 数据获取层
- **钱包数据源**
  - Helius DAS API: 代币详细信息
  - RPC API: 备选数据源
  - 并发处理: 多钱包同时处理

- **价格数据源**
  - Jupiter API v3: 实时价格和可信度
  - Birdeye API: 备选价格源

### 2. 数据处理层
- **代币数据处理**
  - 智能合并同类代币
  - 自动处理代币精度
  - 多级变化率计算

- **价格处理**
  - 实时价格验证
  - 智能异常检测
  - 价格来源优先级

### 3. 报警引擎
- **多维度报警策略**
  ```
  价格报警 = 价格变化率 > 设定阈值
  资产报警 = 资产变化率 > 设定阈值
  组合报警 = 多条件联合触发
  ```
- **报警优化**
  - 报警去重处理
  - 智能报警过滤
  - 自定义报警级别

## 使用方法

### 1. 环境配置
```bash
# 复制并配置环境变量
cp .env.example .env

# 复制并配置钱包文件
cp wallets.example.yaml wallets.yaml

# 编辑配置文件，填入你的API密钥和钱包地址即可运行
```

### 2. 运行程序
```bash
# 默认模式（实时总值监控）
go run main.go -all

# 自定义模式
go run main.go -all -interval 10 -top 50
```

## 优化计划 (v0.9)

### 1. 报警系统升级
- 多渠道报警集成（Telegram, Discord, Email）
- 智能报警过滤算法
- 自定义报警模板
- 报警优先级管理

### 2. 智能分析功能
- AI 辅助异常检测
- 智能投资建议
- 历史趋势分析

### 3. 可视化增强
- Web 界面支持
- 实时图表展示
- 自定义报表生成

### 4. 数据存储升级
- 时序数据库支持
- 分布式存储方案
- 高效数据压缩

## 已解决的问题

### 1. 结构优化
- ✅ 统一的日志系统
- ✅ 明确的模块职责
- ✅ 优化的数据流转
- ✅ 基础报警功能实现

### 2. 性能提升
- ✅ 减少文件IO操作
- ✅ 优化内存使用
- ✅ 提高响应速度

### 3. 功能增强
- ✅ 单币变化率显示
- ✅ 自定义更新频率
- ✅ TOP 50 代币支持
- ✅ 基础价格报警

## 使用建议

### 1. 性能优化
- 建议使用 SSD 存储
- 定期清理历史数据
- 合理设置更新频率

### 2. 监控策略
- 根据需求调整监控范围
- 设置合适的告警阈值
- 关注重点代币变化

### 3. 报警配置
- 合理设置报警阈值
- 避免过于频繁的报警
- 定期检查报警规则

### 4. 数据安全
- 定期备份配置文件
- 保护 API 密钥安全
- 使用安全的网络环境

## 许可证

MIT License 

## 更新日志
- v1.0.0
  - 报警系统升级，在规定时间内，确保每个窗口的报警次数不超过 1 次，且新周期开始后能再次报警