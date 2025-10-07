package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"pointersvc/internal/domain"
	"pointersvc/internal/repo"
	"pointersvc/internal/service"
	"pointersvc/internal/utils"

	"github.com/go-chi/chi/v5"
)

const (
	accountFeature string = "[ account-feature ]"
)

func RegisterConfigsRoutes(r *chi.Mux, redis *repo.RedisRepo, qs *service.QueryService) {

	r.Get("/pointer/accounts/{accountID}/configs", func(w http.ResponseWriter, req *http.Request) {
		acct := chi.URLParam(req, "accountID")
		cfgs, err := qs.GetAccountConfigs(req.Context(), acct)
		if err != nil {
			utils.ErrJSON(w, req, 500, "configs_get_failed", err)
			return
		}
		utils.WriteJSON(w, 200, cfgs)
	})

	r.Get("/pointer/triggers/entry", func(w http.ResponseWriter, req *http.Request) {
		side := strings.ToLower(strings.TrimSpace(req.URL.Query().Get("side")))
		if side != "buy" && side != "sell" {
			utils.ErrJSON(w, req, http.StatusBadRequest, "param 'side' must be buy|sell", errors.New("Erro Inexperado"))
			return
		}
		accountID := strings.TrimSpace(req.URL.Query().Get("account_id"))
		var acctPtr *string
		if accountID != "" {
			acctPtr = &accountID
		}

		min := parseFloatPtr(req.URL.Query().Get("min"))
		max := parseFloatPtr(req.URL.Query().Get("max"))
		limit := parseLimit(req.URL.Query().Get("limit"), 200)
		desc := parseDesc(req.URL.Query().Get("order"))

		items, err := qs.ListEntryTriggers(req.Context(), side, acctPtr, min, max, limit, desc)
		if err != nil {
			utils.ErrJSON(w, req, 500, "list_entry_triggers_failed", err)
			return
		}

		resp := domain.TriggerListResponse{}
		resp.Scope.Resource = "entry"
		resp.Scope.AccountID = acctPtr
		resp.Scope.Side = &side
		resp.Scope.Min = min
		resp.Scope.Max = max
		if desc {
			resp.Scope.Order = "desc"
		} else {
			resp.Scope.Order = "asc"
		}
		resp.Scope.Limit = limit

		resp.Stats.Count = len(items)
		if len(items) > 0 {
			first := items[0].Score
			last := items[len(items)-1].Score
			resp.Stats.FirstScore = &first
			resp.Stats.LastScore = &last
		}
		resp.Items = items

		utils.WriteJSON(w, 200, resp)
	})

	// GET /triggers/margin?account_id=...&min=...&max=...&limit=200&order=asc
	r.Get("/pointer/triggers/margin", func(w http.ResponseWriter, req *http.Request) {
		accountID := strings.TrimSpace(req.URL.Query().Get("account_id"))
		var acctPtr *string
		if accountID != "" {
			acctPtr = &accountID
		}

		min := parseFloatPtr(req.URL.Query().Get("min"))
		max := parseFloatPtr(req.URL.Query().Get("max"))
		limit := parseLimit(req.URL.Query().Get("limit"), 200)
		desc := parseDesc(req.URL.Query().Get("order"))

		items, err := qs.ListMarginTriggers(req.Context(), acctPtr, min, max, limit, desc)
		if err != nil {
			utils.ErrJSON(w, req, 500, "list_margin_triggers_failed", err)
			return
		}

		resp := domain.TriggerListResponse{}
		resp.Scope.Resource = "margin"
		resp.Scope.AccountID = acctPtr
		resp.Scope.Min = min
		resp.Scope.Max = max
		if desc {
			resp.Scope.Order = "desc"
		} else {
			resp.Scope.Order = "asc"
		}
		resp.Scope.Limit = limit

		resp.Stats.Count = len(items)
		if len(items) > 0 {
			first := items[0].Score
			last := items[len(items)-1].Score
			resp.Stats.FirstScore = &first
			resp.Stats.LastScore = &last
		}
		resp.Items = items

		utils.WriteJSON(w, 200, resp)
	})

	// GET /triggers/stopgain?account_id=...&side=buy|sell&min=...&max=...&limit=200&order=asc
	r.Get("/pointer/triggers/stopgain", func(w http.ResponseWriter, req *http.Request) {
		accountID := strings.TrimSpace(req.URL.Query().Get("account_id"))
		side := strings.ToLower(strings.TrimSpace(req.URL.Query().Get("side")))
		var sidePtr *string
		if side == "buy" || side == "sell" {
			sidePtr = &side
		}

		var acctPtr *string
		if accountID != "" {
			acctPtr = &accountID
		}

		min := parseFloatPtr(req.URL.Query().Get("min"))
		max := parseFloatPtr(req.URL.Query().Get("max"))
		limit := parseLimit(req.URL.Query().Get("limit"), 200)
		desc := parseDesc(req.URL.Query().Get("order"))

		items, err := qs.ListStopGainTriggers(req.Context(), acctPtr, sidePtr, min, max, limit, desc)
		if err != nil {
			utils.ErrJSON(w, req, 500, "list_stopgain_triggers_failed", err)
			return
		}

		resp := domain.TriggerListResponse{}
		resp.Scope.Resource = "stopgain"
		resp.Scope.AccountID = acctPtr
		resp.Scope.Side = sidePtr
		resp.Scope.Min = min
		resp.Scope.Max = max
		if desc {
			resp.Scope.Order = "desc"
		} else {
			resp.Scope.Order = "asc"
		}
		resp.Scope.Limit = limit

		resp.Stats.Count = len(items)
		if len(items) > 0 {
			first := items[0].Score
			last := items[len(items)-1].Score
			resp.Stats.FirstScore = &first
			resp.Stats.LastScore = &last
		}
		resp.Items = items

		utils.WriteJSON(w, 200, resp)
	})

	r.Get("/pointer/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

}

func parseFloatPtr(q string) *float64 {
	if q == "" {
		return nil
	}
	if v, err := strconv.ParseFloat(q, 64); err == nil {
		return &v
	}
	return nil
}
func parseLimit(q string, def int64) int64 {
	if q == "" {
		return def
	}
	if v, err := strconv.ParseInt(q, 10, 64); err == nil && v > 0 {
		return v
	}
	return def
}
func parseDesc(q string) bool {
	switch strings.ToLower(q) {
	case "desc", "true", "1":
		return true
	default:
		return false
	}
}
