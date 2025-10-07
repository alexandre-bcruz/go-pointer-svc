// internal/service/query.go
package service

import (
	"context"
	"fmt"
	"log"
	"math"

	"pointersvc/internal/client"
	"pointersvc/internal/domain"
	"pointersvc/internal/repo"
	"pointersvc/internal/utils"
)

type QueryService struct {
	r        *repo.RedisRepo
	cli      *client.LNExchangeClient
	entrySvc *EntryService
}

func NewQueryService(r *repo.RedisRepo, cli *client.LNExchangeClient, entrySvc *EntryService) *QueryService {
	return &QueryService{r: r, cli: cli, entrySvc: entrySvc}
}

func (s *QueryService) GetAccountConfigs(ctx context.Context, acct string) (map[string]any, error) {
	entryBuy, _ := s.r.GetEntryCfgBySide(ctx, acct, string(domain.SideBuy))
	entrySell, _ := s.r.GetEntryCfgBySide(ctx, acct, string(domain.SideSell))
	profit, _ := s.r.GetProfitCfg(ctx, acct)
	margin, _ := s.r.GetMarginCfg(ctx, acct)

	return map[string]any{
		"entry_cfg_buy":  entryBuy,
		"entry_cfg_sell": entrySell,
		"profit_cfg":     profit,
		"margin_rule":    margin,
	}, nil
}

/* ---------- per-order config upserts ---------- */

func (s *QueryService) SetOrderProfitCfg(ctx context.Context, orderID string, req domain.ProfitCfgSet) (domain.ProfitCfgSet, error) {
	return s.r.SetOrderProfitCfg(ctx, orderID, req)
}

func (s *QueryService) SetOrderMarginCfg(ctx context.Context, orderID string, req domain.MarginCfg) (domain.MarginCfg, error) {
	return s.r.SetOrderMarginCfg(ctx, orderID, req)
}

func (s *QueryService) GetOrderMarginCfg(ctx context.Context, orderID string) (domain.MarginCfg, error) {
	return s.r.GetOrderMarginCfg(ctx, orderID)
}

func (s *QueryService) GetOrderProfitCfg(ctx context.Context, orderID string) (domain.ProfitCfgSet, error) {
	return s.r.GetOrderProfitCfg(ctx, orderID)
}

/* ---------- orders list (somente GS/automation) ---------- */

// ListOrders: lista ordens GS (automation) a partir de order:idx:{accountID}.
// Lê a ordem em JSON (domain.Order), aplica EnrichProfit/EnrichMargin com os próprios
// campos da ordem e retorna a lista de domain.Order (já pronta para a API).
func (s *QueryService) ListOrders(ctx context.Context, accountID string) ([]domain.Order, error) {
	ordersLn, err := s.cli.GetTradesByAccount(ctx, accountID, "closed")
	if err != nil {
		log.Printf("Erro ao consultar as ordens da LN. %s %s ", accountID, err)
		ordersLn = domain.LNOrders{}
	}

	// ✅ Carrega configurações separadas por lado
	buyCfg, _ := s.r.GetEntryCfgBySide(ctx, accountID, string(domain.SideBuy))
	sellCfg, _ := s.r.GetEntryCfgBySide(ctx, accountID, string(domain.SideSell))

	// ✅ Gera lista + detecta se precisa rebuildar
	out := s.createList(ctx, accountID, ordersLn, buyCfg, sellCfg)

	return out, nil
}

func checkIfClosed(orders []domain.Order, id string) bool {
	for _, ord := range orders {
		if ord.BrokerOrderID == id {
			return true
		}
	}
	return false
}

