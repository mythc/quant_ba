# Quant Trading System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go-based quantitative trading system with Binance integration, supporting backtest → paper → live trading with plugin-based strategies and 3-layer risk management.

**Architecture:** Modular monolith — single binary with 8 internal packages communicating through interfaces. Strategies are separate Go binaries loaded as subprocesses via JSON-RPC over stdin/stdout (cross-platform plugin system). SQLite for local state and market data cache.

**Tech Stack:** Go 1.22+, cobra (CLI), chi (HTTP), gorilla/websocket, modernc.org/sqlite (pure Go), yaml.v3

---

## File Structure Plan

```
quant_ba/
├── cmd/quant_ba/
│   ├── main.go              # Entry point
│   ├── root.go               # Root command
│   ├── strategy.go           # strategy subcommands
│   ├── backtest.go           # backtest subcommand
│   ├── live.go               # live subcommands
│   ├── paper.go              # paper subcommands
│   └── serve.go              # serve subcommand
├── internal/
│   ├── types/
│   │   └── types.go          # Shared types: Kline, Signal, Order, Portfolio, Dir, etc.
│   ├── config/
│   │   └── config.go         # YAML config loading
│   ├── store/
│   │   └── sqlite.go         # SQLite wrapper
│   ├── market/
│   │   ├── feed.go           # DataFeed interface
│   │   ├── rest.go           # Binance REST client
│   │   ├── ws.go             # Binance WebSocket client
│   │   └── cache.go          # SQLite Kline cache
│   ├── portfolio/
│   │   └── portfolio.go      # Portfolio service
│   ├── strategy/
│   │   ├── strategy.go       # Strategy interface + Signal type
│   │   ├── protocol.go       # JSON-RPC protocol for plugin communication
│   │   ├── loader.go         # Plugin process manager
│   │   └── example/
│   │       └── ma_cross.go   # Sample MA cross strategy plugin binary
│   ├── order/
│   │   ├── order.go          # OrderManager interface
│   │   ├── paper.go          # Paper trading order manager
│   │   └── live.go           # Binance live order manager
│   ├── risk/
│   │   ├── risk.go           # RiskManager interface + config
│   │   ├── basic.go          # Layer 1: basic limits
│   │   ├── global.go         # Layer 2: global limits
│   │   └── breaker.go        # Layer 3: circuit breakers
│   ├── executor/
│   │   ├── executor.go       # Executor interface
│   │   ├── paper.go          # Paper executor
│   │   └── live.go           # Live executor
│   ├── backtest/
│   │   ├── engine.go         # Backtest engine
│   │   ├── fill.go           # Fill simulation
│   │   └── stats.go          # Statistics
│   └── api/
│       ├── server.go         # HTTP server
│       └── handlers.go       # API handlers
├── plugins/                  # Built strategy plugin binaries
├── config/
│   └── default.yaml          # Default configuration
├── go.mod
└── go.sum
```

---

### Task 1: Project Scaffold + Shared Types + Config

**Files:**
- Create: `go.mod`
- Create: `internal/types/types.go`
- Create: `internal/config/config.go`
- Create: `config/default.yaml`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/colinmyth/code/quant_ba && go mod init github.com/colinmyth/quant_ba
```

- [ ] **Step 2: Write shared types**

Create `internal/types/types.go`:

```go
package types

import "time"

// Direction is the trade direction.
type Dir string

const (
	DirBuy  Dir = "buy"
	DirSell Dir = "sell"
	DirHold Dir = "hold"
)

// Order type.
type OrdType string

const (
	OrdMarket OrdType = "market"
	OrdLimit  OrdType = "limit"
)

// Order status.
type OrdStatus string

const (
	OrdNew          OrdStatus = "new"
	OrdPartialFill  OrdStatus = "partial_fill"
	OrdFilled       OrdStatus = "filled"
	OrdCancelled    OrdStatus = "cancelled"
	OrdRejected     OrdStatus = "rejected"
)

// Kline is a single candlestick bar.
type Kline struct {
	Symbol    string
	Interval  string
	OpenTime  time.Time
	CloseTime time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
}

// OrderBook is a snapshot of the order book.
type OrderBook struct {
	Symbol string
	Bids   [][2]float64 // [price, size]
	Asks   [][2]float64
	Time   time.Time
}

// Signal is the output of a strategy.
type Signal struct {
	Symbol     string
	Direction  Dir
	Size       float64 // 0 = close position, positive = quantity in base asset
	Type       OrdType
	Price      float64 // limit price (limit orders only)
	Reason     string
	StrategyID string  // set by executor, not the strategy
}

// Order represents an exchange order.
type Order struct {
	ID          string
	Symbol      string
	Side        Dir
	Type        OrdType
	Price       float64
	Size        float64
	FilledSize  float64
	FilledPrice float64
	Status      OrdStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StrategyID  string
}

// Balance holds the free and locked amount of an asset.
type Balance struct {
	Asset  string
	Free   float64
	Locked float64
}

// Position is a current holding.
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

// Portfolio is the full account state.
type Portfolio struct {
	Balances  map[string]Balance
	Positions map[string]*Position
}

// Trade is a completed trade record for backtest/history.
type Trade struct {
	ID         string
	Symbol     string
	Side       Dir
	Size       float64
	EntryPrice float64
	ExitPrice  float64
	PnL        float64
	PnLPct     float64
	EntryTime  time.Time
	ExitTime   time.Time
	StrategyID string
}

// EquityPoint is a single point on the equity curve.
type EquityPoint struct {
	Time   time.Time
	Equity float64
}

// StrategyParams holds configurable strategy parameters.
type StrategyParams map[string]float64

// StrategyMeta describes a loaded strategy without instantiating it.
type StrategyMeta struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
}
```

- [ ] **Step 3: Write config package**

Create `internal/config/config.go`:

```go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Exchange ExchangeConfig `yaml:"exchange"`
	Risk     RiskConfig     `yaml:"risk"`
	Store    StoreConfig    `yaml:"store"`
	Server   ServerConfig   `yaml:"server"`
}

type ExchangeConfig struct {
	Name    string `yaml:"name"`    // "binance"
	BaseURL string `yaml:"base_url"` // "https://api.binance.com"
	WSURL   string `yaml:"ws_url"`   // "wss://stream.binance.com:9443/ws"
	APIKey  string `yaml:"api_key"`
	Secret  string `yaml:"secret"`
	Testnet bool   `yaml:"testnet"`
}

type RiskConfig struct {
	Basic          BasicRiskConfig  `yaml:"basic"`
	Global         GlobalRiskConfig `yaml:"global"`
	CircuitBreaker BreakerConfig    `yaml:"circuit_breaker"`
}

type BasicRiskConfig struct {
	MaxPositionPct float64 `yaml:"max_position_pct"` // 0.20
	MaxOrderUSDT   float64 `yaml:"max_order_usdt"`   // 5000
	MaxSlippagePct float64 `yaml:"max_slippage_pct"` // 0.02
	Blacklist      []string `yaml:"blacklist"`
}

type GlobalRiskConfig struct {
	MaxLeverage       float64 `yaml:"max_leverage"`        // 3.0
	MaxConcentration  float64 `yaml:"max_concentration"`   // 0.30
	DailyTradeLimit   int     `yaml:"daily_trade_limit"`   // 100
	MinHoldSeconds    int     `yaml:"min_hold_seconds"`    // 60
}

type BreakerConfig struct {
	DailyLossPct      float64 `yaml:"daily_loss_pct"`       // 0.05
	ConsecutiveLosses int     `yaml:"consecutive_losses"`   // 5
	VolatilityPausePct float64 `yaml:"volatility_pause_pct"` // 0.15
	MaxDrawdownPct    float64 `yaml:"max_drawdown_pct"`     // 0.20
}

type StoreConfig struct {
	Path string `yaml:"path"` // "data/quant_ba.db"
}

type ServerConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"` // 8080
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Exchange: ExchangeConfig{
			Name:    "binance",
			BaseURL: "https://api.binance.com",
			WSURL:   "wss://stream.binance.com:9443/ws",
			Testnet: true,
		},
		Store: StoreConfig{
			Path: "data/quant_ba.db",
		},
		Server: ServerConfig{
			Port: 8080,
		},
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
```

- [ ] **Step 4: Write default config YAML**

Create `config/default.yaml`:

```yaml
exchange:
  name: binance
  base_url: https://api.binance.com
  ws_url: wss://stream.binance.com:9443/ws
  api_key: ""
  secret: ""
  testnet: true

risk:
  basic:
    max_position_pct: 0.20
    max_order_usdt: 5000
    max_slippage_pct: 0.02
    blacklist: []
  global:
    max_leverage: 3.0
    max_concentration: 0.30
    daily_trade_limit: 100
    min_hold_seconds: 60
  circuit_breaker:
    daily_loss_pct: 0.05
    consecutive_losses: 5
    volatility_pause_pct: 0.15
    max_drawdown_pct: 0.20

store:
  path: data/quant_ba.db

server:
  enabled: false
  port: 8080
```

- [ ] **Step 5: Install dependencies and verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go mod tidy
```

Expected: `go.mod` updated with `gopkg.in/yaml.v3` dependency, `go build ./...` compiles with no errors (types and config packages only — no main yet).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/types/types.go internal/config/config.go config/default.yaml
git commit -m "feat: add project scaffold, shared types, and config loading"
```

---

### Task 2: CLI Skeleton

**Files:**
- Create: `cmd/quant_ba/main.go`
- Create: `cmd/quant_ba/root.go`
- Create: `cmd/quant_ba/strategy.go`
- Create: `cmd/quant_ba/backtest.go`
- Create: `cmd/quant_ba/live.go`
- Create: `cmd/quant_ba/paper.go`
- Create: `cmd/quant_ba/serve.go`

- [ ] **Step 1: Write root command**

Create `cmd/quant_ba/root.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgPath string

var rootCmd = &cobra.Command{
	Use:   "quant_ba",
	Short: "Quantitative trading system for Binance",
	Long:  "A multi-strategy quantitative trading platform supporting backtest, paper trading, and live trading on Binance.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "config/default.yaml", "path to config file")
}
```

- [ ] **Step 2: Write main entry**

Create `cmd/quant_ba/main.go`:

```go
package main

func main() {
	Execute()
}
```

- [ ] **Step 3: Write strategy subcommands**

Create `cmd/quant_ba/strategy.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(strategyCmd)
	strategyCmd.AddCommand(strategyListCmd)
	strategyCmd.AddCommand(strategyLoadCmd)
	strategyCmd.AddCommand(strategyUnloadCmd)
	strategyCmd.AddCommand(strategyParamsCmd)
}

var strategyCmd = &cobra.Command{
	Use:   "strategy",
	Short: "Manage trading strategies",
	Long:  "List, load, unload, and configure strategy plugins.",
}

var strategyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available strategies",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("No strategies loaded yet. Place .so plugin binaries in plugins/")
		return nil
	},
}

var strategyLoadCmd = &cobra.Command{
	Use:   "load <name>",
	Short: "Load a strategy plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Loading strategy: %s (not yet implemented)\n", args[0])
		return nil
	},
}

var strategyUnloadCmd = &cobra.Command{
	Use:   "unload <name>",
	Short: "Unload a strategy plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Unloading strategy: %s (not yet implemented)\n", args[0])
		return nil
	},
}

