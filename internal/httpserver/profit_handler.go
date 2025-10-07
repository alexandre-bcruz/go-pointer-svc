package httpserver

import (
	"errors"
	"log"
	"net/http"
	"pointersvc/internal/domain"
	"pointersvc/internal/repo"
	"pointersvc/internal/service"
	"pointersvc/internal/utils"
	"time"

	"github.com/go-chi/chi/v5"
)

func RegisterProfitRoutes(r *chi.Mux, redis *repo.RedisRepo, profitSvc *service.ProfitService) {

	r.Put("/pointer/accounts/{accountID}/profit", func(w http.ResponseWriter, req *http.Request) {
		accountID := chi.URLParam(req, "accountID")

		var newCfg domain.ProfitCfgSet
		if !utils.DecodeJSON(w, req, &newCfg) {
			return
		}
		if newCfg.TakeProfit.Enabled && newCfg.StopGain.Enabled {
			utils.ErrJSON(w, req, 400, "profit_cfg_invalid", errors.New("Apenas uma estrategia pode estar habilitada. (takeprofit/stopgain)"))
			return
		}

		// Atualiza timestamps (controlado pelo backend)
		now := time.Now().Unix()
		newCfg.TakeProfit.UpdatedAt = now
		newCfg.StopGain.UpdatedAt = now

		// Configuração anterior
		prevCfg := profitSvc.GetConfig(req.Context(), accountID)

		// Upsert da nova configuração
		if err := profitSvc.SetConfig(req.Context(), accountID, newCfg); err != nil {
			utils.ErrJSON(w, req, 400, "profit_cfg_invalid", err)
			return
		}

		var (
			applied bool
			action  string
		)

		// Aplica estratégia ativa se necessário
		switch {
		case newCfg.TakeProfit.Enabled && ShouldApplyProfit(prevCfg.TakeProfit, newCfg.TakeProfit):
			action = "takeprofit:apply"
			if err := profitSvc.ApplyTakeProfit(req.Context(),
				domain.TakeProfitRequest{
					AccountID: accountID,
					ProfitCfg: newCfg.TakeProfit,
				}); err != nil {
				utils.ErrJSON(w, req, 409, "takeprofit_conflict", err)
				return
			}
			applied = true

		case newCfg.StopGain.Enabled && ShouldApplyProfit(prevCfg.StopGain, newCfg.StopGain):
			action = "stopgain:apply"
			if err := profitSvc.ApplyStopGainToOpenOrders(req.Context(), accountID, newCfg.StopGain); err != nil {
				utils.ErrJSON(w, req, 500, "stopgain_apply_failed", err)
				return
			}
			applied = true
		}

		// JSON de resposta
		resp := struct {
			domain.ProfitCfgSet `json:",inline"`
			Meta                map[string]any `json:"_meta"`
		}{
			ProfitCfgSet: newCfg,
			Meta: map[string]any{
				"applied":    applied,
				"action":     action,
				"account_id": accountID,
				"previous":   prevCfg,
			},
		}

		utils.WriteJSON(w, http.StatusOK, resp)
	})
}

func ShouldApplyProfit(prev, cur domain.ProfitCfg) bool {
	// se desabilitado, não aplica
	if !cur.Enabled {
		log.Printf("[profit] Nao foi preciso aplicar alteracoes nos valores das ordens.")
		return false
	}

	if prev.Type != cur.Type {
		return true
	}

	switch cur.Type {
	case domain.ProfitTakeProfit:
		if !utils.AlmostEqual(prev.Percent, cur.Percent) {
			return true
		}
		log.Printf("[takeprofit] Nao foi preciso aplicar alteracoes nos valores das ordens.")
		return false
	case domain.ProfitStopGain:
		if !utils.AlmostEqual(prev.Percent, cur.Percent) {
			return true
		}
		if !utils.AlmostEqual(prev.IntervalUSD, cur.IntervalUSD) {
			return true
		}
		log.Printf("[stopgain] Nao foi preciso aplicar alteracoes nos valores das ordens.")
		return false
	default:
		return false
	}

}
