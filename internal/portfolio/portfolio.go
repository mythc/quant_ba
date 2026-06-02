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

	// Update quote balance (USDT)
	quote := "USDT"
	if bal, ok := s.balances[quote]; ok {
		if order.Side == types.DirBuy {
			bal.Free -= fillValue
		} else {
			bal.Free += fillValue
		}
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

// extractBase strips USDT from symbol to get base asset (e.g., BTCUSDT → BTC).
func extractBase(symbol string) string {
	if len(symbol) > 4 {
		return symbol[:len(symbol)-4]
	}
	return symbol
}