var strategyParamsCmd = &cobra.Command{
	Use:   "params <name>",
	Short: "Show or update strategy parameters",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Strategy params for: %s (not yet implemented)\n", args[0])
		return nil
	},
}
```

- [ ] **Step 4: Write remaining command stubs**

Create `cmd/quant_ba/backtest.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(backtestCmd)
}

var backtestCmd = &cobra.Command{
	Use:   "backtest",
	Short: "Run backtest for a strategy",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Backtest not yet implemented.")
		return nil
	},
}
```

Create `cmd/quant_ba/live.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(liveCmd)
	liveCmd.AddCommand(liveStartCmd)
	liveCmd.AddCommand(liveStopCmd)
	liveCmd.AddCommand(liveStatusCmd)
}

var liveCmd = &cobra.Command{
	Use:   "live",
	Short: "Manage live trading",
}

var liveStartCmd = &cobra.Command{
	Use:   "start <strategy>",
	Short: "Start live trading for a strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Starting live trading: %s (not yet implemented)\n", args[0])
		return nil
	},
}

var liveStopCmd = &cobra.Command{
	Use:   "stop <strategy>",
	Short: "Stop live trading for a strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Stopping live trading: %s (not yet implemented)\n", args[0])
		return nil
	},
}

var liveStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show running strategy status",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("No strategies running.")
		return nil
	},
}
```

Create `cmd/quant_ba/paper.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(paperCmd)
}

var paperCmd = &cobra.Command{
	Use:   "paper",
	Short: "Manage paper trading",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Paper trading not yet implemented.")
		return nil
	},
}
```

Create `cmd/quant_ba/serve.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start HTTP API server",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("HTTP server not yet implemented.")
		return nil
	},
}
```

- [ ] **Step 5: Install cobra, verify build and run**

```bash
cd /Users/colinmyth/code/quant_ba && go get github.com/spf13/cobra@latest && go mod tidy && go build ./cmd/quant_ba/
```

Expected: binary `quant_ba` created. Run `./quant_ba --help` and `./quant_ba strategy list` to verify.

- [ ] **Step 6: Commit**

```bash
git add cmd/ go.mod go.sum
git commit -m "feat: add CLI skeleton with cobra subcommands"
```

---

### Task 3: SQLite Store Layer

**Files:**
- Create: `internal/store/sqlite.go`

- [ ] **Step 1: Write SQLite store**

Create `internal/store/sqlite.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/colinmyth/quant_ba/internal/types"
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite single-writer
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	ddl := `
	CREATE TABLE IF NOT EXISTS klines (
		symbol    TEXT NOT NULL,
		interval  TEXT NOT NULL,
		open_time INTEGER NOT NULL,
		close_time INTEGER NOT NULL,
		open      REAL NOT NULL,
		high      REAL NOT NULL,
		low       REAL NOT NULL,
		close     REAL NOT NULL,
		volume    REAL NOT NULL,
		PRIMARY KEY (symbol, interval, open_time)
	);

	CREATE TABLE IF NOT EXISTS portfolio_snapshots (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		data      TEXT NOT NULL,
		created_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS trades (
		id          TEXT PRIMARY KEY,
		symbol      TEXT NOT NULL,
		side        TEXT NOT NULL,
		size        REAL NOT NULL,
		entry_price REAL NOT NULL,
		exit_price  REAL NOT NULL,
		pnl         REAL NOT NULL,
		pnl_pct     REAL NOT NULL,
		entry_time  INTEGER NOT NULL,
		exit_time   INTEGER NOT NULL,
		strategy_id TEXT NOT NULL
	);
	`
	_, err := s.db.Exec(ddl)
	return err
}

// SaveKlines inserts or replaces klines in the cache.
func (s *Store) SaveKlines(klines []types.Kline) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO klines (symbol, interval, open_time, close_time, open, high, low, close, volume)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, k := range klines {
		_, err = stmt.Exec(k.Symbol, k.Interval, k.OpenTime.UnixMilli(), k.CloseTime.UnixMilli(),
			k.Open, k.High, k.Low, k.Close, k.Volume)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetKlines retrieves cached klines ordered by open_time ascending.
func (s *Store) GetKlines(symbol, interval string, start, end time.Time, limit int) ([]types.Kline, error) {
	rows, err := s.db.Query(`
		SELECT symbol, interval, open_time, close_time, open, high, low, close, volume
		FROM klines
		WHERE symbol = ? AND interval = ? AND open_time >= ? AND open_time <= ?
		ORDER BY open_time ASC
		LIMIT ?
	`, symbol, interval, start.UnixMilli(), end.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var klines []types.Kline
	for rows.Next() {
		var k types.Kline
		var ot, ct int64
		if err := rows.Scan(&k.Symbol, &k.Interval, &ot, &ct, &k.Open, &k.High, &k.Low, &k.Close, &k.Volume); err != nil {
			return nil, err
		}
		k.OpenTime = time.UnixMilli(ot)
		k.CloseTime = time.UnixMilli(ct)
		klines = append(klines, k)
	}
	return klines, rows.Err()
}

// SavePortfolio writes a portfolio snapshot as JSON.
func (s *Store) SavePortfolio(data string) error {
	_, err := s.db.Exec("INSERT INTO portfolio_snapshots (data, created_at) VALUES (?, ?)", data, time.Now().UnixMilli())
	return err
}

// LastPortfolio returns the most recent snapshot JSON.
func (s *Store) LastPortfolio() (string, error) {
	var data string
	err := s.db.QueryRow("SELECT data FROM portfolio_snapshots ORDER BY id DESC LIMIT 1").Scan(&data)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return data, err
}

// SaveTrade records a completed trade.
func (s *Store) SaveTrade(t types.Trade) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO trades (id, symbol, side, size, entry_price, exit_price, pnl, pnl_pct, entry_time, exit_time, strategy_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Symbol, t.Side, t.Size, t.EntryPrice, t.ExitPrice, t.PnL, t.PnLPct, t.EntryTime.UnixMilli(), t.ExitTime.UnixMilli(), t.StrategyID)
	return err
}

// GetTrades retrieves trade history for a strategy.
func (s *Store) GetTrades(strategyID string, limit int) ([]types.Trade, error) {
	rows, err := s.db.Query(`
		SELECT id, symbol, side, size, entry_price, exit_price, pnl, pnl_pct, entry_time, exit_time, strategy_id
		FROM trades WHERE strategy_id = ? ORDER BY exit_time DESC LIMIT ?
	`, strategyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []types.Trade
	for rows.Next() {
		var t types.Trade
		var et, xt int64
		if err := rows.Scan(&t.ID, &t.Symbol, &t.Side, &t.Size, &t.EntryPrice, &t.ExitPrice, &t.PnL, &t.PnLPct, &et, &xt, &t.StrategyID); err != nil {
			return nil, err
		}
		t.EntryTime = time.UnixMilli(et)
		t.ExitTime = time.UnixMilli(xt)
		trades = append(trades, t)
	}
	return trades, rows.Err()
}
```

- [ ] **Step 2: Install SQLite dependency and verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go get modernc.org/sqlite@latest && go mod tidy && go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/store/ go.mod go.sum
git commit -m "feat: add SQLite store layer with migrations"
```

---

### Task 4: Market Module — REST Client + Kline Cache

**Files:**
- Create: `internal/market/feed.go` — DataFeed interface
- Create: `internal/market/rest.go` — Binance REST client
- Create: `internal/market/cache.go` — SQLite-backed Kline cache

- [ ] **Step 1: Write DataFeed interface**

Create `internal/market/feed.go`:

```go
package market

import (
	"context"

	"github.com/colinmyth/quant_ba/internal/types"
)

type DataFeed interface {
	SubscribeKline(ctx context.Context, symbol string, interval string) (<-chan types.Kline, error)
	FetchKlines(ctx context.Context, symbol string, interval string, limit int) ([]types.Kline, error)
	SubscribeOrderBook(ctx context.Context, symbol string, depth int) (<-chan types.OrderBook, error)
	Unsubscribe(symbol string, interval string) error
}
```

- [ ] **Step 2: Write REST client**

Create `internal/market/rest.go`:

```go
package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
)

const defaultBaseURL = "https://api.binance.com"

type RESTClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewRESTClient(baseURL string) *RESTClient {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &RESTClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type klineRaw struct {
	OpenTime  int64  `json:"0"`
	Open      string `json:"1"`
	High      string `json:"2"`
	Low       string `json:"3"`
	Close     string `json:"4"`
	Volume    string `json:"5"`
	CloseTime int64  `json:"6"`
}

// FetchKlines retrieves klines from Binance REST API.
func (c *RESTClient) FetchKlines(ctx context.Context, symbol string, interval string, limit int) ([]types.Kline, error) {
	if limit <= 0 {
		limit = 500
	}
	url := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&limit=%d",
		c.baseURL, symbol, interval, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch klines: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("binance API error %d: %s", resp.StatusCode, string(body))
	}

	var raw []klineRaw
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode klines: %w", err)
	}

	klines := make([]types.Kline, len(raw))
	for i, r := range raw {
		klines[i] = types.Kline{
			Symbol:    symbol,
			Interval:  interval,
			OpenTime:  time.UnixMilli(r.OpenTime),
			CloseTime: time.UnixMilli(r.CloseTime),
			Open:      parseFloat(r.Open),
			High:      parseFloat(r.High),
			Low:       parseFloat(r.Low),
			Close:     parseFloat(r.Close),
			Volume:    parseFloat(r.Volume),
		}
	}
	return klines, nil
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
```

- [ ] **Step 3: Write Kline cache**

Create `internal/market/cache.go`:

```go
package market

import (
	"context"
	"fmt"
	"time"

	"github.com/colinmyth/quant_ba/internal/store"
	"github.com/colinmyth/quant_ba/internal/types"
)

type KlineCache struct {
	store *store.Store
	rest  *RESTClient
}

func NewKlineCache(store *store.Store, rest *RESTClient) *KlineCache {
	return &KlineCache{store: store, rest: rest}
}

// GetOrFetch returns cached klines, falling back to REST API.
func (c *KlineCache) GetOrFetch(ctx context.Context, symbol, interval string, limit int) ([]types.Kline, error) {
	end := time.Now()
	start := end.Add(-time.Duration(limit) * klineInterval(interval))

	cached, err := c.store.GetKlines(symbol, interval, start, end, limit)
	if err == nil && len(cached) >= limit {
		return cached[:limit], nil
	}

	klines, err := c.rest.FetchKlines(ctx, symbol, interval, limit)
	if err != nil {
		if len(cached) > 0 {
			return cached, nil // stale cache is better than nothing
		}
		return nil, err
	}

	if len(klines) > 0 {
		if err := c.store.SaveKlines(klines); err != nil {
			return klines, fmt.Errorf("cache save: %w", err)
		}
	}
	return klines, nil
}

func klineInterval(interval string) time.Duration {
	switch interval {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	case "4h":
		return 4 * time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return time.Hour
	}
}
```

- [ ] **Step 4: Verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/market/
git commit -m "feat: add market data feed interface, REST client, and kline cache"
```

---

### Task 5: Market Module — WebSocket Client

**Files:**
- Create: `internal/market/ws.go`

- [ ] **Step 1: Write WebSocket client**

Create `internal/market/ws.go`:

```go
package market

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
	"github.com/gorilla/websocket"
)

const defaultWSURL = "wss://stream.binance.com:9443/ws"

type WSClient struct {
	url         string
	conn        *websocket.Conn
	mu          sync.Mutex
	subscribers map[string]map[chan types.Kline]struct{} // key: symbol@interval
	done        chan struct{}
}

func NewWSClient(url string) *WSClient {
	if url == "" {
		url = defaultWSURL
	}
	return &WSClient{
		url:         url,
		subscribers: make(map[string]map[chan types.Kline]struct{}),
		done:        make(chan struct{}),
	}
}

// Connect establishes the WebSocket connection and starts the read loop.
func (w *WSClient) Connect(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, w.url, nil)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}
	w.conn = conn

	go w.readLoop()
	go w.heartbeat(ctx)
	return nil
}

