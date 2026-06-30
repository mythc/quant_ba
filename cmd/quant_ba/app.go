package main

import (
	"context"
	"fmt"
	"os"

	"github.com/colinmyth/quant_ba/internal/api"
	"github.com/colinmyth/quant_ba/internal/config"
	"github.com/colinmyth/quant_ba/internal/executor"
	"github.com/colinmyth/quant_ba/internal/market"
	"github.com/colinmyth/quant_ba/internal/order"
	"github.com/colinmyth/quant_ba/internal/portfolio"
	"github.com/colinmyth/quant_ba/internal/risk"
	"github.com/colinmyth/quant_ba/internal/store"
	"github.com/colinmyth/quant_ba/internal/strategy"
)

// App holds all initialized components of the trading system.
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
	PaperExec *executor.PaperExecutor
	APIServer *api.Server
}

// NewApp initializes all components from the given config file path.
func NewApp(cfgPath string) (*App, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// Ensure data directory exists.
	if err := os.MkdirAll("data", 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	st, err := store.New(cfg.Store.Path)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	futures := cfg.Exchange.IsFutures()
	rest := market.NewRESTClient(cfg.Exchange.ActiveBaseURL())
	ws := market.NewWSClient(cfg.Exchange.ActiveWSURL(), rest)
	cache := market.NewKlineCache(st, rest)

	loader := strategy.NewLoader()
	riskMgr := risk.New(cfg.Risk)

	paperOM := order.NewPaperOrderManager()
	liveOM := order.NewLiveOrderManager(cfg.Exchange.ActiveBaseURL(), cfg.Exchange.APIKey, cfg.Exchange.Secret, futures)

	portf := portfolio.New(st, futures)

	// Restore portfolio from snapshot.
	if err := portf.Restore(); err != nil {
		fmt.Printf("warning: could not restore portfolio: %v\n", err)
	}

	// Connect WebSocket (non-blocking, for paper/live modes).
	go func() {
		if err := ws.Connect(context.Background()); err != nil {
			fmt.Printf("warning: ws connect failed: %v\n", err)
		}
	}()

	paperExec := executor.NewPaperExecutor(ws, loader, riskMgr, paperOM, portf)

	var apiServer *api.Server
	if cfg.Server.Enabled {
		apiServer = api.NewServer(cfg.Server.Port, loader, portf)
	}

	return &App{
		Config:    cfg,
		Store:     st,
		Feed:      ws,
		Cache:     cache,
		Loader:    loader,
		Risk:      riskMgr,
		PaperOM:   paperOM,
		LiveOM:    liveOM,
		Portfolio: portf,
		PaperExec: paperExec,
		APIServer: apiServer,
	}, nil
}
