package service

import (
	"context"
	"errors"
	"fmt"
	"pointersvc/internal/domain"
	"pointersvc/internal/utils"
	"time"
)

// ApplyStopGainToOpenOrders: aplica stopgain às ordens abertas que tenham dados mínimos.
func (s *ProfitService) ApplyStopGainToOpenOrders(ctx context.Context, accountID string, cfg domain.ProfitCfg) error {
	orders, err := s.querySvc.ListOrders(ctx, accountID)
	if err != nil {
		return err
	}

	created := make([]string, 0, len(orders))
	for _, ov := range orders {
		if ov.BrokerOrderID == "" || ov.Operation.PriceUSD <= 0 || ov.Operation.Status != domain.StatusExecuted {
			continue
		}
		id, err := s.StopGain(ctx, cfg, ov)
		if err == nil && id != "" {
			created = append(created, id)
		}
	}

	utils.InfoJ(ctx, "profit.stopgain.batch.completed",
		"account_id", accountID,
		"created_count", len(created),
	)
	return nil
}

// StopGain: cria o ponteiro inicial (gatilho) segundo a sua regra.
func (s *ProfitService) StopGain(
	ctx context.Context,
	cfg domain.ProfitCfg,
	order domain.Order,
) (string, error) {

	order.Profit.Enabled = cfg.Enabled
	order.Profit.Percent = cfg.Percent
	order.Profit.Type = domain.ProfitStopGain

	target, stoploss := utils.CalcStopGainTargets(order.Operation.PriceUSD, cfg.Percent, cfg.IntervalUSD, string(order.Operation.Side), utils.DaysSinceNow(order.Operation.ExecutedAt))
	order.Profit.StopLossUSD = stoploss
	order.Profit.TargetUSD = target

	if err := s.r.SaveStopGainPointer(ctx, order, target); err != nil {
		return "", err
	}

	return order.ID, nil
}

func (s *ProfitService) SetConfig(ctx context.Context, accountID string, set domain.ProfitCfgSet) error {
	// Atualiza timestamps
	now := time.Now().Unix()
	set.TakeProfit.UpdatedAt = now
	set.StopGain.UpdatedAt = now

	// Salva cada config separadamente
	if err := s.r.SetProfitCfgByType(ctx, accountID, string(set.TakeProfit.Type), set.TakeProfit); err != nil {
		return fmt.Errorf("SetConfig (TakeProfit): %w", err)
	}
	if err := s.r.SetProfitCfgByType(ctx, accountID, string(set.StopGain.Type), set.StopGain); err != nil {
		return fmt.Errorf("SetConfig (StopGain): %w", err)
	}

	// Enriquecimento das ordens com ambas estratégias (se estiverem ativas)
	ids, _ := s.r.ListAccountOrderIDs(ctx, accountID)
	orders, _ := s.r.GetOrdersByIDs(ctx, ids)
	for _, ord := range orders {
		ord.EnrichProfit(set)
		_ = s.r.SaveOrder(ctx, ord, nil)
	}

	return nil
}