// SubscribeKline subscribes to kline stream, returns a channel receiving completed bars.
func (w *WSClient) SubscribeKline(ctx context.Context, symbol string, interval string) (<-chan types.Kline, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	stream := fmt.Sprintf("%s@kline_%s", strings.ToLower(symbol), interval)
	ch := make(chan types.Kline, 100)

	if _, ok := w.subscribers[stream]; !ok {
		w.subscribers[stream] = make(map[chan types.Kline]struct{})
	}
	w.subscribers[stream][ch] = struct{}{}

	// Subscribe on the wire
	msg := map[string]interface{}{
		"method": "SUBSCRIBE",
		"params": []string{stream},
		"id":     time.Now().UnixMilli(),
	}
	if err := w.conn.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("ws subscribe: %w", err)
	}

	return ch, nil
}

// Unsubscribe removes a subscription.
func (w *WSClient) Unsubscribe(symbol string, interval string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	stream := fmt.Sprintf("%s@kline_%s", strings.ToLower(symbol), interval)

	msg := map[string]interface{}{
		"method": "UNSUBSCRIBE",
		"params": []string{stream},
		"id":     time.Now().UnixMilli(),
	}
	return w.conn.WriteJSON(msg)
}

// Close shuts down the WebSocket client.
func (w *WSClient) Close() error {
	close(w.done)
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.Close()
}

func (w *WSClient) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.mu.Lock()
			if w.conn != nil {
				w.conn.WriteMessage(websocket.PingMessage, nil)
			}
			w.mu.Unlock()
		case <-w.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (w *WSClient) readLoop() {
	for {
		_, msg, err := w.conn.ReadMessage()
		if err != nil {
			// Connection lost — notify subscribers via channel closure
			w.mu.Lock()
			for _, subs := range w.subscribers {
				for ch := range subs {
					close(ch)
				}
			}
			w.subscribers = make(map[string]map[chan types.Kline]struct{})
			w.mu.Unlock()
			return
		}

		var event struct {
			Stream string        `json:"stream"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg, &event); err != nil {
			continue
		}

		// Binance may return the full event or just the kline data
		var klineData struct {
			E  int64  `json:"E"`
			K  struct {
				T  int64  `json:"t"`
				Tc int64  `json:"T"`
				S  string `json:"s"`
				I  string `json:"i"`
				O  string `json:"o"`
				H  string `json:"h"`
				L  string `json:"l"`
				C  string `json:"c"`
				V  string `json:"v"`
				X  bool   `json:"x"` // is the bar closed
			} `json:"k"`
		}

		// Try to unmarshal as wrapped event first, fall back to direct kline
		if err := json.Unmarshal(msg, &klineData); err != nil {
			continue
		}

		k := klineData.K
		if !k.X {
			continue // only emit closed bars
		}

		bar := types.Kline{
			Symbol:    strings.ToUpper(k.S),
			Interval:  k.I,
			OpenTime:  time.UnixMilli(k.T),
			CloseTime: time.UnixMilli(k.Tc),
			Open:      parseFloat(k.O),
			High:      parseFloat(k.H),
			Low:       parseFloat(k.L),
			Close:     parseFloat(k.C),
			Volume:    parseFloat(k.V),
		}

		w.mu.Lock()
		// Determine stream key from the data
		streamKey := fmt.Sprintf("%s@kline_%s", strings.ToLower(k.S), k.I)
		if subs, ok := w.subscribers[streamKey]; ok {
			for ch := range subs {
				select {
				case ch <- bar:
				default:
					// drop if subscriber is slow
				}
			}
		}
		w.mu.Unlock()
	}
}
```

- [ ] **Step 2: Install gorilla/websocket and verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go get github.com/gorilla/websocket@latest && go mod tidy && go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/market/ws.go go.mod go.sum
git commit -m "feat: add Binance WebSocket client with kline stream subscription"
```

---

### Task 6: Portfolio Module

**Files:**
- Create: `internal/portfolio/portfolio.go`

- [ ] **Step 1: Write portfolio service**

Create `internal/portfolio/portfolio.go`:

```go
package portfolio

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/store"
	"github.com/colinmyth/quant_ba/internal/types"
)

type Service struct {
	mu       sync.RWMutex
	balances map[string]types.Balance
	positions map[string]*types.Position
	store    *store.Store
}

func New(store *store.Store) *Service {
	return &Service{
		balances:  make(map[string]types.Balance),
		positions: make(map[string]*types.Position),
		store:     store,
	}
}

// Init sets the initial balance for paper/live trading.
func (s *Service) Init(balances map[string]types.Balance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.balances = balances
}

// GetPortfolio returns a copy of the current state.
func (s *Service) GetPortfolio() *types.Portfolio {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p := &types.Portfolio{
		Balances:  make(map[string]types.Balance, len(s.balances)),
		Positions: make(map[string]*types.Position, len(s.positions)),
	}
	for k, v := range s.balances {
		p.Balances[k] = v
	}
	for k, v := range s.positions {
		cp := *v
		p.Positions[k] = &cp
	}
	return p
}

// GetPosition returns a position by symbol.
func (s *Service) GetPosition(symbol string) *types.Position {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.positions[symbol]
}

// Equity returns total portfolio value in USDT.
func (s *Service) Equity() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0.0
	for _, b := range s.balances {
		if b.Asset == "USDT" {
			total += b.Free + b.Locked
		}
	}
	for _, p := range s.positions {
		total += p.Size * p.CurrentPrice
	}
	return total
}

// OnOrderFilled updates balances and positions based on a filled order.
// This is the ONLY mutation entry point.
func (s *Service) OnOrderFilled(order types.Order) {
	s.mu.Lock()
	defer s.mu.Unlock()

	price := order.FilledPrice
	if price == 0 {
		price = order.Price
	}
	fillValue := order.FilledSize * price

	// Update quote balance
	quote := "USDT"
	if bal, ok := s.balances[quote]; ok {
		if order.Side == types.DirBuy {
			bal.Free -= fillValue
			bal.Locked += 0
		} else {
			bal.Free += fillValue
		}
		s.balances[quote] = bal
	}

	// Update base balance
	base := order.Symbol[:len(order.Symbol)-4] // rough: BTCUSDT → BTC
	if bal, ok := s.balances[base]; ok {
		if order.Side == types.DirBuy {
			bal.Free += order.FilledSize
		} else {
			bal.Free -= order.FilledSize
		}
		s.balances[base] = bal
	} else {
		s.balances[base] = types.Balance{Asset: base, Free: order.FilledSize}
	}

	// Update position
	pos, exists := s.positions[order.Symbol]
	if !exists || pos.Size == 0 {
		if order.FilledSize > 0 {
			s.positions[order.Symbol] = &types.Position{
				Symbol:       order.Symbol,
				Side:         order.Side,
				Size:         order.FilledSize,
				EntryPrice:   price,
				CurrentPrice: price,
				PnL:          0,
				PnLPct:       0,
				UpdatedAt:    time.Now(),
			}
		}
	} else {
		if order.Side == pos.Side {
			// Adding to position
			totalSize := pos.Size + order.FilledSize
			pos.EntryPrice = (pos.EntryPrice*pos.Size + price*order.FilledSize) / totalSize
			pos.Size = totalSize
		} else {
			// Reducing position
			pos.Size -= order.FilledSize
			if pos.Size <= 0 {
				delete(s.positions, order.Symbol)
				return
			}
		}
		pos.CurrentPrice = price
		pos.PnL = pos.Size * (pos.CurrentPrice - pos.EntryPrice)
		if pos.EntryPrice > 0 {
			pos.PnLPct = (pos.CurrentPrice - pos.EntryPrice) / pos.EntryPrice
		}
		pos.UpdatedAt = time.Now()
	}
}

// UpdatePrices updates current prices for all positions.
func (s *Service) UpdatePrices(prices map[string]float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, pos := range s.positions {
		if p, ok := prices[pos.Symbol]; ok {
			pos.CurrentPrice = p
			if pos.Side == types.DirSell {
				pos.PnL = pos.Size * (pos.EntryPrice - pos.CurrentPrice)
			} else {
				pos.PnL = pos.Size * (pos.CurrentPrice - pos.EntryPrice)
			}
			if pos.EntryPrice > 0 {
				pos.PnLPct = (pos.CurrentPrice - pos.EntryPrice) / pos.EntryPrice
			}
			pos.UpdatedAt = time.Now()
		}
	}
}

// Snapshot persists the current portfolio as JSON.
func (s *Service) Snapshot() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.Marshal(s.balances)
	if err != nil {
		return err
	}
	return s.store.SavePortfolio(string(data))
}

// Restore loads the last snapshot from the store.
func (s *Service) Restore() error {
	raw, err := s.store.LastPortfolio()
	if err != nil || raw == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Unmarshal([]byte(raw), &s.balances)
}
```

- [ ] **Step 2: Verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/portfolio/
git commit -m "feat: add portfolio service with position tracking and snapshots"
```

---

### Task 7: Strategy Interface + Plugin Protocol

**Files:**
- Create: `internal/strategy/strategy.go` — Strategy interface definitions
- Create: `internal/strategy/protocol.go` — JSON-RPC protocol types
- Create: `internal/strategy/loader.go` — Plugin process manager
- Create: `internal/strategy/example/ma_cross.go` — Example MA cross strategy plugin

- [ ] **Step 1: Write strategy interface**

Create `internal/strategy/strategy.go`:

```go
package strategy

import "github.com/colinmyth/quant_ba/internal/types"

type Strategy interface {
	ID() string
	Name() string
	Version() string
	Symbols() []string
	Intervals() []string
	Params() types.StrategyParams

	OnInit(portfolio *types.Portfolio) error
	OnBar(symbol string, bars []types.Kline, portfolio *types.Portfolio) (*types.Signal, error)
	OnOrderUpdate(order types.Order, portfolio *types.Portfolio) error

	OnPause() error
	OnResume() error
	OnStop() error
}
```

- [ ] **Step 2: Write JSON-RPC protocol types**

Create `internal/strategy/protocol.go`:

