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
	Name    string `yaml:"name"`
	BaseURL string `yaml:"base_url"`
	WSURL   string `yaml:"ws_url"`
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
	MaxPositionPct float64  `yaml:"max_position_pct"`
	MaxOrderUSDT   float64  `yaml:"max_order_usdt"`
	MaxSlippagePct float64  `yaml:"max_slippage_pct"`
	Blacklist      []string `yaml:"blacklist"`
}

type GlobalRiskConfig struct {
	MaxLeverage      float64 `yaml:"max_leverage"`
	MaxConcentration float64 `yaml:"max_concentration"`
	DailyTradeLimit  int     `yaml:"daily_trade_limit"`
	MinHoldSeconds   int     `yaml:"min_hold_seconds"`
}

type BreakerConfig struct {
	DailyLossPct       float64 `yaml:"daily_loss_pct"`
	ConsecutiveLosses  int     `yaml:"consecutive_losses"`
	VolatilityPausePct float64 `yaml:"volatility_pause_pct"`
	MaxDrawdownPct     float64 `yaml:"max_drawdown_pct"`
}

type StoreConfig struct {
	Path string `yaml:"path"`
}

type ServerConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
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
