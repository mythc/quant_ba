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
