// internal/domain/triggers.go
package domain

type TriggerItem struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

type TriggerListResponse struct {
	Scope struct {
		Resource  string   `json:"resource"`            // entry | margin | stopgain
		AccountID *string  `json:"account_id,omitempty"`
		Side      *string  `json:"side,omitempty"`      // entry/stopgain
		Min       *float64 `json:"min,omitempty"`
		Max       *float64 `json:"max,omitempty"`
		Order     string   `json:"order"`               // asc | desc
		Limit     int64    `json:"limit"`
	} `json:"scope"`
	Stats struct {
		Count      int      `json:"count"`
		FirstScore *float64 `json:"first_score,omitempty"`
		LastScore  *float64 `json:"last_score,omitempty"`
	} `json:"stats"`
	Items []TriggerItem `json:"items"`
}