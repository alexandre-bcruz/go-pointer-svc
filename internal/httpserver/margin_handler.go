package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"pointersvc/internal/domain"
	"pointersvc/internal/repo"
	"pointersvc/internal/service"
	"pointersvc/internal/utils"
)

func RegisterMarginRoutes(r *chi.Mux, redis *repo.RedisRepo, marginSvc *service.MarginService) {

	r.Put("/pointer/accounts/{accountID}/margin", func(w http.ResponseWriter, req *http.Request) {
		accountID := chi.URLParam(req, "accountID")

		var body domain.MarginCfg
		if !utils.DecodeJSON(w, req, &body) {
			return
		}

		// lê a configuração anterior antes de persistir a nova
		prev := marginSvc.GetMarginConfig(req.Context(), accountID)

		// upsert da config
		if err := marginSvc.SetMarginConfig(req.Context(), accountID, body); err != nil {
			utils.ErrJSON(w, req, http.StatusInternalServerError, "margin_rule_upsert_failed", err)
			return
		}

		// aplica somente se houve mudança material
		if ShouldApplyMargin(prev, body) {
			ids, err := marginSvc.ApplyMarginToOpenOrders(req.Context(), accountID, body)
			if err != nil {
				utils.ErrJSON(w, req, http.StatusInternalServerError, "margin_apply_failed", err)
				return
			}
			utils.WriteJSON(w, http.StatusOK, map[string]any{
				"account_id":    accountID,
				"applied":       true,
				"applied_count": len(ids),
				"affected_ids":  ids, // se não quiser listar, remova esta linha
				"previous":      prev,
				"current":       body, // corpo padronizado com o req
			})
			return
		}

		// nada a aplicar (apenas salvou/ligou/desligou sem mudar valores)
		utils.WriteJSON(w, http.StatusOK, map[string]any{
			"account_id": accountID,
			"applied":    false,
			"previous":   prev,
			"current":    body, // ecoa o req
		})
	})
}

// Só dispara aplicação quando houver mudança MATERIAL nos parâmetros.
// - Não aplica se apenas liga/desliga enabled.
// - Aplica se ThresholdValPct ou ExtraMarginPct mudarem.
func ShouldApplyMargin(prev, cur domain.MarginCfg) bool {
    if prev.Enabled != cur.Enabled { return true }
    if !utils.AlmostEqual(prev.ThresholdValPct, cur.ThresholdValPct) { return true }
    if !utils.AlmostEqual(prev.ExtraMarginPct, cur.ExtraMarginPct) { return true }
    return false
}