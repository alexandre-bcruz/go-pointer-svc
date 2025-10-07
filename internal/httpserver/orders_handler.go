package httpserver

import (
	"net/http"
	"pointersvc/internal/domain"
	"pointersvc/internal/repo"
	"pointersvc/internal/service"
	"pointersvc/internal/utils"

	"github.com/go-chi/chi/v5"
)

func RegisterOrderRoutes(r *chi.Mux, redis *repo.RedisRepo, profitSvc *service.ProfitService, qs *service.QueryService) {

	openedSvc := service.NewOpenedService(redis, profitSvc)

	// Ordem aberta
	r.Put("/pointer/orders/{orderID}/opened", func(w http.ResponseWriter, req *http.Request) {
		orderID := chi.URLParam(req, "orderID")
		var body domain.OrderOpened
		if !utils.DecodeJSON(w, req, &body) {
			return
		}
		res, err := openedSvc.Handle(req.Context(), orderID, body)
		if err != nil {
			utils.ErrJSON(w, req, 500, "orders_opened_failed", err)
			return
		}
		utils.WriteJSON(w, 200, res)
	})

	r.Get("/pointer/orders/{orderID}/configs", func(w http.ResponseWriter, req *http.Request) {
		orderID := chi.URLParam(req, "orderID")

		profit, _ := qs.GetOrderProfitCfg(req.Context(), orderID)
		margin, _ := qs.GetOrderMarginCfg(req.Context(), orderID)
		utils.WriteJSON(w, 200, map[string]any{"profit_cfg": profit, "margin_cfg": margin})
	})

	r.Put("/pointer/orders/{orderID}/profit", func(w http.ResponseWriter, req *http.Request) {
		orderID := chi.URLParam(req, "orderID")

		var body domain.ProfitCfgSet
		if !utils.DecodeJSON(w, req, &body) {
			return
		}
		profit, err := qs.SetOrderProfitCfg(req.Context(), orderID, body)
		if err != nil {
			utils.ErrJSON(w, req, 500, "order_profit_upsert_failed", err)
			return
		}
		utils.WriteJSON(w, http.StatusOK, profit)
	})

	r.Put("/pointer/orders/{orderID}/margin", func(w http.ResponseWriter, req *http.Request) {
		orderID := chi.URLParam(req, "orderID")

		var body domain.MarginCfg
		if !utils.DecodeJSON(w, req, &body) {
			return
		}
		margin, err := qs.SetOrderMarginCfg(req.Context(), orderID, body)
		if err != nil {
			utils.ErrJSON(w, req, 500, "order_margin_upsert_failed", err)
			return
		}
		utils.WriteJSON(w, http.StatusOK, margin)
	})

	r.Get("/pointer/accounts/{accountID}/orders", func(w http.ResponseWriter, req *http.Request) {
		acct := chi.URLParam(req, "accountID")
		list, err := qs.ListOrders(req.Context(), acct)
		if err != nil {
			utils.ErrJSON(w, req, 500, "orders_list_failed", err)
			return
		}
		utils.WriteJSON(w, 200, list)
	})

}
