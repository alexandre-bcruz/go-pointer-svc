// internal/domain/profitcfg.go
package domain

type ProfitType string

const (
	ProfitTakeProfit ProfitType = "takeprofit"
	ProfitStopGain   ProfitType = "stopgain"
)

type ProfitUpsert struct {
	Enabled     bool       `json:"enabled"`
	Type        ProfitType `json:"type"`    // "takeprofit" | "stopgain"
	Percent     float64    `json:"percent"` // ex.: 5.0
	IntervalUSD float64    `json:"interval_usd,omitempty"`
}

type ProfitCfgSet struct {
	TakeProfit ProfitCfg `json:"takeprofit"`
	StopGain   ProfitCfg `json:"stopgain"`
}

type ProfitCfg struct {
	Enabled     bool    `json:"enabled"`
	Type        ProfitType  `json:"type"`           // "takeprofit" ou "stopgain"
	Percent     float64 `json:"percent"`        // percentual alvo
	IntervalUSD float64 `json:"interval_usd"`   // usado no stopgain
	UpdatedAt   int64   `json:"updated_at"`     // timestamp opcional
}

type StopGainRebaseRequest struct {
    PrevTargetUSD  float64 `json:"prev_target_usd,omitempty"`
    NewTargetUSD   float64 `json:"new_target_usd"`
    NewStopUSD     float64 `json:"new_stop_usd,omitempty"`
    IdempotencyKey string  `json:"idempotency_key,omitempty"`
}
type StopGainAdvanceRequest struct {
    PrevTargetUSD  float64 `json:"prev_target_usd,omitempty"`
    NextTargetUSD  float64 `json:"next_target_usd"`
    NewStopUSD     float64 `json:"new_stop_usd,omitempty"`
    IdempotencyKey string  `json:"idempotency_key,omitempty"`
}

type ProfitEdge struct {
	TargetUSD   float64 `json:"target_usd"`
	StopLossUSD float64 `json:"stoploss_usd,omitempty"`
}

type StopGainRebaseResponse struct {
	OrderID      string     `json:"order_id"`
	Action       string     `json:"action"` // "rebase"
	Old          ProfitEdge `json:"old"`
	New          ProfitEdge `json:"new"`
	Persisted    bool       `json:"persisted"`
	ZSetsUpdated bool       `json:"zsets_updated"`
}

type StopGainAdvanceResponse struct {
	OrderID      string     `json:"order_id"`
	Action       string     `json:"action"` // "advance"
	Old          ProfitEdge `json:"old"`    // target/stop atuais antes do avanço
	New          ProfitEdge `json:"new"`
	Persisted    bool       `json:"persisted"`
	ZSetsUpdated bool       `json:"zsets_updated"`
}


type TakeProfitRequest struct {
	ProfitCfg     ProfitCfg    `json:"profit_cfg"`
	AccountID      string     `json:"account_id"`

}