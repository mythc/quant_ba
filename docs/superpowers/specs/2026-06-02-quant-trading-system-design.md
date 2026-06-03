# Quant Trading System Design Spec

## Overview

A quantitative trading system integrating with Binance. Multi-strategy platform supporting backtest → paper trading → live trading with complete risk management.

**Language:** Go  
**Exchange:** Binance (spot first, futures extensible)  

---

## Architecture: Modular Monolith

Single binary, internally decomposed by responsibility. Each module communicates through well-defined interfaces. Future microservice extraction is possible but not planned.

```
quant_ba/
├── cmd/quant_ba/           # Main entry, CLI commands
├── internal/
│   ├── market/             # Market data: WebSocket + REST + SQLite cache
│   ├── strategy/           # Strategy interface + plugin loader (.so)
│   ├── order/              # Order management: place, cancel, track
│   ├── risk/               # Risk engine: 3-layer pre/post checks
│   ├── backtest/           # Backtest engine: replay, simulate, stats
│   ├── executor/           # Main loop: wires market → strategy → risk → order
│   ├── portfolio/          # Portfolio: balances, positions, PnL, snapshots
│   ├── api/                # HTTP API (optional serve mode)
│   └── store/              # Data storage abstraction (SQLite impl)
├── plugins/                # Compiled .so strategy plugins
├── config/                 # Example config files
└── docs/                   # Documentation
```

---

## Module Design

### 1. Market Module

Responsibility: market data acquisition, caching, and distribution. One interface, multiple internal implementations (WebSocket live stream, REST fallback, historical replay for backtest).

```go
type DataFeed interface {
    SubscribeKline(ctx context.Context, symbol string, interval string) (<-chan Kline, error)
    FetchKlines(ctx context.Context, symbol string, interval string, limit int) ([]Kline, error)
    SubscribeOrderBook(ctx context.Context, symbol string, depth int) (<-chan OrderBook, error)
    Unsubscribe(symbol string, interval string) error
}
```

Internals:
- **WebSocket Manager** — single Binance WS connection with multi-stream subscription, auto-reconnect, heartbeat keepalive (ping every 3 minutes)
- **REST Client** — historical Kline fetcher, rate-limited (1200 weight/min)
- **SQLite Cache** — local Kline cache keyed by (symbol, interval, open_time), check cache before network
- **Data Distributor** — fan-out: one data stream per symbol/interval serves all subscribers

Binance-specific handling:
- Spot vs futures symbol mapping (BTCUSDT vs BTCUSDT_PERP)
- Unify Kline format across intervals
- WS Kline bars are live-updating (last bar changes until close)

### 2. Strategy Module

Strategy plugins compiled as `.so` files, dynamically loaded at runtime. One strategy per plugin, gob-plugin based.

```go
type Strategy interface {
    ID() string
    Name() string
    Version() string
    Symbols() []string
    Intervals() []string
    Params() StrategyParams

    OnInit(portfolio *Portfolio) error
    OnBar(symbol string, bars []Kline, portfolio *Portfolio) (*Signal, error)
    OnOrderUpdate(order Order, portfolio *Portfolio) error // optional event

    OnPause() error
    OnResume() error
    OnStop() error
}

type Signal struct {
    Symbol    string
    Direction Dir       // Buy, Sell, Hold
    Size      float64   // 0 = close position, positive = size in base asset
    Type      OrdType   // Market, Limit
    Price     float64   // limit price (limit orders only)
    Reason    string    // signal rationale for logging
}
```

Lifecycle:
```
[build .so] → [place in plugins/] → [CLI: quant_ba strategy load macd_v1]
                                          │
OnInit() ←─────────────────────────────────┘
    │
    ▼  ┌─── Live execution loop ───┐
       │  OnBar() called per bar   │
       │  returns Signal           │
       │  → risk check → place     │
       └───────────────────────────┘
    │
OnPause() / OnStop()
```

### 3. Order Module

```go
type OrderManager interface {
    Place(ctx context.Context, signal *Signal) (*Order, error)
    Cancel(ctx context.Context, orderID string) error
    Status(ctx context.Context, orderID string) (*Order, error)
    OpenOrders(ctx context.Context, symbol string) ([]Order, error)
}

type Order struct {
    ID          string
    Symbol      string
    Side        Dir
    Type        OrdType
    Price       float64
    Size        float64
    FilledSize  float64
    FilledPrice float64
    Status      OrdStatus // New, PartialFilled, Filled, Cancelled, Rejected
    CreatedAt   time.Time
    UpdatedAt   time.Time
    StrategyID  string
}
```

Pipeline: Validate → Rate Limit → Binance API → Track Order Status

Key design:
- **Quantity precision alignment** — align to LOT_SIZE/STEP_SIZE from exchange info
- **Order state machine** — New → PartialFill → Fill (terminal) | Cancelled | Rejected (terminal)
- **Exponential backoff** — retry 3x on network error (1s/2s/4s), no retry on business errors
- **PaperOrderManager** — same interface, local simulated fill for paper trading

### 4. Risk Module

Three-layer chain-of-responsibility. Any layer returning error blocks the order.

```go
type RiskManager interface {
    PreCheck(ctx context.Context, signal *Signal, portfolio *Portfolio) error
    PostCheck(ctx context.Context, order Order, portfolio *Portfolio) error
}
```

