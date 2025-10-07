package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"pointersvc/internal/domain"
	"pointersvc/internal/repo"
	"pointersvc/internal/utils"
	"strings"
)

type MarginService struct{ r *repo.RedisRepo }

func NewMarginService(r *repo.RedisRepo) *MarginService { return &MarginService{r: r} }

func (s *MarginService) SetMarginConfig(ctx context.Context, accountID string, marginCfg domain.MarginCfg) error {
	return s.r.SetMarginCfg(ctx, accountID, marginCfg)
}

func (s *MarginService) GetMarginConfig(ctx context.Context, accountID string) domain.MarginCfg {
	cfg, err := s.r.GetMarginCfg(ctx, accountID)
	if err != nil {
		log.Println("Configuracao de margem nao encontrada.")
		return domain.MarginCfg{}
	}
	return cfg
}

func (s *MarginService) ApplyMarginToOpenOrders(ctx context.Context, accountID string, cfg domain.MarginCfg) ([]string, error) {
	applied := make([]string, 0, 16)

	ids, err := s.r.ListAccountOrderIDs(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("listar ids da conta: %w", err)
	}
	if len(ids) == 0 {
		return applied, nil
	}

	orders, err := s.r.GetOrdersByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("carregar ordens por ids: %w", err)
	}

	for _, ord := range orders {
		if ord.AccountID != accountID {
			continue
		}
		if ord.Operation.Status != domain.StatusExecuted {
			continue
		}
		if ord.Operation.Side != domain.SideBuy && ord.Operation.Side != domain.SideSell {
			continue
		}
		L := ord.Operation.LiqPriceUSD
		if L <= 0 {
			continue
		}

		// ✅ Single source of truth for trigger calc (correct formula + directional rounding)
		trigger := utils.CalcMarginTriggerDirectional(L, string(ord.Operation.Side), cfg.ThresholdValPct)
		if trigger <= 0 {
			continue
		}

		// Mirror into object and persist + reindex
		ord.Margin.Enabled = cfg.Enabled
		ord.Margin.ThresholdValPct = cfg.ThresholdValPct
		ord.Margin.ExtraMarginPct = cfg.ExtraMarginPct
		ord.Margin.MarginTrigger = trigger

		if err := s.r.UpsertOrderMarginTrigger(ctx, ord, trigger); err != nil {
			log.Printf("[margin] upsert trigger error account_id=%s order_id=%s err=%v",
				accountID, ord.ID, err)
			continue
		}
		applied = append(applied, ord.ID)
	}

	log.Printf("[margin] applied triggers account_id=%s count=%d", accountID, len(applied))
	return applied, nil
}

func (s *MarginService) RebaseMargin(ctx context.Context, orderID string, body domain.MarginRebaseRequest) (domain.MarginRebaseResponse, error) {
	var out domain.MarginRebaseResponse

	// 1) load order
	ord, err := s.r.GetOrder(ctx, orderID)
	if err != nil {
		return out, fmt.Errorf("rebase_margin: ordem %s não encontrada: %w", orderID, err)
	}
	if ord.ID == "" || ord.AccountID == "" {
		return out, fmt.Errorf("rebase_margin: ordem inválida (id/account vázio)")
	}
	sideStr := strings.ToLower(string(ord.Operation.Side))
	if sideStr != "buy" && sideStr != "sell" {
		return out, fmt.Errorf("rebase_margin: side inválido: %s", sideStr)
	}

	// 2) choose inputs
	liq := ord.Operation.LiqPriceUSD
	if body.LiquidationUSD != nil && *body.LiquidationUSD > 0 {
		liq = *body.LiquidationUSD
	}
	if liq <= 0 {
		return out, fmt.Errorf("rebase_margin: liquidation_usd ausente/<=0")
	}

	thr := ord.Margin.ThresholdValPct
	if body.ThresholdValPct != nil && *body.ThresholdValPct > 0 {
		thr = *body.ThresholdValPct
	}
	if thr <= 0 {
		return out, fmt.Errorf("rebase_margin: threshold_val_pct ausente/<=0")
	}

	extra := ord.Margin.ExtraMarginPct
	if body.ExtraMarginPct != nil && !math.IsNaN(*body.ExtraMarginPct) {
		extra = *body.ExtraMarginPct
	}

	prev := ord.Margin.MarginTrigger

	// 3) recompute trigger using the SAME helper
	newTrigger := utils.CalcMarginTriggerDirectional(liq, string(ord.Operation.Side), thr)
	if newTrigger <= 0 {
		return out, fmt.Errorf("rebase_margin: trigger calculado inválido: %.2f", newTrigger)
	}

	// 4) update fields
	ord.Margin.ThresholdValPct = thr
	ord.Margin.ExtraMarginPct = extra
	ord.Margin.MarginTrigger = newTrigger
	if !ord.Margin.Enabled {
		ord.Margin.Enabled = true
	}

	// 5) persist + reindex
	if err := s.r.UpsertOrderMarginTrigger(ctx, ord, newTrigger); err != nil {
		return out, fmt.Errorf("rebase_margin: persistência/indexação falhou: %w", err)
	}

	// 6) response
	out = domain.MarginRebaseResponse{
		OrderID:         ord.ID,
		AccountID:       ord.AccountID,
		Side:            ord.Operation.Side,
		LiquidationUSD:  liq,
		ThresholdValPct: thr,
		ExtraMarginPct:  extra,
		PrevTrigger:     prev,
		NewTrigger:      newTrigger,
		Updated:         newTrigger != prev,
	}
	return out, nil
}