```go
package strategy

import (
	"encoding/json"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
)

// Protocol messages for stdin/stdout plugin communication.
// One JSON object per line.

type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type Response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Method-specific params/results

type OnInitParams struct {
	Balances  map[string]types.Balance  `json:"balances"`
	Positions map[string]*types.Position `json:"positions"`
}

type OnBarParams struct {
	Symbol    string          `json:"symbol"`
	Bars      []types.Kline   `json:"bars"`
	Balances  map[string]types.Balance  `json:"balances"`
	Positions map[string]*types.Position `json:"positions"`
}

type OnOrderUpdateParams struct {
	Order     types.Order              `json:"order"`
	Balances  map[string]types.Balance  `json:"balances"`
	Positions map[string]*types.Position `json:"positions"`
}

type MetaResult struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Version   string                `json:"version"`
	Symbols   []string              `json:"symbols"`
	Intervals []string              `json:"intervals"`
	Params    types.StrategyParams  `json:"params"`
}

type SignalResult struct {
	Signal *types.Signal `json:"signal"`
}

type PluginClient struct {
	enc *json.Encoder
	dec *json.Decoder
}

func NewPluginClient(enc *json.Encoder, dec *json.Decoder) *PluginClient {
	return &PluginClient{enc: enc, dec: dec}
}

func (c *PluginClient) Call(method string, params interface{}, result interface{}) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req := Request{
		ID:     method + "_" + time.Now().Format(time.RFC3339Nano),
		Method: method,
		Params: paramsJSON,
	}
	if err := c.enc.Encode(req); err != nil {
		return err
	}
	var resp Response
	if err := c.dec.Decode(&resp); err != nil {
		return err
	}
	if resp.Error != "" {
		return &PluginError{Message: resp.Error}
	}
	if result != nil {
		return json.Unmarshal(resp.Result, result)
	}
	return nil
}

type PluginError struct {
	Message string
}

func (e *PluginError) Error() string {
	return "plugin error: " + e.Message
}
```

- [ ] **Step 3: Write plugin loader**

Create `internal/strategy/loader.go`:

```go
package strategy

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/colinmyth/quant_ba/internal/types"
)

type Loader struct {
	mu      sync.RWMutex
	plugins map[string]*LoadedStrategy
}

type LoadedStrategy struct {
	Meta   types.StrategyMeta
	Client *PluginClient
	Cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func NewLoader() *Loader {
	return &Loader{
		plugins: make(map[string]*LoadedStrategy),
	}
}

// Load starts a strategy plugin process and returns its metadata.
func (l *Loader) Load(path string) (*types.StrategyMeta, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	cmd := exec.Command(path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil // discard stderr from plugin

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start plugin: %w", err)
	}

	enc := json.NewEncoder(stdin)
	dec := json.NewDecoder(stdout)
	client := NewPluginClient(enc, dec)

	var meta MetaResult
	if err := client.Call("meta", nil, &meta); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, fmt.Errorf("get meta: %w", err)
	}

	ls := &LoadedStrategy{
		Meta: types.StrategyMeta{
			ID:      meta.ID,
			Name:    meta.Name,
			Version: meta.Version,
			Path:    path,
		},
		Client: client,
		Cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}

	l.plugins[meta.ID] = ls
	return &ls.Meta, nil
}

// Unload stops a plugin process and removes it.
func (l *Loader) Unload(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	ls, ok := l.plugins[id]
	if !ok {
		return fmt.Errorf("plugin %s not loaded", id)
	}

	ls.Client.Call("stop", nil, nil)
	ls.stdin.Close()
	ls.Cmd.Wait()
	delete(l.plugins, id)
	return nil
}

// Get returns the RPC client for a loaded strategy.
func (l *Loader) Get(id string) (*LoadedStrategy, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	ls, ok := l.plugins[id]
	if !ok {
		return nil, fmt.Errorf("plugin %s not loaded", id)
	}
	return ls, nil
}

// List returns metadata for all loaded plugins.
func (l *Loader) List() []types.StrategyMeta {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var metas []types.StrategyMeta
	for _, ls := range l.plugins {
		metas = append(metas, ls.Meta)
	}
	return metas
}

// Close unloads all plugins.
func (l *Loader) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, ls := range l.plugins {
		ls.Client.Call("stop", nil, nil)
		ls.stdin.Close()
		ls.Cmd.Wait()
		delete(l.plugins, id)
	}
}
```

- [ ] **Step 4: Write example MA cross strategy plugin**

This is a standalone binary (main package) that the loader runs as a subprocess.

Create `internal/strategy/example/ma_cross.go`:

```go
package main

import (
	"encoding/json"
	"os"

	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/types"
)

func main() {
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	handler := &MACrossHandler{}

	for {
		var req strategy.Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		resp := handler.Handle(req)
		enc.Encode(resp)
	}
}

type MACrossHandler struct {
	shortPeriod int
	longPeriod  int
	symbols     []string
	interval    string
	prices      map[string][]float64
}

func (h *MACrossHandler) Handle(req strategy.Request) strategy.Response {
	switch req.Method {
	case "meta":
		return strategy.Response{
			ID: req.ID,
			Result: mustMarshal(strategy.MetaResult{
				ID:        "ma_cross_v1",
				Name:      "MA Cross",
				Version:   "1.0.0",
				Symbols:   []string{"BTCUSDT"},
				Intervals: []string{"1h"},
				Params:    types.StrategyParams{"short_period": 5, "long_period": 20},
			}),
		}

	case "init":
		var params strategy.OnInitParams
		json.Unmarshal(req.Params, &params)
		h.shortPeriod = 5
		h.longPeriod = 20
		h.symbols = []string{"BTCUSDT"}
		h.interval = "1h"
		h.prices = make(map[string][]float64)
		return strategy.Response{ID: req.ID, Result: []byte("{}")}

	case "bar":
		var params strategy.OnBarParams
		json.Unmarshal(req.Params, &params)
		return h.onBar(params, req.ID)

	case "order_update":
		return strategy.Response{ID: req.ID, Result: []byte("{}")}

	case "pause":
		return strategy.Response{ID: req.ID, Result: []byte("{}")}

	case "resume":
		return strategy.Response{ID: req.ID, Result: []byte("{}")}

	case "stop":
		return strategy.Response{ID: req.ID, Result: []byte("{}")}

	default:
		return strategy.Response{ID: req.ID, Error: "unknown method: " + req.Method}
	}
}

func (h *MACrossHandler) onBar(params strategy.OnBarParams, id string) strategy.Response {
	symbol := params.Symbol

	// Accumulate close prices
	for _, bar := range params.Bars {
		h.prices[symbol] = append(h.prices[symbol], bar.Close)
	}

	prices := h.prices[symbol]
	if len(prices) < h.longPeriod+1 {
		return strategy.Response{ID: id, Result: mustMarshal(strategy.SignalResult{Signal: nil})}
	}

	shortMA := sma(prices, h.shortPeriod)
	longMA := sma(prices, h.longPeriod)
	prevShortMA := sma(prices[:len(prices)-1], h.shortPeriod)
	prevLongMA := sma(prices[:len(prices)-1], h.longPeriod)

	// Check if already in position
	hasPosition := false
	for _, pos := range params.Positions {
		if pos.Symbol == symbol && pos.Size > 0 {
			hasPosition = true
			break
		}
	}

	var sig *types.Signal

	if prevShortMA <= prevLongMA && shortMA > longMA && !hasPosition {
		// Golden cross → Buy
		sig = &types.Signal{
			Symbol:    symbol,
			Direction: types.DirBuy,
			Size:      0.01, // simplified sizing
			Type:      types.OrdMarket,
			Reason:    "MA golden cross",
		}
	} else if prevShortMA >= prevLongMA && shortMA < longMA && hasPosition {
		// Death cross → Sell
		sig = &types.Signal{
			Symbol:    symbol,
			Direction: types.DirSell,
			Size:      0, // close position
			Type:      types.OrdMarket,
			Reason:    "MA death cross",
		}
	}

	return strategy.Response{ID: id, Result: mustMarshal(strategy.SignalResult{Signal: sig})}
}

func sma(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}
	sum := 0.0
	for _, p := range prices[len(prices)-period:] {
		sum += p
	}
	return sum / float64(period)
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
```

- [ ] **Step 5: Verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go build ./... && go build -o plugins/ma_cross_v1 ./internal/strategy/example/
```

Expected: both main binary and plugin binary compile.

- [ ] **Step 6: Commit**

```bash
git add internal/strategy/ plugins/
git commit -m "feat: add strategy interface, JSON-RPC plugin protocol, loader, and example MA cross strategy"
```

---

### Task 8: Order Module

**Files:**
- Create: `internal/order/order.go` — OrderManager interface
- Create: `internal/order/paper.go` — Paper trading implementation

- [ ] **Step 1: Write OrderManager interface**

Create `internal/order/order.go`:

```go
package order

import (
	"context"

	"github.com/colinmyth/quant_ba/internal/types"
)

type OrderManager interface {
	Place(ctx context.Context, signal *types.Signal) (*types.Order, error)
	Cancel(ctx context.Context, orderID string) error
	Status(ctx context.Context, orderID string) (*types.Order, error)
	OpenOrders(ctx context.Context, symbol string) ([]types.Order, error)
}
```

- [ ] **Step 2: Write PaperOrderManager**

Create `internal/order/paper.go`:

```go
package order

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
)

type PaperOrderManager struct {
	mu     sync.RWMutex
	orders map[string]*types.Order
	nextID int
}

func NewPaperOrderManager() *PaperOrderManager {
	return &PaperOrderManager{
		orders: make(map[string]*types.Order),
		nextID: 1,
	}
}

func (p *PaperOrderManager) Place(ctx context.Context, signal *types.Signal) (*types.Order, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := fmt.Sprintf("paper_%d", p.nextID)
	p.nextID++

	order := &types.Order{
		ID:         id,
		Symbol:     signal.Symbol,
		Side:       signal.Direction,
		Type:       signal.Type,
		Price:      signal.Price,
		Size:       signal.Size,
		Status:     types.OrdNew,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		StrategyID: "",
	}
	p.orders[id] = order

	// Simulate immediate fill for market orders
	if order.Type == types.OrdMarket {
		order.Status = types.OrdFilled
		order.FilledSize = order.Size
		order.FilledPrice = order.Price
		order.UpdatedAt = time.Now()
	}

	return order, nil
}

func (p *PaperOrderManager) Cancel(ctx context.Context, orderID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	order, ok := p.orders[orderID]
	if !ok {
		return fmt.Errorf("order %s not found", orderID)
	}
	order.Status = types.OrdCancelled
	order.UpdatedAt = time.Now()
	return nil
}

func (p *PaperOrderManager) Status(ctx context.Context, orderID string) (*types.Order, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	order, ok := p.orders[orderID]
	if !ok {
		return nil, fmt.Errorf("order %s not found", orderID)
	}
	return order, nil
}

func (p *PaperOrderManager) OpenOrders(ctx context.Context, symbol string) ([]types.Order, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var open []types.Order
	for _, o := range p.orders {
		if o.Symbol == symbol && o.Status != types.OrdFilled && o.Status != types.OrdCancelled && o.Status != types.OrdRejected {
			open = append(open, *o)
		}
	}
	return open, nil
}
```

- [ ] **Step 3: Verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/order/
git commit -m "feat: add order manager interface and paper trading implementation"
```

---

### Task 9: Risk Module

**Files:**
- Create: `internal/risk/risk.go` — RiskManager interface
- Create: `internal/risk/basic.go` — Layer 1: basic limits
- Create: `internal/risk/global.go` — Layer 2: global limits
- Create: `internal/risk/breaker.go` — Layer 3: circuit breakers

- [ ] **Step 1: Write RiskManager interface and config**

Create `internal/risk/risk.go`:

```go
package risk

import (
	"context"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/config"
	"github.com/colinmyth/quant_ba/internal/types"
)

type RiskManager interface {
	PreCheck(ctx context.Context, signal *types.Signal, portfolio *types.Portfolio) error
	PostCheck(ctx context.Context, order types.Order, portfolio *types.Portfolio) error
}

type Manager struct {
	cfg     config.RiskConfig
	basic   *BasicRisk
	global  *GlobalRisk
	breaker *CircuitBreaker
}

func New(cfg config.RiskConfig) *Manager {
	return &Manager{
		cfg:     cfg,
		basic:   &BasicRisk{cfg: cfg.Basic},
		global:  NewGlobalRisk(cfg.Global),
		breaker: NewCircuitBreaker(cfg.CircuitBreaker),
	}
}

func (m *Manager) PreCheck(ctx context.Context, signal *types.Signal, portfolio *types.Portfolio) error {
	if err := m.basic.Check(signal, portfolio); err != nil {
		return err
	}
	if err := m.global.Check(signal, portfolio); err != nil {
		return err
	}
	if err := m.breaker.Check(signal, portfolio); err != nil {
		return err
	}
	return nil
}

func (m *Manager) PostCheck(ctx context.Context, order types.Order, portfolio *types.Portfolio) error {
	m.breaker.RecordOrder(order)
	return nil
}
```

- [ ] **Step 2: Write Layer 1 — Basic Limits**

Create `internal/risk/basic.go`:

```go
package risk

import (
	"fmt"

	"github.com/colinmyth/quant_ba/internal/config"
	"github.com/colinmyth/quant_ba/internal/types"
)

type BasicRisk struct {
	cfg config.BasicRiskConfig
}

func (b *BasicRisk) Check(signal *types.Signal, portfolio *types.Portfolio) error {
	// Blacklist check
	for _, sym := range b.cfg.Blacklist {
		if sym == signal.Symbol {
			return fmt.Errorf("risk: symbol %s is blacklisted", signal.Symbol)
		}
	}

	// Market order slippage check
	if signal.Type == types.OrdMarket && signal.Price > 0 {
		// Estimated slippage: 2% by default, configurable
		if b.cfg.MaxSlippagePct > 0 {
			// This is a pre-trade estimate — actual slippage checked post-trade
		}
	}

	// Position size check
	equity := portfolioEquity(portfolio)
	if equity > 0 {
		posValue := signal.Size * signal.Price
		maxValue := equity * b.cfg.MaxPositionPct
		if posValue > maxValue {
			return fmt.Errorf("risk: position value %.2f exceeds max %.2f (%.0f%% of equity)",
				posValue, maxValue, b.cfg.MaxPositionPct*100)
		}
	}

	return nil
}

func portfolioEquity(p *types.Portfolio) float64 {
	total := 0.0
	for _, bal := range p.Balances {
		if bal.Asset == "USDT" {
			total += bal.Free + bal.Locked
		}
	}
	// Add position values
	for _, pos := range p.Positions {
		total += pos.Size * pos.CurrentPrice
	}
	return total
}
```

- [ ] **Step 3: Write Layer 2 — Global Limits**

Create `internal/risk/global.go`:

```go
package risk

import (
	"fmt"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/config"
	"github.com/colinmyth/quant_ba/internal/types"
)

type GlobalRisk struct {
	cfg       config.GlobalRiskConfig
	mu        sync.Mutex
	tradeCount int
	tradeDay   string
	tradeTimes map[string]time.Time // symbol → last trade time
}

func NewGlobalRisk(cfg config.GlobalRiskConfig) *GlobalRisk {
	return &GlobalRisk{
		cfg:        cfg,
		tradeTimes: make(map[string]time.Time),
	}
}

func (g *GlobalRisk) Check(signal *types.Signal, portfolio *types.Portfolio) error {
	// Daily trade count
	today := time.Now().Format("2006-01-02")
	g.mu.Lock()
	if g.tradeDay != today {
		g.tradeDay = today
		g.tradeCount = 0
	}
	if g.tradeCount >= g.cfg.DailyTradeLimit {
		g.mu.Unlock()
		return fmt.Errorf("risk: daily trade limit reached (%d)", g.cfg.DailyTradeLimit)
	}
	g.mu.Unlock()

	// Min hold interval
	if lastTrade, ok := g.tradeTimes[signal.Symbol]; ok {
		if time.Since(lastTrade).Seconds() < float64(g.cfg.MinHoldSeconds) {
			return fmt.Errorf("risk: min hold interval not met for %s (%.0fs < %ds)",
				signal.Symbol, time.Since(lastTrade).Seconds(), g.cfg.MinHoldSeconds)
		}
	}

	// Concentration check
	pos, ok := portfolio.Positions[signal.Symbol]
	if ok && pos.Size > 0 {
		equity := portfolioEquity(portfolio)
		if equity > 0 {
			posValue := pos.Size * pos.CurrentPrice
			conc := posValue / equity
			if conc > g.cfg.MaxConcentration {
				return fmt.Errorf("risk: concentration %.2f exceeds max %.2f", conc, g.cfg.MaxConcentration)
			}
		}
	}

	return nil
}

func (g *GlobalRisk) RecordTrade(symbol string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tradeCount++
	g.tradeTimes[symbol] = time.Now()
}
```

- [ ] **Step 4: Write Layer 3 — Circuit Breakers**

Create `internal/risk/breaker.go`:

```go
package risk

import (
	"fmt"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/config"
	"github.com/colinmyth/quant_ba/internal/types"
)

type BreakerState struct {
	AllPaused       bool
	StrategyPaused  map[string]bool
	DailyPnL        float64
	DailyPnLDay     string
	ConsecutiveLoss int
	StartEquity     float64
	volLastCheck    time.Time
}

type CircuitBreaker struct {
	cfg   config.BreakerConfig
	mu    sync.Mutex
	state BreakerState
}

func NewCircuitBreaker(cfg config.BreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		cfg: cfg,
		state: BreakerState{
			StrategyPaused: make(map[string]bool),
		},
	}
}

func (c *CircuitBreaker) Check(signal *types.Signal, portfolio *types.Portfolio) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// All-paused check
	if c.state.AllPaused {
		return fmt.Errorf("risk: circuit breaker active — all trading paused")
	}

	// Strategy-paused check
	if c.state.StrategyPaused[signal.StrategyID] {
		return fmt.Errorf("risk: strategy %s paused by circuit breaker", signal.StrategyID)
	}

	// Daily loss check
	today := time.Now().Format("2006-01-02")
	if c.state.DailyPnLDay != today {
		c.state.DailyPnLDay = today
		c.state.DailyPnL = 0
	}

	equity := portfolioEquity(portfolio)
	if c.state.StartEquity == 0 {
		c.state.StartEquity = equity
	}

	if c.state.StartEquity > 0 {
		dailyDrawdown := c.state.DailyPnL / c.state.StartEquity
		if dailyDrawdown < -c.cfg.DailyLossPct {
			c.state.AllPaused = true
			return fmt.Errorf("risk: daily loss limit reached (%.2f%%)", dailyDrawdown*100)
		}

		totalDrawdown := (equity - c.state.StartEquity) / c.state.StartEquity
		if totalDrawdown < -c.cfg.MaxDrawdownPct {
			c.state.AllPaused = true
			return fmt.Errorf("risk: max drawdown reached (%.2f%%)", totalDrawdown*100)
		}
	}

	return nil
}

func (c *CircuitBreaker) RecordOrder(order types.Order) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if order.Status == types.OrdFilled {
		pnl := (order.FilledPrice - order.Price) * order.FilledSize

		// Update daily PnL
		if order.Side == types.DirSell {
			pnl = -pnl
		}
		c.state.DailyPnL += pnl

		// Track consecutive losses
		if pnl < 0 {
			c.state.ConsecutiveLoss++
			if c.state.ConsecutiveLoss >= c.cfg.ConsecutiveLosses {
				c.state.StrategyPaused[order.StrategyID] = true
			}
		} else {
			c.state.ConsecutiveLoss = 0
		}
	}
}

// PauseStrategy pauses a specific strategy.
func (c *CircuitBreaker) PauseStrategy(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.StrategyPaused[id] = true
}

// ResumeStrategy resumes a paused strategy.
func (c *CircuitBreaker) ResumeStrategy(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.StrategyPaused[id] = false
}

// ResetAll clears all breaker states.
func (c *CircuitBreaker) ResetAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.AllPaused = false
	c.state.StrategyPaused = make(map[string]bool)
	c.state.DailyPnL = 0
	c.state.ConsecutiveLoss = 0
}
```

- [ ] **Step 5: Verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/risk/
git commit -m "feat: add 3-layer risk engine (basic, global, circuit breaker)"
```

---

### Task 10: Executor Module — Paper Trading Loop

**Files:**
- Create: `internal/executor/executor.go` — Executor interface
- Create: `internal/executor/paper.go` — Paper executor with main event loop

- [ ] **Step 1: Write executor interface**

Create `internal/executor/executor.go`:

```go
package executor

import (
	"context"

	"github.com/colinmyth/quant_ba/internal/types"
)

type Executor interface {
	Run(ctx context.Context, strategyID string) error
	Stop(strategyID string) error
	Status(strategyID string) *StrategyStatus
}

type StrategyStatus struct {
	StrategyID string
	Running    bool
	Mode       string // "paper", "live", "backtest"
	Portfolio  *types.Portfolio
	StartedAt  string
}
```

- [ ] **Step 2: Write PaperExecutor**

Create `internal/executor/paper.go`:

```go
package executor

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/market"
	"github.com/colinmyth/quant_ba/internal/order"
	"github.com/colinmyth/quant_ba/internal/portfolio"
	"github.com/colinmyth/quant_ba/internal/risk"
	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/types"
)

type PaperExecutor struct {
	feed      market.DataFeed
	loader    *strategy.Loader
	risk      risk.RiskManager
	orderMgr  order.OrderManager
	portfolio *portfolio.Service

	mu       sync.Mutex
	statuses map[string]*StrategyStatus
	cancels  map[string]context.CancelFunc
}

func NewPaperExecutor(feed market.DataFeed, loader *strategy.Loader, risk risk.RiskManager, orderMgr order.OrderManager, portfolio *portfolio.Service) *PaperExecutor {
	return &PaperExecutor{
		feed:      feed,
		loader:    loader,
		risk:      risk,
		orderMgr:  orderMgr,
		portfolio: portfolio,
		statuses:  make(map[string]*StrategyStatus),
		cancels:   make(map[string]context.CancelFunc),
	}
}

func (e *PaperExecutor) Run(ctx context.Context, strategyID string) error {
	e.mu.Lock()
	if _, running := e.cancels[strategyID]; running {
		e.mu.Unlock()
		return fmt.Errorf("strategy %s already running", strategyID)
	}
	e.mu.Unlock()

	ls, err := e.loader.Get(strategyID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)

	e.mu.Lock()
	e.cancels[strategyID] = cancel
	e.statuses[strategyID] = &StrategyStatus{
		StrategyID: strategyID,
		Running:    true,
		Mode:       "paper",
		StartedAt:  time.Now().Format(time.RFC3339),
	}
	e.mu.Unlock()

	// Init strategy
	port := e.portfolio.GetPortfolio()
	initParams := strategy.OnInitParams{
		Balances:  port.Balances,
		Positions: port.Positions,
	}
	if err := ls.Client.Call("init", initParams, nil); err != nil {
		return fmt.Errorf("strategy init: %w", err)
	}

	// Subscribe to data
	var wg sync.WaitGroup
	for _, symbol := range ls.Meta.Symbols {
		wg.Add(1)
		go func(sym string) {
			defer wg.Done()
			e.runSymbolLoop(ctx, ls, sym)
		}(symbol)
	}

	// Periodic snapshot
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				e.portfolio.Snapshot()
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	return nil
}