func (s *QueryService) createList(
	ctx context.Context,
	accountID string,
	ordersLn domain.LNOrders,
	buyCfg, sellCfg domain.EntryUpsertCfg,
) ([]domain.Order) {

	ids, _ := s.r.ListAccountOrderIDs(ctx, accountID)
	out := make([]domain.Order, 0, len(ids))

	for _, id := range ids {
		ord, err := s.r.GetOrder(ctx, id)
		if err != nil {
			continue // ignora órfãos/corrompidos
		}



		profitSetOrder, _ := s.r.GetOrderProfitCfg(ctx, id)
		profitSetAccount, _ := s.r.GetProfitCfg(ctx, accountID)

		profitSet, apply := ChooseBestProfit(profitSetOrder, profitSetAccount)
		if apply {
			ord.EnrichProfit(profitSet)
		}

		// Entry (aplica config por lado se for GS e pending)
		if ord.Source == domain.SourceGS && ord.Operation.Status == domain.StatusPending {
			var cfg domain.EntryUpsertCfg
			switch ord.Operation.Side {
			case domain.SideBuy:
				cfg = buyCfg
			case domain.SideSell:
				cfg = sellCfg
			}
			ord.Entry.Enabled = cfg.Enabled
			ord.Entry.ContractsQuantity = cfg.ContractsQuantity
			ord.Entry.Fingerprint = cfg.Fingerprint
		}

		// Margin (prioriza config por ordem > global)
		pCfgMargin, _ := s.r.GetOrderMarginCfg(ctx, id)
		if pCfgMargin.Enabled {
			ord.EnrichMargin(pCfgMargin)
		} else {
			mCfg, _ := s.r.GetMarginCfg(ctx, accountID)
			if mCfg.Enabled {
				ord.EnrichMargin(mCfg)
				mCfg.MarginTrigger = utils.CalcMarginTriggerDirectional(
					ord.Operation.LiqPriceUSD,
					string(ord.Operation.Side),
					ord.Margin.ThresholdValPct,
				)
			}
		}


		// Remove da lista se já fechou na LN
		if ord.Operation.Status == domain.StatusExecuted &&
			checkIfClosed(ordersLn.Closed, ord.BrokerOrderID) {
			log.Printf("Ordem %s foi fechada na LN!", ord.ID)
			ord.Operation.Status = domain.StatusPending
			ord.Operation.ExecutedAt = ""
			ord.BrokerOrderID = ""
			s.r.UpdateOrder(ctx, ord)
			continue
		}

		out = append(out, ord)
	}

	return out
}

// ---------- Entry (grid) ----------
func (s *QueryService) ListEntryTriggers(
	ctx context.Context,
	side string,
	accountID *string,
	min, max *float64,
	limit int64,
	desc bool,
) ([]domain.TriggerItem, error) {

	var key string
	if accountID != nil && *accountID != "" {
		key = repo.KeyEntryTriggerZ(*accountID, side)
	} else {
		key = repo.KeyEntryTriggerAll(side)
	}
	return s.listZWithScores(ctx, key, min, max, limit, desc)
}

// ---------- Margin (risk) ----------
func (s *QueryService) ListMarginTriggers(
	ctx context.Context,
	accountID *string,
	min, max *float64,
	limit int64,
	desc bool,
) ([]domain.TriggerItem, error) {

	var key string
	if accountID != nil && *accountID != "" {
		key = repo.KeyRiskZ(*accountID)
	} else {
		key = repo.KeyRiskAll()
	}
	return s.listZWithScores(ctx, key, min, max, limit, desc)
}

// ---------- StopGain ----------
func (s *QueryService) ListStopGainTriggers(
	ctx context.Context,
	accountID *string,
	side *string, // opcional: buy|sell para o global
	min, max *float64,
	limit int64,
	desc bool,
) ([]domain.TriggerItem, error) {

	var key string
	if accountID != nil && *accountID != "" {
		key = repo.KeyStopGainZ(*accountID)
	} else if side != nil && (*side == "buy" || *side == "sell") {
		key = repo.KeyStopGainAllSide(*side)
	} else {
		key = repo.KeyStopGainAll()
	}
	return s.listZWithScores(ctx, key, min, max, limit, desc)
}

/* ================= helpers ================= */

func (s *QueryService) listZWithScores(
	ctx context.Context,
	key string,
	min, max *float64,
	limit int64,
	desc bool,
) ([]domain.TriggerItem, error) {

	minStr := "-inf"
	maxStr := "+inf"
	if min != nil {
		minStr = fmt.Sprintf("%.0f", math.Round(*min))
	}
	if max != nil {
		maxStr = fmt.Sprintf("%.0f", math.Round(*max))
	}
	if limit <= 0 {
		limit = 200
	}

	items, err := s.r.ListPointersFiltered(ctx, key, minStr, maxStr, limit, desc)
	if err != nil {
		return nil, err
	}

	return items, nil
}
