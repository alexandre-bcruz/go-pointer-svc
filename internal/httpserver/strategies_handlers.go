// internal/httpserver/stopgain_handlers.go
package httpserver

import (
	"net/http"

	"pointersvc/internal/domain"
	"pointersvc/internal/service"
	"pointersvc/internal/utils"

	"github.com/go-chi/chi/v5"
)

func RegisterStrategiesRoutes(r *chi.Mux, ps *service.ProfitService, ms *service.MarginService) {
	r.Post("/pointer/orders/{order_id}/stopgain:rebase", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "order_id")
		var body domain.StopGainRebaseRequest
		if !utils.DecodeJSON(w, req, &body) {
			return
		}

		resp, err := ps.StopgainRebase(req.Context(), id, body)
		if err != nil {
			if err == service.ErrOptimisticLock {
				utils.ErrJSON(w, req, 409, "optimistic_lock_failed", err)
				return
			}
			utils.ErrJSON(w, req, 400, "rebase_failed", err)
			return
		}
		utils.WriteJSON(w, 200, resp)
	})

	r.Post("/pointer/orders/{order_id}/stopgain:advance", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "order_id")
		var body domain.StopGainAdvanceRequest
		if !utils.DecodeJSON(w, req, &body) {
			return
		}

		resp, err := ps.StopgainAdvance(req.Context(), id, body)
		if err != nil {
			if err == service.ErrOptimisticLock {
				utils.ErrJSON(w, req, 409, "optimistic_lock_failed", err)
				return
			}
			utils.ErrJSON(w, req, 400, "advance_failed", err)
			return
		}
		utils.WriteJSON(w, 200, resp)
	})

	r.Post("/pointer/orders/{order_id}/margin:advance", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "order_id")
		var body domain.MarginRebaseRequest
		if !utils.DecodeJSON(w, req, &body) {
			return
		}

		resp, err := ms.RebaseMargin(req.Context(), id, body)
		if err != nil {
			if err == service.ErrOptimisticLock {
				utils.ErrJSON(w, req, 409, "optimistic_lock_failed", err)
				return
			}
			utils.ErrJSON(w, req, 400, "advance_failed", err)
			return
		}
		utils.WriteJSON(w, 200, resp)
	})
}
