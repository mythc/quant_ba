package portfolio

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/store"
	"github.com/colinmyth/quant_ba/internal/types"
)

type Service struct {
	mu        sync.RWMutex
	balances  map[string]types.Balance
	positions map[string]*types.Position
	store     *store.Store
	futures   bool
}

// New creates a portfolio service. When futures is true, fills are accounted
// with margin-based, long/short (one-way net) semantics; otherwise spot.
func New(store *store.Store, futures bool) *Service {
	return &Service{
		balances:  make(map[string]types.Balance),
		positions: make(map[string]*types.Position),
		store:     store,
		futures:   futures,
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
		if s.futures {
			// Free already excludes locked margin; add it back plus unrealized.
			total += p.Margin + realizedPnL(p.Side, p.EntryPrice, p.CurrentPrice, p.Size)
		} else {
			total += p.Size * p.CurrentPrice
		}
	}
	return total
}

// OnOrderFilled updates balances and positions based on a filled order.
// This is the ONLY mutation entry point.
func (s *Service) OnOrderFilled(order types.Order) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.futures {
		s.onFuturesFill(order)
		return
	}

	price := order.FilledPrice
	if price == 0 {
		price = order.Price
	}
	fillValue := order.FilledSize * price

	// Update quote balance (USDT). Trading fees always reduce cash.
	quote := "USDT"
	if bal, ok := s.balances[quote]; ok {
		if order.Side == types.DirBuy {
			bal.Free -= fillValue
		} else {
			bal.Free += fillValue
		}
		bal.Free -= order.Fee
		s.balances[quote] = bal
	}

	// Extract base asset from symbol (e.g., BTCUSDT → BTC)
	base := extractBase(order.Symbol)
	if bal, ok := s.balances[base]; ok {
		if order.Side == types.DirBuy {
			bal.Free += order.FilledSize
		} else {
			bal.Free -= order.FilledSize
		}
		s.balances[base] = bal
	} else {
		if order.Side == types.DirBuy {
			s.balances[base] = types.Balance{Asset: base, Free: order.FilledSize}
		} else {
			s.balances[base] = types.Balance{Asset: base, Free: -order.FilledSize}
		}
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
			// Adding to position: recalculate average entry price
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

// onFuturesFill applies a filled order using margin-based, one-way net
// position accounting. A buy increases the net position toward long, a sell
// toward short. Margin is locked from the USDT balance on open and released
// (with realized PnL) on reduce/close. Funding payments are not modeled.
func (s *Service) onFuturesFill(order types.Order) {
	price := order.FilledPrice
	if price == 0 {
		price = order.Price
	}
	lev := order.Leverage
	if lev <= 0 {
		lev = 1
	}

	bal := s.balances["USDT"]

	pos := s.positions[order.Symbol]
	incoming := order.Side // DirBuy adds long exposure, DirSell adds short

	// Opposite of the current position side reduces/closes/flips it.
	if pos != nil && pos.Size > 0 && incoming != pos.Side {
		reduceQty := order.FilledSize
		if reduceQty > pos.Size {
			reduceQty = pos.Size
		}
		realized := realizedPnL(pos.Side, pos.EntryPrice, price, reduceQty)
		releasedMargin := pos.Margin * (reduceQty / pos.Size)
		bal.Free += releasedMargin + realized

		pos.Size -= reduceQty
		pos.Margin -= releasedMargin

		remainder := order.FilledSize - reduceQty
		if pos.Size <= 1e-12 {
			delete(s.positions, order.Symbol)
			pos = nil
			// A larger opposite order flips into a new position with remainder.
			if remainder > 1e-12 {
				s.openFutures(order.Symbol, incoming, remainder, price, lev, &bal)
			}
		}
	} else {
		// No position or same direction: open or add to the position.
		s.openFutures(order.Symbol, incoming, order.FilledSize, price, lev, &bal)
	}

	bal.Free -= order.Fee
	bal.Asset = "USDT"
	s.balances["USDT"] = bal
}

// openFutures opens or increases a position in the given direction, locking
// margin from bal.
func (s *Service) openFutures(symbol string, side types.Dir, size, price, lev float64, bal *types.Balance) {
	margin := size * price / lev
	bal.Free -= margin

	pos, ok := s.positions[symbol]
	if !ok || pos.Size == 0 {
		s.positions[symbol] = &types.Position{
			Symbol:       symbol,
			Side:         side,
			Size:         size,
			EntryPrice:   price,
			CurrentPrice: price,
			Leverage:     lev,
			Margin:       margin,
			LiqPrice:     liquidationPrice(side, price, lev),
			UpdatedAt:    time.Now(),
		}
		return
	}
	totalSize := pos.Size + size
	pos.EntryPrice = (pos.EntryPrice*pos.Size + price*size) / totalSize
	pos.Size = totalSize
	pos.Margin += margin
	pos.Leverage = lev
	pos.CurrentPrice = price
	pos.LiqPrice = liquidationPrice(side, pos.EntryPrice, lev)
	pos.UpdatedAt = time.Now()
}

// realizedPnL returns the PnL realized when closing `qty` of a position.
func realizedPnL(side types.Dir, entry, exit, qty float64) float64 {
	if side == types.DirSell { // short
		return (entry - exit) * qty
	}
	return (exit - entry) * qty
}

// liquidationPrice returns an approximate liquidation price (maintenance margin
// ignored): the price at which the loss equals the posted initial margin.
func liquidationPrice(side types.Dir, entry, lev float64) float64 {
	if lev <= 0 {
		return 0
	}
	if side == types.DirSell { // short liquidates on the way up
		return entry * (1 + 1/lev)
	}
	return entry * (1 - 1/lev)
}

// UpdatePrices updates current prices for all positions. In futures mode a
// position whose mark price breaches its liquidation price is force-closed,
// forfeiting its margin.
func (s *Service) UpdatePrices(prices map[string]float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for sym, pos := range s.positions {
		p, ok := prices[pos.Symbol]
		if !ok {
			continue
		}
		pos.CurrentPrice = p
		pos.PnL = realizedPnL(pos.Side, pos.EntryPrice, p, pos.Size)
		if pos.EntryPrice > 0 {
			if pos.Side == types.DirSell {
				pos.PnLPct = (pos.EntryPrice - p) / pos.EntryPrice
			} else {
				pos.PnLPct = (p - pos.EntryPrice) / pos.EntryPrice
			}
		}
		pos.UpdatedAt = time.Now()

		if s.futures && pos.LiqPrice > 0 {
			liquidated := (pos.Side == types.DirBuy && p <= pos.LiqPrice) ||
				(pos.Side == types.DirSell && p >= pos.LiqPrice)
			if liquidated {
				// Margin is fully lost; it was already deducted from Free.
				delete(s.positions, sym)
			}
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

// extractBase strips USDT from symbol to get base asset (e.g., BTCUSDT → BTC).
func extractBase(symbol string) string {
	if len(symbol) > 4 {
		return symbol[:len(symbol)-4]
	}
	return symbol
}
