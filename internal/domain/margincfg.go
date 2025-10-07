// internal/domain/margincfg.go
package domain

type MarginCfg struct {
	Enabled         bool    `json:"enabled"`
	ThresholdValPct float64 `json:"threshold_val_pct,omitempty"`
	ExtraMarginPct  float64 `json:"extra_margin_pct,omitempty"`
	MarginTrigger  float64 `json:"margin_trigger,omitempty"`
}

type MarginRebaseRequest struct {
    LiquidationUSD  *float64 `json:"liquidation_usd,omitempty"`
    ThresholdValPct *float64 `json:"threshold_val_pct,omitempty"`
    ExtraMarginPct  *float64 `json:"extra_margin_pct,omitempty"`
}

type MarginRebaseResponse struct {
	OrderID         string  `json:"order_id"`
	AccountID       string  `json:"account_id"`
	Side            Side    `json:"side"`
	LiquidationUSD  float64 `json:"liquidation_usd"`
	ThresholdValPct float64 `json:"threshold_val_pct"`
	ExtraMarginPct  float64 `json:"extra_margin_pct"`
	PrevTrigger     float64 `json:"prev_trigger"`
	NewTrigger      float64 `json:"new_trigger"`
	Updated         bool    `json:"updated"`
}