Layer 1 — Basic Limits:
- Max position size: 20% of portfolio
- Max order amount: 5000 USDT
- Market order protection: max 2% estimated slippage
- Blacklisted symbols rejected

Layer 2 — Global Limits:
- Total leverage ≤ 3x
- Single asset concentration ≤ 30%
- Daily trade count ≤ 100
- Minimum hold interval: 60s

Layer 3 — Circuit Breakers:
- Daily loss ≥ 5% → pause all strategies
- 5 consecutive losses → pause that strategy
- 30-min volatility ≥ 15% → pause trading
- Total drawdown ≥ 20% → close all positions

Configurable via YAML. Breaker state held in memory, shared across strategies.

### 5. Backtest Module

Independent engine optimized for speed and statistics. Bar-by-bar replay of historical data.

```go
type BacktestResult struct {
    TotalReturn   float64
    SharpeRatio   float64
    MaxDrawdown   float64
    WinRate       float64
    ProfitFactor  float64
    TotalTrades   int
    TradeLog      []Trade
    EquityCurve   []EquityPoint
}
```

Fill model (per-bar):
- Market orders: fill at bar VWAP ≈ (O+H+L+C)/4
- Limit orders: check if price touched within bar high-low range
- Slippage: buy × 1.0005, sell × 0.9995
- Fee: trade value × 0.001 (0.00075 with BNB discount)

Output: JSON result file + equity curve CSV for external visualization.

### 6. Executor Module

Main event loop per strategy. One goroutine per running strategy.

```
strategy.OnInit()
       │
       ▼
  ┌─────────────────────────────────────┐
  │  Main Loop (per strategy goroutine)  │
  │                                      │
  │  1. Receive new bar from DataFeed    │
  │  2. strategy.OnBar() → Signal        │
  │  3. risk.PreCheck(Signal)            │
  │  4. order.Place(Signal) → Order      │
  │  5. Wait for order status change     │
  │  6. portfolio.OnOrderFilled(Order)   │
  │  7. strategy.OnOrderUpdate(Order)    │
  │                                      │
  │  Parallel: 5s portfolio.Snapshot()   │
  │  Parallel: WS reconnect/heartbeat    │
  └─────────────────────────────────────┘
```

Three executor implementations: `LiveExecutor`, `PaperExecutor`, `BacktestExecutor` — same interface, different component wiring.

### 7. Portfolio Module

```go
type Portfolio struct {
    Balances  map[string]Balance
    Positions map[string]*Position
}

type Position struct {
    Symbol       string
    Side         Dir
    Size         float64
    EntryPrice   float64
    CurrentPrice float64
    PnL          float64
    PnLPct       float64
    UpdatedAt    time.Time
}
```

- Single mutation entry point: `OnOrderFilled()` — no concurrent writes
- Periodic snapshot: every 5 seconds to SQLite
- Restart recovery: restore from DB, then reconcile via Binance REST order history

### 8. CLI

```bash
# Strategy management
quant_ba strategy list
quant_ba strategy load grid_v1
quant_ba strategy unload grid_v1
quant_ba strategy params grid_v1

# Backtest
quant_ba backtest run grid_v1 \
  --symbols BTCUSDT,ETHUSDT \
  --interval 1h \
  --start 2025-01-01 \
  --end 2025-12-31 \
  --out results/

# Paper trading
quant_ba paper start grid_v1

# Live trading
quant_ba live start grid_v1
quant_ba live stop grid_v1
quant_ba live status

# Global
quant_ba serve       # HTTP API + dashboard
quant_ba config show
```

---

## Data Flow (Live Trading)

```
Binance WS/REST
       │
       ▼
  [market] ───────► [executor] ───────► [strategy plugin]
                        │                      │
                        ▼                      ▼
                    [risk]                [portfolio]
                        │                      │
                        ▼                      ▼
                    [order] ◄──────── [signal + sizing]
                        │
                        ▼
                    Binance API
```

---

## Technology Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go | Concurrency model matches system needs; single binary deployment |
| Storage | SQLite | Lightweight, zero-ops, sufficient for single-instance use |
| Data format | JSON for config/results, SQLite for cached market data | Human-readable config; fast query for time-series |
| Plugin system | go-plugin (.so) | Dynamic strategy loading with crash isolation |
| HTTP framework | net/http + chi (stdlib-compatible) | Lightweight, no heavy framework needed |
| CLI framework | cobra | Standard Go CLI, subcommand support |

---

## What This System Does NOT Cover (v1 Scope)

- No built-in indicator library (strategies implement their own)
- No GUI dashboard (CLI + optional HTTP API only)
- No multi-exchange support beyond Binance
- No distributed/HA deployment (single process)
- No strategy marketplace or version control
- No Telegram/WeChat bot integration

---

## Implementation Order

1. Project scaffold + CLI skeleton + config loading
2. Market module (WS + REST + SQLite cache)
3. Strategy interface + plugin loader (with a sample strategy)
4. Portfolio module
5. Order module (live + paper)
6. Risk module
7. Executor (paper mode first)
8. Live executor (Binance API key integration)
9. Backtest engine
10. HTTP API (optional serve mode)
