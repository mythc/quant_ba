package api

import (
	"encoding/json"
	"net/http"

	"github.com/colinmyth/quant_ba/internal/portfolio"
	"github.com/colinmyth/quant_ba/internal/strategy"
)

type Handlers struct {
	loader    *strategy.Loader
	portfolio *portfolio.Service
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
