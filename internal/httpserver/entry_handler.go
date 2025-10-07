package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"pointersvc/internal/domain"
	"pointersvc/internal/repo"
	"pointersvc/internal/service"
	"pointersvc/internal/utils"

	goRedis "github.com/redis/go-redis/v9"

	"github.com/go-chi/chi/v5"
)

func RegisterEntryAutomationRoutes(r *chi.Mux, redis *repo.RedisRepo, entrySvc *service.EntryService) {

	r.Put("/pointer/accounts/{accountID}/entry", func(w http.ResponseWriter, req *http.Request) {
		accountID := chi.URLParam(req, "accountID")

		var body domain.EntryUpsertCfg
		if !utils.DecodeJSON(w, req, &body) {
			return
		}

		res, err := entrySvc.PutCfg(req.Context(), body, accountID)
		if err != nil {
			utils.ErrJSON(w, req, 500, "entry_cfg_upsert_failed", err)
			return
		}

		utils.WriteJSON(w, http.StatusOK, map[string]any{
			"account_id":  accountID,
			"enabled":     body.Enabled,
			"side":        body.Side,
			"fingerprint": res.Fingerprint,
			"rebuild":     res.Rebuild,
			"grid": map[string]any{
				// "zset":              res.Grid.ZSet,
				"count":             res.Grid.Count,
				"initial_price_usd": res.Grid.InitialPriceUSD,
				"interval_usd":      res.Grid.IntervalUSD,
				"min_price_usd":     res.Grid.MinPriceUSD,
				"max_price_usd":     res.Grid.MaxPriceUSD,
			},
			"effective": map[string]any{
				"order_value_usd":   body.OrderValueUSD,
				"order_margin_sats": body.OrderMarginSats,
				"leverage":          body.Leverage,
			},
		})
	})

	r.Get("/pointer/accounts/{accountID}/entry", func(w http.ResponseWriter, req *http.Request) {
		acct := chi.URLParam(req, "accountID")

		cfgBuy, errBuy := redis.GetEntryCfgBySide(req.Context(), acct, string(domain.SideBuy))
		cfgSell, errSell := redis.GetEntryCfgBySide(req.Context(), acct, string(domain.SideSell))

		if errors.Is(errBuy, goRedis.Nil) && errors.Is(errSell, goRedis.Nil) {
			utils.ErrJSON(w, req, 404, "entry_cfg_not_found", fmt.Errorf("nenhuma config de entrada encontrada para a conta %s", acct))
			return
		}
		if errBuy != nil && !errors.Is(errBuy, goRedis.Nil) {
			utils.ErrJSON(w, req, 500, "entry_cfg_get_failed_buy", errBuy)
			return
		}
		if errSell != nil && !errors.Is(errSell, goRedis.Nil) {
			utils.ErrJSON(w, req, 500, "entry_cfg_get_failed_sell", errSell)
			return
		}

		buildCfgResp := func(cfg domain.EntryUpsertCfg) map[string]any {
			side := cfg.Side
			qty := cfg.ContractsQuantity
			initial := cfg.InitialPriceUSD
			interval := cfg.PriceIntervalUSD

			minPrice, maxPrice := initial, initial
			if qty > 0 && interval > 0 {
				if side == domain.SideBuy {
					minPrice = initial - interval*float64(qty-1)
					maxPrice = initial
				} else if side == domain.SideSell {
					minPrice = initial
					maxPrice = initial + interval*float64(qty-1)
				}
			}

			// contagem de triggers
			var zcount int64
			if side == domain.SideBuy || side == domain.SideSell {
				zkey := repo.KeyEntryTriggerZ(acct, string(side))
				if n, zerr := redis.Rdb().ZCard(req.Context(), zkey).Result(); zerr == nil {
					zcount = n
				}
			}

			return map[string]any{
				"enabled":     cfg.Enabled,
				"side":        side,
				"fingerprint": cfg.Fingerprint,
				"grid": map[string]any{
					"count_configured":  qty,
					"count_triggers":    zcount,
					"initial_price_usd": initial,
					"interval_usd":      interval,
					"range": map[string]float64{
						"min_price_usd": minPrice,
						"max_price_usd": maxPrice,
					},
				},
				"effective": map[string]any{
					"order_value_usd":   cfg.OrderValueUSD,
					"order_margin_sats": cfg.OrderMarginSats,
					"leverage":          cfg.Leverage,
				},
			}
		}

		resp := map[string]any{
			"account_id": acct,
		}
		if errBuy == nil {
			resp["entry_cfg_buy"] = buildCfgResp(cfgBuy)
		}
		if errSell == nil {
			resp["entry_cfg_sell"] = buildCfgResp(cfgSell)
		}

		utils.WriteJSON(w, 200, resp)
	})
}