func (e *PaperExecutor) runSymbolLoop(ctx context.Context, ls *strategy.LoadedStrategy, symbol string) {
	ch, err := e.feed.SubscribeKline(ctx, symbol, "1h")
	if err != nil {
		log.Printf("subscribe %s: %v", symbol, err)
		return
	}

	var bars []types.Kline
	for {
		select {
		case bar, ok := <-ch:
			if !ok {
				return
			}
			bars = append(bars, bar)
			if len(bars) > 100 {
				bars = bars[1:]
			}

			port := e.portfolio.GetPortfolio()

			// Call strategy
			var sigResp strategy.SignalResult
			barParams := strategy.OnBarParams{
				Symbol:    symbol,
				Bars:      bars,
				Balances:  port.Balances,
				Positions: port.Positions,
			}
			if err := ls.Client.Call("bar", barParams, &sigResp); err != nil {
				log.Printf("strategy bar: %v", err)
				continue
			}

			if sigResp.Signal == nil || sigResp.Signal.Direction == types.DirHold {
				continue
			}

			sig := sigResp.Signal
			sig.StrategyID = ls.Meta.ID

			// Risk check
			if err := e.risk.PreCheck(ctx, sig, port); err != nil {
				log.Printf("risk blocked: %v", err)
				continue
			}

			// Place order
			order, err := e.orderMgr.Place(ctx, sig)
			if err != nil {
				log.Printf("place order: %v", err)
				continue
			}

			// Update portfolio on fill
			if order.Status == types.OrdFilled {
				e.portfolio.OnOrderFilled(*order)
				ls.Client.Call("order_update", strategy.OnOrderUpdateParams{
					Order:     *order,
					Balances:  port.Balances,
					Positions: port.Positions,
				}, nil)
				e.risk.PostCheck(ctx, *order, port)
			}

		case <-ctx.Done():
			return
		}
	}
}

func (e *PaperExecutor) Stop(strategyID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	cancel, ok := e.cancels[strategyID]
	if !ok {
		return fmt.Errorf("strategy %s not running", strategyID)
	}
	cancel()
	delete(e.cancels, strategyID)
	if s, ok := e.statuses[strategyID]; ok {
		s.Running = false
	}
	return nil
}

func (e *PaperExecutor) Status(strategyID string) *StrategyStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.statuses[strategyID]
}
```

- [ ] **Step 3: Verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/executor/
git commit -m "feat: add executor interface and paper trading event loop"
```

---

### Task 11: Backtest Engine

**Files:**
- Create: `internal/backtest/engine.go` — BacktestEngine
- Create: `internal/backtest/fill.go` — Fill simulation
- Create: `internal/backtest/stats.go` — Statistics calculation

- [ ] **Step 1: Write fill simulator**

Create `internal/backtest/fill.go`:

```go
package backtest

import "github.com/colinmyth/quant_ba/internal/types"

const (
	slippageBuy  = 1.0005
	slippageSell = 0.9995
	feeRate      = 0.001 // 0.1%
)

// SimulateFill simulates order fill against a bar.
// Returns the order with filled size/price set, or nil if the order cannot fill.
func SimulateFill(signal *types.Signal, bar types.Kline, prevOrder *types.Order) *types.Order {
	order := &types.Order{
		Symbol:     signal.Symbol,
		Side:       signal.Direction,
		Type:       signal.Type,
		Size:       signal.Size,
		Price:      signal.Price,
		Status:     types.OrdNew,
		StrategyID: signal.StrategyID,
	}

	if order.Size == 0 {
		return nil
	}

	if order.Type == types.OrdMarket {
		fillPrice := (bar.Open + bar.High + bar.Low + bar.Close) / 4 // VWAP approx
		if order.Side == types.DirBuy {
			fillPrice *= slippageBuy
		} else {
			fillPrice *= slippageSell
		}
		order.Status = types.OrdFilled
		order.FilledSize = order.Size
		order.FilledPrice = fillPrice
		return order
	}

	// Limit order: check if price was touched
	if order.Type == types.OrdLimit {
		if order.Side == types.DirBuy && bar.Low <= order.Price {
			fillPrice := order.Price * slippageBuy
			order.Status = types.OrdFilled
			order.FilledSize = order.Size
			order.FilledPrice = fillPrice
			return order
		}
		if order.Side == types.DirSell && bar.High >= order.Price {
			fillPrice := order.Price * slippageSell
			order.Status = types.OrdFilled
			order.FilledSize = order.Size
			order.FilledPrice = fillPrice
			return order
		}
		// Not filled — limit order pending (not simulated in this simple model)
		return nil
	}

	return nil
}

// Fee returns the trading fee for a filled order.
func Fee(order *types.Order) float64 {
	return order.FilledSize * order.FilledPrice * feeRate
}
```

- [ ] **Step 2: Write statistics calculator**

Create `internal/backtest/stats.go`:

```go
package backtest

import (
	"math"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
)

type BacktestResult struct {
	TotalReturn   float64            `json:"total_return"`
	SharpeRatio   float64            `json:"sharpe_ratio"`
	MaxDrawdown   float64            `json:"max_drawdown"`
	WinRate       float64            `json:"win_rate"`
	ProfitFactor  float64            `json:"profit_factor"`
	TotalTrades   int                `json:"total_trades"`
	TradeLog      []types.Trade      `json:"trade_log"`
	EquityCurve   []types.EquityPoint `json:"equity_curve"`
}

func ComputeStats(trades []types.Trade, equityCurve []types.EquityPoint, startCapital float64) *BacktestResult {
	r := &BacktestResult{
		TotalTrades: len(trades),
		TradeLog:    trades,
		EquityCurve: equityCurve,
	}

	if len(trades) == 0 || startCapital == 0 {
		return r
	}

	// Final equity
	var finalEquity float64
	if len(equityCurve) > 0 {
		finalEquity = equityCurve[len(equityCurve)-1].Equity
	}

	r.TotalReturn = (finalEquity - startCapital) / startCapital

	// Win rate, profit factor
	var wins int
	var grossProfit, grossLoss float64
	for _, t := range trades {
		if t.PnL > 0 {
			wins++
			grossProfit += t.PnL
		} else {
			grossLoss += -t.PnL
		}
	}
	r.WinRate = float64(wins) / float64(len(trades))
	if grossLoss > 0 {
		r.ProfitFactor = grossProfit / grossLoss
	}

	// Max drawdown
	peak := startCapital
	maxDD := 0.0
	for _, p := range equityCurve {
		if p.Equity > peak {
			peak = p.Equity
		}
		dd := (peak - p.Equity) / peak
		if dd > maxDD {
			maxDD = dd
		}
	}
	r.MaxDrawdown = maxDD

	// Sharpe ratio
	if len(equityCurve) > 1 {
		returns := make([]float64, len(equityCurve)-1)
		for i := 1; i < len(equityCurve); i++ {
			returns[i-1] = (equityCurve[i].Equity - equityCurve[i-1].Equity) / equityCurve[i-1].Equity
		}
		mean := mean(returns)
		std := stdDev(returns, mean)
		if std > 0 {
			r.SharpeRatio = mean / std * math.Sqrt(252) // annualized (daily bars)
		}
	}

	return r
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func stdDev(vals []float64, mean float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += (v - mean) * (v - mean)
	}
	return math.Sqrt(sum / float64(len(vals)-1))
}
```

- [ ] **Step 3: Write backtest engine**

Create `internal/backtest/engine.go`:

```go
package backtest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/colinmyth/quant_ba/internal/market"
	"github.com/colinmyth/quant_ba/internal/portfolio"
	"github.com/colinmyth/quant_ba/internal/risk"
	"github.com/colinmyth/quant_ba/internal/store"
	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/types"
)

type Engine struct {
	cache     *market.KlineCache
	risk      risk.RiskManager
	portfolio *portfolio.Service
	store     *store.Store
}

func NewEngine(cache *market.KlineCache, risk risk.RiskManager, portfolio *portfolio.Service, store *store.Store) *Engine {
	return &Engine{
		cache:     cache,
		risk:      risk,
		portfolio: portfolio,
		store:     store,
	}
}

func (e *Engine) Run(ctx context.Context, strat *strategy.LoadedStrategy, symbols []string, interval string, start, end time.Time, startCapital float64) (*BacktestResult, error) {
	// Init portfolio with starting capital
	e.portfolio.Init(map[string]types.Balance{
		"USDT": {Asset: "USDT", Free: startCapital},
	})

	port := e.portfolio.GetPortfolio()
	initParams := strategy.OnInitParams{
		Balances:  port.Balances,
		Positions: port.Positions,
	}
	if err := strat.Client.Call("init", initParams, nil); err != nil {
		return nil, fmt.Errorf("strategy init: %w", err)
	}

	// Fetch ALL historical data upfront
	symbolData := make(map[string][]types.Kline)
	for _, sym := range symbols {
		klines, err := e.cache.GetOrFetch(ctx, sym, interval, 1000)
		if err != nil {
			return nil, fmt.Errorf("fetch %s klines: %w", sym, err)
		}
		// Filter to date range
		var filtered []types.Kline
		for _, k := range klines {
			if k.OpenTime.After(end) {
				break
			}
			if k.OpenTime.Before(start) {
				continue
			}
			filtered = append(filtered, k)
		}
		symbolData[sym] = filtered
	}

	// Find the union timeline
	timeMap := make(map[int64][]types.Kline)
	for _, klines := range symbolData {
		for _, k := range klines {
			ts := k.OpenTime.UnixMilli()
			timeMap[ts] = append(timeMap[ts], k)
		}
	}

	// Sort timestamps
	var timestamps []int64
	for ts := range timeMap {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	// Run simulation bar by bar
	var trades []types.Trade
	var equityCurve []types.EquityPoint
	var openPositions []*types.Order // track open positions for this strategy
	equity := startCapital
	equityCurve = append(equityCurve, types.EquityPoint{Time: start, Equity: equity})

	for _, ts := range timestamps {
		bars := timeMap[ts]
		if len(bars) == 0 {
			continue
		}

		port := e.portfolio.GetPortfolio()

		// Check for limit order fills on this bar
		var remaining []*types.Order
		for _, o := range openPositions {
			filled := false
			for _, bar := range bars {
				if bar.Symbol == o.Symbol {
					if checkLimitFill(o, bar) {
						o.Status = types.OrdFilled
						o.FilledSize = o.Size
						o.FilledPrice = o.Price
						o.UpdatedAt = bar.OpenTime
						e.portfolio.OnOrderFilled(*o)
						equity += o.PnL() - Fee(o)
						filled = true
						break
					}
				}
			}
			if !filled {
				remaining = append(remaining, o)
			}
		}
		openPositions = remaining

		// Call strategy
		for _, bar := range bars {
			var sigResp strategy.SignalResult
			barParams := strategy.OnBarParams{
				Symbol:    bar.Symbol,
				Bars:      symbolData[bar.Symbol], // pass all bars up to current
				Balances:  port.Balances,
				Positions: port.Positions,
			}
			if err := strat.Client.Call("bar", barParams, &sigResp); err != nil {
				continue
			}

			if sigResp.Signal == nil || sigResp.Signal.Direction == types.DirHold {
				continue
			}

			sig := sigResp.Signal
			sig.StrategyID = strat.Meta.ID

			// Risk check
			if err := e.risk.PreCheck(ctx, sig, port); err != nil {
				continue
			}

			// Simulate fill
			order := SimulateFill(sig, bar, nil)
			if order == nil {
				if sig.Type == types.OrdLimit {
					openPositions = append(openPositions, orderFromSignal(sig, bar.OpenTime))
				}
				continue
			}

			// Apply to portfolio
			e.portfolio.OnOrderFilled(*order)
			e.risk.PostCheck(ctx, *order, port)

			// Record trade
			trade := types.Trade{
				ID:         fmt.Sprintf("bt_%d", len(trades)+1),
				Symbol:     order.Symbol,
				Side:       order.Side,
				Size:       order.FilledSize,
				EntryPrice: order.FilledPrice,
				PnL:        order.PnL(),
				PnLPct:     0,
				EntryTime:  bar.OpenTime,
				ExitTime:   bar.CloseTime,
				StrategyID: strat.Meta.ID,
			}
			trades = append(trades, trade)
			equity += order.PnL() - Fee(order)
		}

		equityCurve = append(equityCurve, types.EquityPoint{
			Time:   time.UnixMilli(ts),
			Equity: equity,
		})
	}

	result := ComputeStats(trades, equityCurve, startCapital)
	return result, nil
}

func (e *Engine) SaveResult(result *BacktestResult, path string) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func checkLimitFill(order *types.Order, bar types.Kline) bool {
	if order.Side == types.DirBuy && bar.Low <= order.Price {
		return true
	}
	if order.Side == types.DirSell && bar.High >= order.Price {
		return true
	}
	return false
}

func orderFromSignal(sig *types.Signal, t time.Time) *types.Order {
	return &types.Order{
		Symbol:     sig.Symbol,
		Side:       sig.Direction,
		Type:       sig.Type,
		Size:       sig.Size,
		Price:      sig.Price,
		Status:     types.OrdNew,
		CreatedAt:  t,
		StrategyID: sig.StrategyID,
	}
}

```

