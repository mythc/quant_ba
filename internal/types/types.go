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
	OrdNew         OrdStatus = "new"
	OrdPartialFill OrdStatus = "partial_fill"
	OrdFilled      OrdStatus = "filled"
	OrdCancelled   OrdStatus = "cancelled"
	OrdRejected    OrdStatus = "rejected"
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

// PnL returns the realized profit/loss for a filled order.
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
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Path      string   `json:"path"`
	Symbols   []string `json:"symbols"`
	Intervals []string `json:"intervals"`
}
