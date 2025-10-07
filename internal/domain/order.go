package domain

import "pointersvc/internal/utils"


type OrderStatus string

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

const (
	StatusPending  OrderStatus = "pending"
	StatusExecuted OrderStatus = "executed"
	StatusOpen     OrderStatus = "open"
	StatusClosed   OrderStatus = "closed"
)

/* ===== Ordem canônica (como é gravada no Redis) ===== */

type Order struct {
	ID            string   `json:"id"`
	AccountID     string   `json:"account_id"`
	Source        string   `json:"source,omitempty"`          // "gs", "ln", ...
	BrokerOrderID string   `json:"broker_order_id,omitempty"` // se houver
	Operation     OrderOp  `json:"operation"`
	Profit        ProfitSt `json:"profit"`
	Margin        MarginSt `json:"margin"`
	Entry         EntrySt  `json:"entry"`
	Fees		  Fee 	   `json:"fees,omitempty"`
	CreatedAt     string   `json:"created_at"`
}

type Fee struct {
	OpeningFeeSats int `json:"opening_sats"`
	ClosingFeeSats int `json:"closing_sats"`
	CarryFeeSats   int `json:"sum_carry_sats"`
}

type OrderOp struct {
	PriceUSD      float64     `json:"price_usd"`
	OrderValueUSD float64     `json:"order_value_usd"`
	MarginSats    int64       `json:"margin_sats"`
	Leverage      int         `json:"leverage"`
	LiqPriceUSD   float64     `json:"liq_price_usd,omitempty"`
	Side          Side        `json:"side"`
	Status        OrderStatus `json:"status"`
	ExecutedAt    string      `json:"executed_at"`
}

type EntrySt struct {
	Enabled           bool   `json:"enabled"`
	ContractsQuantity int    `json:"contracts_quantity"`
	Fingerprint       string `json:"fingerprint"`
}

type MarginSt struct {
	Enabled         bool    `json:"enabled"`
	ThresholdValPct float64 `json:"threshold_val_pct,omitempty"`
	ExtraMarginPct  float64 `json:"extra_margin_pct,omitempty"`
	MarginTrigger   float64 `json:"margin_trigger,omitempty"`
}

type ProfitSt struct {
	Enabled     bool       `json:"enabled"`
	Type        ProfitType `json:"type"`                 // "stopgain" | "takeprofit"
	Percent     float64    `json:"percent"`              // % bruta de lucro (para cálculo interno)
	IntervalUSD float64    `json:"interval_usd"`         // p/ stopgain
	TargetUSD   float64    `json:"target_usd,omitempty"` // se você persistir
	StopLossUSD float64    `json:"stoploss_usd,omitempty"`
}

type OrderOpened struct {
	AccountID         string  `json:"account_id"`
	BrokerOrderID     string  `json:"broker_order_id"`
	Side              Side    `json:"side"` // "buy" | "sell"
	EntryPriceUSD     float64 `json:"entry_price_usd"`
	OrderValueUSD     float64 `json:"order_value_usd"` // << era qty; total em USD executado
	LiqPriceUSD       float64 `json:"liq_price_usd,omitempty"`
	Leverage          int64   `json:"leverage"`
	InitialMarginSats int64   `json:"initial_margin_sats,omitempty"`
	Source            string  `json:"source"` // ex.: "ln"
	IdempotencyKey    string  `json:"idempotency_key,omitempty"`
	ExecutedAt        string  `json:"executed_at,omitempty"` // RFC3339
}

type LNOrders struct {
	Running []Order `json:"running"`
	Closed  []Order `json:"closed"`
}

// EnrichMargin aplica cfg de margem na ordem.
// Mantém o cálculo do MarginTrigger para fora (falta L / dados de risco aqui).
func (o *Order) EnrichMargin(marginCfg MarginCfg) {
	o.Margin.Enabled = marginCfg.Enabled
	o.Margin.ThresholdValPct = marginCfg.ThresholdValPct
	o.Margin.ExtraMarginPct = marginCfg.ExtraMarginPct
	o.Margin.MarginTrigger = utils.CalcMarginTriggerDirectional(o.Operation.LiqPriceUSD, string(o.Operation.Side), marginCfg.ThresholdValPct)
}

// EnrichProfit aplica cfg de lucro na ordem e calcula campos derivados.
// Regra: alvo líquido = P0 * (1 + (profitPct + 0.1 + 0.1 + 0.03*dias)/100)
// StopGain: StopLoss = target; Trigger = target ± interval (buy:+ / sell:-)
// (Arredondado a 0 casas decimais)
func (o *Order) EnrichProfit(cfgSet ProfitCfgSet) {
	var cfg ProfitCfg

	if cfgSet.StopGain.Enabled {
		cfg = cfgSet.StopGain
		o.Profit.Type = ProfitStopGain
	} else if cfgSet.TakeProfit.Enabled {
		cfg = cfgSet.TakeProfit
		o.Profit.Type = ProfitTakeProfit
	} else {
		o.Profit.Enabled = false
		return
	}

	o.Profit.Enabled = cfg.Enabled
	o.Profit.Percent = cfg.Percent
	o.Profit.IntervalUSD = cfg.IntervalUSD

	if cfg.Type == ProfitStopGain {
		o.Profit.TargetUSD, o.Profit.StopLossUSD = utils.CalcStopGainTargets(
			o.Operation.PriceUSD,
			cfg.Percent,
			cfg.IntervalUSD,
			string(o.Operation.Side),
			utils.DaysSinceNow(o.Operation.ExecutedAt),
		)
	} else {
		o.Profit.TargetUSD = utils.CalcTakeProfitValue(
			o.Operation.PriceUSD,
			cfg.Percent,
			utils.DaysSinceNow(o.Operation.ExecutedAt),
			string(o.Operation.Side),
		)
		o.Profit.StopLossUSD = 0
	}
}