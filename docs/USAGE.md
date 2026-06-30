# quant_ba 使用说明

## 1. 项目简介

`quant_ba` 是一个基于 Go 的量化交易系统，支持 **Binance 现货 / USDT-M 永续合约** 的
**回测、模拟交易、实盘交易** 三种模式。策略以独立插件二进制形式运行，通过
JSON-RPC 与主机通信，互不耦合。

---

## 2. 环境要求

- Go 1.20+
- macOS / Linux
- 网络：可访问 Binance（或通过代理：
  `export https_proxy=http://127.0.0.1:7897 http_proxy=http://127.0.0.1:7897`）

---

## 3. 构建

```bash
# 1) 构建主机二进制
go build -o quant_ba ./cmd/quant_ba

# 2) 构建策略插件（每个策略是一个独立二进制）
go build -o plugins/bollinger_v1 ./internal/strategy/bollinger
go build -o plugins/breakout_v1   ./internal/strategy/breakout
go build -o plugins/macd_v1       ./internal/strategy/macd
go build -o plugins/rsi_v1        ./internal/strategy/rsi
```

> `quant_ba`、`plugins/`、`data/`、`results/` 都在 `.gitignore` 中，每次拉新代码
> 后需重新构建。

---

## 4. 配置

`config/default.yaml` 是运行时配置，字段含义：

```yaml
exchange:
  name: binance
  mode: spot            # "spot" 或 "futures"（USDT-M 永续）
  base_url: https://api.binance.com
  ws_url:   wss://stream.binance.com:9443/ws
  futures_url:    https://fapi.binance.com
  futures_ws_url: wss://fstream.binance.com/ws
  api_key: ""          # 实盘交易必填
  secret:  ""          # 实盘交易必填
  testnet: true

risk:
  basic:           { max_position_pct: 0.20, max_order_usdt: 5000, max_slippage_pct: 0.02, blacklist: [] }
  global:          { max_leverage: 3.0, max_concentration: 0.30, daily_trade_limit: 100, min_hold_seconds: 60 }
  circuit_breaker: { daily_loss_pct: 0.05, consecutive_losses: 5, volatility_pause_pct: 0.15, max_drawdown_pct: 0.20 }

store:  { path: data/quant_ba.db }
server: { enabled: false, port: 8080 }
```

切换合约模式：把 `exchange.mode` 改为 `futures`。`ActiveBaseURL` / `ActiveWSURL`
会自动选择 `fapi.binance.com`。

**注意**：从 repo 根目录运行 CLI（`app.go` 会在 CWD 创建 `data/`）。

---

## 5. 常用命令

```bash
./quant_ba --help
./quant_ba -c /path/to/config.yaml <subcommand>
```

### 5.1 `config show`
打印当前生效配置。

### 5.2 `backtest run <plugin-path>`
回测（端到端可用）。

```bash
./quant_ba backtest run plugins/bollinger_v1 \
  --symbols BTCUSDT,ETHUSDT \
  --interval 1h \
  --start  2025-01-01 \
  --end    2025-12-31 \
  --capital 10000 \
  --out    results/bollinger_2025.json
```

输出指标：总收益、Sharpe、最大回撤、胜率、盈亏比。结果写入 `--out` 指定的 JSON。

### 5.3 `paper start <plugin-path>`
模拟交易（端到端可用）。默认初始资金 10,000 USDT。按 `Ctrl+C` 优雅退出。

```bash
./quant_ba paper start plugins/bollinger_v1
```

### 5.4 `live start <plugin-path>` *(当前为 stub)*
仅加载插件并打印提示，**未运行完整执行循环**——只在 `config/default.yaml` 中
填入 `api_key` / `secret` 仍不会下单。需要先实现 live executor 循环。

### 5.5 `serve`
启动 HTTP API（默认 `:8080`），端点：

- `GET /health`
- `GET /api/strategies` — 当前已加载策略
- `GET /api/portfolio` — 当前组合快照

```bash
./quant_ba serve
curl http://localhost:8080/health
```

### 5.6 `strategy` 子命令

```bash
./quant_ba strategy load   plugins/bollinger_v1   # 仅加载（不进入执行循环）
./quant_ba strategy list                          # 列出已加载策略
./quant_ba strategy unload <strategy-id>
./quant_ba strategy params <strategy-id>
```

### 5.7 `live status` / `live stop`
查看 PaperExecutor 中正在运行的策略、停止指定策略。

---

## 6. 编写自定义策略

每个策略是一个独立 Go 包，二进制即可作为插件被加载。

### 6.1 最小骨架

```go
package main

import (
    "github.com/colinmyth/quant_ba/internal/strategy"
    "github.com/colinmyth/quant_ba/internal/strategy/pluginkit"
    "github.com/colinmyth/quant_ba/internal/types"
)

type myStrat struct{}

func (s *myStrat) Meta() strategy.MetaResult {
    return strategy.MetaResult{
        ID:        "my_strat_v1",
        Name:      "My Strategy",
        Version:   "1.0",
        Symbols:   []string{"BTCUSDT"},
        Intervals: []string{"1h"},
        Params:    types.StrategyParams{},
    }
}

func (s *myStrat) OnBar(p strategy.OnBarParams) *types.Signal {
    closes := pluginkit.Closes(p.Bars)
    _ = closes
    return nil // hold
}

func main() { pluginkit.Run(&myStrat{}) }
```

### 6.2 关键约定

- **持仓模型**：Binance one-way net。`DirBuy`=多头，`DirSell`=空头，`Size: 0`=平仓。
- **市价单**：`Price` 留 0，主机用最新 K 线收盘价做风控/会计。
- **可选 `Initializer`**：实现 `OnInit(OnInitParams)` 接收启动时的余额/持仓。
- **辅助函数**：`pluginkit.Closes(bars)`、`pluginkit.HasPosition(sym, positions)`。

### 6.3 构建

```bash
go build -o plugins/my_strat_v1 ./internal/strategy/mystrat
```

---

## 7. 开发与验证

```bash
go test ./...      # 跑全部单元测试
go vet  ./...      # 静态检查
```

测试覆盖：`backtest`, `risk`, `portfolio`, `indicator`, `executor`。

---

## 8. 架构一览

```
CLI (cobra) ─┬─ config show
             ├─ backtest run  ── Backtest Engine ─┐
             ├─ paper start   ── PaperExecutor ──┤
             ├─ live start    ── (stub)          │
             ├─ serve         ── HTTP API        │
             └─ strategy *    ── Loader/Registry │
                                                 │
                          ┌──────────────────────┴──────────────────────┐
                          │   Strategy Plugin (子进程, JSON-RPC stdio)   │
                          │   Bollinger / Breakout / MACD / RSI / 自定义 │
                          └─────────────────────────────────────────────┘
                                                 │
              ┌─────────┬──────────┬─────────────┼─────────────┐
              ▼         ▼          ▼             ▼             ▼
           Market    RiskMgr    OrderMgr     Portfolio      Store
          (REST+WS)  (3 层)    (paper/live)  (spot/futures) (SQLite)
```

---

## 9. 已知限制

- `live start` 尚未实现执行循环（仅加载插件）。
- `order/live.go` 已支持端点切换（`/api/v3` ↔ `/fapi/v1`），但**杠杆仍需在
  Binance 端预先设置**（`/fapi/v1/leverage` 是粘性的），策略 meta 也尚未声明 leverage。
- `Risk` 层的 `GlobalRisk` 在合约模式下对 `Leverage` 的处理尚未做专门的
  保证金上限检查。
- WebSocket 需直连 Binance；在受限网络下用 `https_proxy` 走代理。