func (s *ProfitService) StopgainRebase(ctx context.Context, orderID string, req domain.StopGainRebaseRequest) (domain.StopGainRebaseResponse, error) {
	var out domain.StopGainRebaseResponse

	ord, err := s.r.GetOrder(ctx, orderID)
	if err != nil {
		return out, err
	}
	if !ord.Profit.Enabled || ord.Profit.Type != domain.ProfitStopGain {
		return out, errors.New("order not stopgain-enabled")
	}
	if ord.Operation.Status != domain.StatusExecuted {
		return out, errors.New("order not executed")
	}

	// optimistic lock on current target (rounded to your integer grid)
	curr := utils.Round(ord.Profit.TargetUSD, 0)
	if req.PrevTargetUSD > 0 && curr != utils.Round(req.PrevTargetUSD, 0) {
		return out, ErrOptimisticLock
	}

	// Decide new target/stop:
	// - prefer explicit values from request when provided (>0)
	// - otherwise recompute from config (backwards compatibility)
	var newTarget = utils.Round(req.NewTargetUSD, 0)
	newStop := req.NewStopUSD

	if newTarget <= 0 {
		days := utils.DaysSinceCreated(ord.Operation.ExecutedAt, time.Now())
		entry := ord.Operation.PriceUSD
		interval := ord.Profit.IntervalUSD
		percent := ord.Profit.Percent
		tgt, stp := utils.CalcStopGainTargets(entry, percent, interval, string(ord.Operation.Side), days)
		newTarget = utils.Round(tgt, 0)
		if newStop <= 0 {
			newStop = stp
		}
	} else {
		// If only NewTargetUSD was provided but not NewStopUSD, keep current stop unless you want to recompute here.
		if newStop <= 0 {
			newStop = ord.Profit.StopLossUSD
		}
	}

	out.OrderID = ord.ID
	out.Action = "rebase"
	out.Old = domain.ProfitEdge{TargetUSD: curr, StopLossUSD: ord.Profit.StopLossUSD}
	out.New = domain.ProfitEdge{TargetUSD: newTarget, StopLossUSD: newStop}

	// Persist
	ord.Profit.TargetUSD = newTarget
	ord.Profit.StopLossUSD = newStop
	if err := s.r.SaveOrder(ctx, ord, nil); err != nil {
		return out, err
	}
	out.Persisted = true

	// Reindex ZSETs
	if err := s.r.ReindexStopGain(ctx, ord.AccountID, ord.ID, newTarget, string(ord.Operation.Side), nil); err != nil {
		return out, err
	}
	out.ZSetsUpdated = true
	return out, nil
}

func (s *ProfitService) StopgainAdvance(ctx context.Context, orderID string, req domain.StopGainAdvanceRequest) (domain.StopGainAdvanceResponse, error) {
	var out domain.StopGainAdvanceResponse

	ord, err := s.r.GetOrder(ctx, orderID)
	if err != nil {
		return out, err
	}
	if !ord.Profit.Enabled || ord.Profit.Type != domain.ProfitStopGain {
		return out, errors.New("order not stopgain-enabled")
	}
	if ord.Operation.Status != domain.StatusExecuted {
		return out, errors.New("order not executed")
	}

	// optimistic lock on current target
	curr := utils.Round(ord.Profit.TargetUSD, 0)
	if curr == 0 {
		return out, errors.New("no current target on order")
	}
	if req.PrevTargetUSD > 0 && curr != utils.Round(req.PrevTargetUSD, 0) {
		return out, ErrOptimisticLock
	}

	// Determine next target:
	nextTarget := utils.Round(req.NextTargetUSD, 0)
	if nextTarget <= 0 {
		// derive from interval and side (backwards compatibility)
		interval := ord.Profit.IntervalUSD
		side := ord.Operation.Side
		if side == domain.SideBuy {
			nextTarget = utils.Round(curr+interval, 0)
		} else {
			nextTarget = utils.Round(curr-interval, 0)
		}
	}

	// Determine stop to persist after advance.
	// If the client computed a new trailing stop for the next window, accept it; else keep current.
	newStop := req.NewStopUSD
	if newStop <= 0 {
		newStop = ord.Profit.StopLossUSD
	}

	out.OrderID = ord.ID
	out.Action = "advance"
	out.Old = domain.ProfitEdge{TargetUSD: curr, StopLossUSD: ord.Profit.StopLossUSD}
	out.New = domain.ProfitEdge{TargetUSD: nextTarget, StopLossUSD: newStop}

	// Persist next target/stop
	ord.Profit.TargetUSD = nextTarget
	ord.Profit.StopLossUSD = newStop
	if err := s.r.SaveOrder(ctx, ord, nil); err != nil {
		return out, err
	}
	out.Persisted = true

	// Reindex ZSETs to the next target
	if err := s.r.ReindexStopGain(ctx, ord.AccountID, ord.ID, nextTarget, string(ord.Operation.Side), nil); err != nil {
		return out, err
	}
	out.ZSetsUpdated = true
	return out, nil
}