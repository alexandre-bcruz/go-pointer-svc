// internal/domain/entrycfg.go
package domain

type EntryUpsertCfg struct {
    Enabled           bool    `json:"enabled"`
    Side              Side    `json:"side"`
    InitialPriceUSD   float64 `json:"initial_price_usd"`
    OrderValueUSD     float64 `json:"order_value_usd"`
    OrderMarginSats   int64   `json:"order_margin_sats"`
    ContractsQuantity int     `json:"contracts_quantity"`
    Leverage          int     `json:"leverage"`
    PriceIntervalUSD  float64 `json:"price_interval_usd"`
	Fingerprint string `json:"fingerprint"`
}

type EntryPutResult struct {
	Fingerprint string `json:"fingerprint"`
	Rebuild     bool   `json:"rebuild"`
	Grid        struct {
		ZSet             string  `json:"zset"`
		Count            int     `json:"count"`
		InitialPriceUSD  float64 `json:"initial_price_usd"`
		IntervalUSD      float64 `json:"interval_usd"`
		MinPriceUSD      float64 `json:"min_price_usd"`
		MaxPriceUSD      float64 `json:"max_price_usd"`
	} `json:"grid"`
}