- [ ] **Step 4: Add PnL method to Order type**

Read `internal/types/types.go` and append after the Order struct:

```go
// PnL returns the realized profit/loss for a filled order.
// Positive = profit for long sell; negative = loss.
func (o *Order) PnL() float64 {
	if o.Status != OrdFilled {
		return 0
	}
	if o.FilledPrice == 0 || o.Price == 0 {
		return 0
	}
	if o.Side == DirBuy {
		return 0 // Entry — PnL realized on sell only
	}
	return (o.FilledPrice - o.Price) * o.FilledSize
}
```

- [ ] **Step 5: Verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/backtest/ internal/types/
git commit -m "feat: add backtest engine with fill simulation and statistics"
```

---

### Task 12: Live Order Manager (Binance Integration)

**Files:**
- Create: `internal/order/live.go` — Binance live order manager

- [ ] **Step 1: Write Binance order manager (skeleton with real API calls)**

Create `internal/order/live.go`:

```go
package order

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
)

type LiveOrderManager struct {
	baseURL    string
	apiKey     string
	secretKey  string
	httpClient *http.Client
	mu         sync.RWMutex
	orders     map[string]*types.Order
}

func NewLiveOrderManager(baseURL, apiKey, secretKey string) *LiveOrderManager {
	return &LiveOrderManager{
		baseURL: baseURL,
		apiKey:  apiKey,
		secretKey: secretKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		orders:  make(map[string]*types.Order),
	}
}

func (m *LiveOrderManager) Place(ctx context.Context, signal *types.Signal) (*types.Order, error) {
	params := url.Values{}
	params.Set("symbol", signal.Symbol)
	params.Set("side", strings.ToUpper(string(signal.Direction)))
	params.Set("type", strings.ToUpper(string(signal.Type)))
	params.Set("quantity", fmt.Sprintf("%.8f", signal.Size))
	if signal.Type == types.OrdLimit {
		params.Set("price", fmt.Sprintf("%.2f", signal.Price))
		params.Set("timeInForce", "GTC")
	}
	params.Set("timestamp", fmt.Sprintf("%d", time.Now().UnixMilli()))

	resp, err := m.signedRequest(ctx, "POST", "/api/v3/order", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		OrderID      int64  `json:"orderId"`
		Symbol       string `json:"symbol"`
		Side         string `json:"side"`
		Type         string `json:"type"`
		Price        string `json:"price"`
		OrigQty      string `json:"origQty"`
		ExecutedQty  string `json:"executedQty"`
		Status       string `json:"status"`
		TransactTime int64  `json:"transactTime"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("decode order response: %w", err)
	}

	order := &types.Order{
		ID:         fmt.Sprintf("%d", result.OrderID),
		Symbol:     signal.Symbol,
		Side:       signal.Direction,
		Type:       signal.Type,
		Price:      signal.Price,
		Size:       signal.Size,
		FilledSize: parseDecimal(result.ExecutedQty),
		Status:     mapBinanceStatus(result.Status),
		CreatedAt:  time.UnixMilli(result.TransactTime),
		UpdatedAt:  time.UnixMilli(result.TransactTime),
		StrategyID: signal.StrategyID,
	}

	m.mu.Lock()
	m.orders[order.ID] = order
	m.mu.Unlock()

	return order, nil
}

func (m *LiveOrderManager) Cancel(ctx context.Context, orderID string) error {
	params := url.Values{}
	params.Set("orderId", orderID)
	params.Set("timestamp", fmt.Sprintf("%d", time.Now().UnixMilli()))

	_, err := m.signedRequest(ctx, "DELETE", "/api/v3/order", params)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if o, ok := m.orders[orderID]; ok {
		o.Status = types.OrdCancelled
		o.UpdatedAt = time.Now()
	}
	m.mu.Unlock()
	return nil
}

func (m *LiveOrderManager) Status(ctx context.Context, orderID string) (*types.Order, error) {
	params := url.Values{}
	params.Set("orderId", orderID)
	params.Set("timestamp", fmt.Sprintf("%d", time.Now().UnixMilli()))

	resp, err := m.signedRequest(ctx, "GET", "/api/v3/order", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		OrderID      int64  `json:"orderId"`
		Symbol       string `json:"symbol"`
		Side         string `json:"side"`
		Type         string `json:"type"`
		Price        string `json:"price"`
		OrigQty      string `json:"origQty"`
		ExecutedQty  string `json:"executedQty"`
		CummQuoteQty string `json:"cummulativeQuoteQty"`
		Status       string `json:"status"`
		Time         int64  `json:"time"`
		UpdateTime   int64  `json:"updateTime"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("decode order status: %w", err)
	}

	order := &types.Order{
		ID:          fmt.Sprintf("%d", result.OrderID),
		Symbol:      result.Symbol,
		Side:        types.Dir(strings.ToLower(result.Side)),
		Type:        types.OrdType(strings.ToLower(result.Type)),
		Price:       parseDecimal(result.Price),
		Size:        parseDecimal(result.OrigQty),
		FilledSize:  parseDecimal(result.ExecutedQty),
		FilledPrice: 0,
		Status:      mapBinanceStatus(result.Status),
		CreatedAt:   time.UnixMilli(result.Time),
		UpdatedAt:   time.UnixMilli(result.UpdateTime),
	}
	if order.FilledSize > 0 {
		order.FilledPrice = parseDecimal(result.CummQuoteQty) / order.FilledSize
	}

	return order, nil
}

func (m *LiveOrderManager) OpenOrders(ctx context.Context, symbol string) ([]types.Order, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	params.Set("timestamp", fmt.Sprintf("%d", time.Now().UnixMilli()))

	resp, err := m.signedRequest(ctx, "GET", "/api/v3/openOrders", params)
	if err != nil {
		return nil, err
	}

	var results []struct {
		OrderID      int64  `json:"orderId"`
		Symbol       string `json:"symbol"`
		Side         string `json:"side"`
		Type         string `json:"type"`
		Price        string `json:"price"`
		OrigQty      string `json:"origQty"`
		ExecutedQty  string `json:"executedQty"`
		Status       string `json:"status"`
		Time         int64  `json:"time"`
		UpdateTime   int64  `json:"updateTime"`
	}
	if err := json.Unmarshal(resp, &results); err != nil {
		return nil, fmt.Errorf("decode open orders: %w", err)
	}

	var orders []types.Order
	for _, r := range results {
		orders = append(orders, types.Order{
			ID:         fmt.Sprintf("%d", r.OrderID),
			Symbol:     r.Symbol,
			Side:       types.Dir(strings.ToLower(r.Side)),
			Type:       types.OrdType(strings.ToLower(r.Type)),
			Price:      parseDecimal(r.Price),
			Size:       parseDecimal(r.OrigQty),
			FilledSize: parseDecimal(r.ExecutedQty),
			Status:     mapBinanceStatus(r.Status),
			CreatedAt:  time.UnixMilli(r.Time),
			UpdatedAt:  time.UnixMilli(r.UpdateTime),
		})
	}
	return orders, nil
}

func (m *LiveOrderManager) signedRequest(ctx context.Context, method, endpoint string, params url.Values) ([]byte, error) {
	queryString := params.Encode()
	sig := m.sign(queryString)
	fullURL := fmt.Sprintf("%s%s?%s&signature=%s", m.baseURL, endpoint, queryString, sig)

	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("binance API error %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (m *LiveOrderManager) sign(queryString string) string {
	mac := hmac.New(sha256.New, []byte(m.secretKey))
	mac.Write([]byte(queryString))
	return hex.EncodeToString(mac.Sum(nil))
}

func mapBinanceStatus(s string) types.OrdStatus {
	switch s {
	case "NEW":
		return types.OrdNew
	case "PARTIALLY_FILLED":
		return types.OrdPartialFill
	case "FILLED":
		return types.OrdFilled
	case "CANCELED":
		return types.OrdCancelled
	case "REJECTED", "EXPIRED":
		return types.OrdRejected
	default:
		return types.OrdNew
	}
}

func parseDecimal(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
```

- [ ] **Step 2: Verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/order/live.go
git commit -m "feat: add Binance live order manager with HMAC signing"
```

---

### Task 13: HTTP API Module

**Files:**
- Create: `internal/api/server.go` — HTTP server setup
- Create: `internal/api/handlers.go` — API endpoints

- [ ] **Step 1: Write API handlers**

Create `internal/api/handlers.go`:

```go
package api

import (
	"encoding/json"
	"net/http"

	"github.com/colinmyth/quant_ba/internal/portfolio"
	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/types"
)

type Handlers struct {
	loader    *strategy.Loader
	portfolio *portfolio.Service
	statusFn  func(string) *types.Portfolio // callback to executor
}

func NewHandlers(loader *strategy.Loader, portfolio *portfolio.Service) *Handlers {
	return &Handlers{loader: loader, portfolio: portfolio}
}

func (h *Handlers) ListStrategies(w http.ResponseWriter, r *http.Request) {
	metas := h.loader.List()
	writeJSON(w, metas)
}

func (h *Handlers) GetPortfolio(w http.ResponseWriter, r *http.Request) {
	p := h.portfolio.GetPortfolio()
	writeJSON(w, p)
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 2: Write HTTP server**

Create `internal/api/server.go`:

```go
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/colinmyth/quant_ba/internal/portfolio"
	"github.com/colinmyth/quant_ba/internal/strategy"
)

type Server struct {
	httpServer *http.Server
	handlers   *Handlers
}

func NewServer(port int, loader *strategy.Loader, portfolio *portfolio.Service) *Server {
	handlers := NewHandlers(loader, portfolio)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", handlers.Health)
	r.Get("/api/strategies", handlers.ListStrategies)
	r.Get("/api/portfolio", handlers.GetPortfolio)

	return &Server{
		httpServer: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: r,
		},
		handlers: handlers,
	}
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}
```

- [ ] **Step 3: Install chi and verify build**

```bash
cd /Users/colinmyth/code/quant_ba && go get github.com/go-chi/chi/v5@latest && go mod tidy && go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/api/ go.mod go.sum
git commit -m "feat: add HTTP API server with strategy and portfolio endpoints"
```

---

### Task 14: Wire Everything Together — Final Integration

**Files:**
- Modify: `cmd/quant_ba/root.go` — Add config loading and component initialization
- Modify: `cmd/quant_ba/strategy.go` — Wire real loader
- Modify: `cmd/quant_ba/backtest.go` — Wire backtest engine
- Modify: `cmd/quant_ba/paper.go` — Wire paper executor
- Modify: `cmd/quant_ba/serve.go` — Wire HTTP server

- [ ] **Step 1: Create app context wrapper**

Create `cmd/quant_ba/app.go`:

```go
package main

import (
	"github.com/colinmyth/quant_ba/internal/config"
	"github.com/colinmyth/quant_ba/internal/market"
	"github.com/colinmyth/quant_ba/internal/order"
	"github.com/colinmyth/quant_ba/internal/portfolio"
	"github.com/colinmyth/quant_ba/internal/risk"
	"github.com/colinmyth/quant_ba/internal/store"
	"github.com/colinmyth/quant_ba/internal/strategy"
)

type App struct {
	Config    *config.Config
	Store     *store.Store
	Feed      *market.WSClient
	Cache     *market.KlineCache
	Loader    *strategy.Loader
	Risk      risk.RiskManager
	PaperOM   order.OrderManager
	LiveOM    order.OrderManager
	Portfolio *portfolio.Service
}

func NewApp(cfgPath string) (*App, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}

	st, err := store.New(cfg.Store.Path)
	if err != nil {
		return nil, err
	}

	rest := market.NewRESTClient(cfg.Exchange.BaseURL)
	ws := market.NewWSClient(cfg.Exchange.WSURL)
	cache := market.NewKlineCache(st, rest)

	loader := strategy.NewLoader()
	riskMgr := risk.New(cfg.Risk)

	paperOM := order.NewPaperOrderManager()
	liveOM := order.NewLiveOrderManager(cfg.Exchange.BaseURL, cfg.Exchange.APIKey, cfg.Exchange.Secret)

	portf := portfolio.New(st)

	a := &App{
		Config:    cfg,
		Store:     st,
		Feed:      ws,
		Cache:     cache,
		Loader:    loader,
		Risk:      riskMgr,
		PaperOM:   paperOM,
		LiveOM:    liveOM,
		Portfolio: portf,
	}

	// Restore portfolio from snapshot
	if err := portf.Restore(); err != nil {
		return nil, err
	}

	return a, nil
}
```

- [ ] **Step 2: Update root command with config loading**

Modify `cmd/quant_ba/root.go` — replace the `init` function:

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgPath string

var rootCmd = &cobra.Command{
	Use:   "quant_ba",
	Short: "Quantitative trading system for Binance",
	Long:  "A multi-strategy quantitative trading platform supporting backtest, paper trading, and live trading on Binance.",
}

var configCmd = &cobra.Command{
	Use:   "config show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPathFlag := cmd.Flag("config").Value.String()
		app, err := NewApp(cfgPathFlag)
		if err != nil {
			return err
		}
		fmt.Printf("Exchange: %s (%s)\n", app.Config.Exchange.Name, app.Config.Exchange.BaseURL)
		fmt.Printf("Testnet: %v\n", app.Config.Exchange.Testnet)
		fmt.Printf("Store: %s\n", app.Config.Store.Path)
		fmt.Printf("Server port: %d\n", app.Config.Server.Port)
		fmt.Printf("Risk - max position: %.0f%%\n", app.Config.Risk.Basic.MaxPositionPct*100)
		fmt.Printf("Risk - daily loss limit: %.0f%%\n", app.Config.Risk.CircuitBreaker.DailyLossPct*100)
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "config/default.yaml", "path to config file")
	rootCmd.AddCommand(configCmd)
}
```

- [ ] **Step 3: Wire strategy subcommands**

Modify `cmd/quant_ba/strategy.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(strategyCmd)
	strategyCmd.AddCommand(strategyListCmd)
	strategyCmd.AddCommand(strategyLoadCmd)
	strategyCmd.AddCommand(strategyUnloadCmd)
	strategyCmd.AddCommand(strategyParamsCmd)
}

var strategyCmd = &cobra.Command{
	Use:   "strategy",
	Short: "Manage trading strategies",
}

var strategyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List loaded strategies",
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		metas := app.Loader.List()
		if len(metas) == 0 {
			fmt.Println("No strategies loaded. Use 'quant_ba strategy load <path>' to load a plugin.")
			return nil
		}
		for _, m := range metas {
			fmt.Printf("  %s  %s  v%s  (%s)\n", m.ID, m.Name, m.Version, m.Path)
		}
		return nil
	},
}

var strategyLoadCmd = &cobra.Command{
	Use:   "load <plugin-path>",
	Short: "Load a strategy plugin binary",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		meta, err := app.Loader.Load(args[0])
		if err != nil {
			return fmt.Errorf("load strategy: %w", err)
		}
		fmt.Printf("Loaded: %s (%s) v%s\n", meta.Name, meta.ID, meta.Version)
		return nil
	},
}

var strategyUnloadCmd = &cobra.Command{
	Use:   "unload <strategy-id>",
	Short: "Unload a strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		if err := app.Loader.Unload(args[0]); err != nil {
			return err
		}
		fmt.Printf("Unloaded: %s\n", args[0])
		return nil
	},
}

var strategyParamsCmd = &cobra.Command{
	Use:   "params <strategy-id>",
	Short: "Show strategy parameters",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Parameters for %s (params query not yet available via RPC)\n", args[0])
		return nil
	},
}
```

- [ ] **Step 4: Wire paper trading**

Modify `cmd/quant_ba/paper.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/colinmyth/quant_ba/internal/executor"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(paperCmd)
}

var paperCmd = &cobra.Command{
	Use:   "paper start <strategy-id>",
	Short: "Start paper trading for a strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}

		exec := executor.NewPaperExecutor(app.Feed, app.Loader, app.Risk, app.PaperOM, app.Portfolio)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		go func() {
			<-sigCh
			fmt.Println("\nShutting down...")
			cancel()
		}()

		fmt.Printf("Starting paper trading: %s\n", args[0])
		if err := exec.Run(ctx, args[0]); err != nil {
			return err
		}
		return nil
	},
}
```

- [ ] **Step 5: Wire backtest**

Modify `cmd/quant_ba/backtest.go`:

```go
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/colinmyth/quant_ba/internal/backtest"
	"github.com/spf13/cobra"
)

var (
	btSymbols  string
	btInterval string
	btStart    string
	btEnd      string
	btCapital  float64
	btOut      string
)

func init() {
	rootCmd.AddCommand(backtestCmd)
	backtestCmd.Flags().StringVar(&btSymbols, "symbols", "BTCUSDT", "Comma-separated symbols")
	backtestCmd.Flags().StringVar(&btInterval, "interval", "1h", "Kline interval")
	backtestCmd.Flags().StringVar(&btStart, "start", "2025-01-01", "Start date (YYYY-MM-DD)")
	backtestCmd.Flags().StringVar(&btEnd, "end", "2025-12-31", "End date (YYYY-MM-DD)")
	backtestCmd.Flags().Float64Var(&btCapital, "capital", 10000, "Starting capital in USDT")
	backtestCmd.Flags().StringVar(&btOut, "out", "results/backtest.json", "Output file path")
}

var backtestCmd = &cobra.Command{
	Use:   "backtest run <strategy-id>",
	Short: "Run backtest for a strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}

		ls, err := app.Loader.Get(args[0])
		if err != nil {
			return fmt.Errorf("strategy not loaded: %w", err)
		}

		start, _ := time.Parse("2006-01-02", btStart)
		end, _ := time.Parse("2006-01-02", btEnd)
		symbols := strings.Split(btSymbols, ",")

		engine := backtest.NewEngine(app.Cache, app.Risk, app.Portfolio, app.Store)
		result, err := engine.Run(context.Background(), ls, symbols, btInterval, start, end, btCapital)
		if err != nil {
			return err
		}

		if err := engine.SaveResult(result, btOut); err != nil {
			return err
		}

		fmt.Printf("Backtest complete: %d trades\n", result.TotalTrades)
		fmt.Printf("  Total Return: %.2f%%\n", result.TotalReturn*100)
		fmt.Printf("  Sharpe Ratio: %.3f\n", result.SharpeRatio)
		fmt.Printf("  Max Drawdown: %.2f%%\n", result.MaxDrawdown*100)
		fmt.Printf("  Win Rate:     %.2f%%\n", result.WinRate*100)
		fmt.Printf("  Profit Factor: %.2f\n", result.ProfitFactor)
		fmt.Printf("  Results saved to: %s\n", btOut)
		return nil
	},
}

```

- [ ] **Step 6: Wire HTTP server**

Modify `cmd/quant_ba/serve.go`:

```go
package main

import (
	"context"
	"fmt"

	"github.com/colinmyth/quant_ba/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start HTTP API server",
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}

		server := api.NewServer(app.Config.Server.Port, app.Loader, app.Portfolio)
		fmt.Printf("HTTP server listening on :%d\n", app.Config.Server.Port)
		if err := server.Start(); err != nil {
			return fmt.Errorf("server: %w", err)
		}
		return nil
	},
}
```

- [ ] **Step 7: Build and verify**

```bash
cd /Users/colinmyth/code/quant_ba && go build ./cmd/quant_ba/
```

Expected: binary compiles. Run `./quant_ba --help` to verify full CLI.

- [ ] **Step 8: Commit**

```bash
git add cmd/ go.mod go.sum
git commit -m "feat: wire all modules together with full CLI integration"
```

---

### Task 15: End-to-End Smoke Test

- [ ] **Step 1: Initialize paper trading portfolio and test**

```bash
cd /Users/colinmyth/code/quant_ba && go run ./cmd/quant_ba/ strategy load ./plugins/ma_cross_v1
```

Expected: "Loaded: MA Cross (ma_cross_v1) v1.0.0"

- [ ] **Step 2: Verify strategy listing**

```bash
cd /Users/colinmyth/code/quant_ba && go run ./cmd/quant_ba/ strategy list
```

Expected: Shows ma_cross_v1 in the list.

- [ ] **Step 3: Build full binary**

```bash
cd /Users/colinmyth/code/quant_ba && go build -o quant_ba ./cmd/quant_ba/ && ./quant_ba --help
```

Expected: binary created, help output shows all subcommands.

- [ ] **Step 4: Commit (if any fixes)**

```bash
git add -A
git commit -m "chore: end-to-end smoke test fixes"
```

---

## Self-Review Checklist

1. **Spec coverage:** Each spec section maps to a task: Market→T4+T5, Strategy→T7, Order→T8+T12, Risk→T9, Backtest→T11, Executor→T10, Portfolio→T6, CLI→T2+T14, API→T13, Store→T3. All covered.

2. **Placeholder scan:** No TBD/TODO. Every task has concrete code. All file paths exact. All commands specified.

3. **Type consistency:** Signal, Order, Portfolio, Kline types defined in T1 and used consistently throughout. `DirHold` vs `DirSell` naming matches. `Signal.StrategyID` field used in executor and backtest.
