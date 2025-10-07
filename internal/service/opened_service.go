// internal/service/opened.go
package service

import (
	"context"
	"errors"
	"fmt"
	"time"
	"github.com/redis/go-redis/v9"

	"pointersvc/internal/domain"
	"pointersvc/internal/repo"
)

type OpenedService struct {
	r         *repo.RedisRepo
	profitSvc *ProfitService // mantenha se precisar em outros fluxos
}

func NewOpenedService(r *repo.RedisRepo, profit *ProfitService) *OpenedService {
	return &OpenedService{r: r, profitSvc: profit}
}

// Handle: upsert da ordem executada (JSON), indexação e criação de gatilhos (se habilitado).
func (s *OpenedService) Handle(ctx context.Context, orderID string, o domain.OrderOpened) (map[string]string, error) {
	// 1) Validação mínima
	if o.Source == "" {
		return nil, errors.New("campo 'source' é obrigatório")
	}
	if orderID == "" || o.AccountID == "" {
		return nil, errors.New("orderID e account_id são obrigatórios")
	}

	// 2) Carrega configs de ordem e conta
	profitOrd, marginOrd := s.r.GetOrderConfigs(ctx, orderID)

	profitAcc, err := s.r.GetProfitCfg(ctx, o.AccountID)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("erro ao obter ProfitCfg da conta: %w", err)
	}

	marginAcc, err := s.r.GetMarginCfg(ctx, o.AccountID)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("erro ao obter MarginCfg da conta: %w", err)
	}

	// 3) Aplica lógica de herança automática da MELHOR estratégia disponível
	pCfgSet, applyP := ChooseBestProfit(profitOrd, profitAcc)
	mCfg, applyM := ChooseMargin(marginOrd, marginAcc)

	// 4) Determina qual das duas estratégias foi escolhida
	var selectedType domain.ProfitType

	switch {
	case pCfgSet.StopGain.Enabled:
		selectedType = domain.ProfitStopGain
	case pCfgSet.TakeProfit.Enabled:
		selectedType = domain.ProfitTakeProfit
	}

	// 5) Persiste a ProfitCfg aplicada (mesmo que herdada)
	if _, err := s.r.SetOrderProfitCfg(ctx, orderID, pCfgSet); err != nil {
		return nil, fmt.Errorf("erro ao persistir ProfitCfg por ordem: %w", err)
	}

	// 6) Determina instante de execução
	execAt := parseExecutedAt(o.ExecutedAt)

	// 7) Monta objeto Order canônico
	ord := s.makeExecutedOrder(orderID, o, execAt)

	// 8) Enriquecimento de dados
	if applyP {
		ord.Profit.Type = selectedType
		ord.EnrichProfit(pCfgSet)
	} else {
		ord.Profit.Enabled = false
	}

	if applyM {
		ord.EnrichMargin(mCfg)
	} else {
		ord.Margin.Enabled = false
	}

	// 9) Persiste ordem + indexa para listagem
	if err := s.handleOrderRepo(ctx, ord, execAt); err != nil {
		return nil, fmt.Errorf("erro ao salvar ordem: %w", err)
	}

	out := map[string]string{"order_id": orderID, "status": "executed"}

	// 10) Gatilhos automáticos
	if ord.Margin.Enabled {
		if err := s.r.UpsertOrderMarginTrigger(ctx, ord, ord.Margin.MarginTrigger); err != nil {
			return nil, fmt.Errorf("erro ao indexar gatilho de margem: %w", err)
		}
		out["risk_trigger_set"] = "1"
	}

	if ord.Profit.Enabled && ord.Profit.Type == domain.ProfitStopGain {
		if err := s.r.UpsertOrderStopgainTrigger(ctx, ord, ord.Profit.TargetUSD); err != nil {
			return nil, fmt.Errorf("erro ao indexar gatilho de stopgain: %w", err)
		}
		out["stopgain_trigger_set"] = "1"
	}

	return out, nil
}

/* ====================== helpers ====================== */

func parseExecutedAt(s string) time.Time {
	if s == "" {
		return time.Now().UTC()
	}
	// tenta RFC3339Nano, depois RFC3339; fallback = now
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return time.Now().UTC()
}


func (s *OpenedService) makeExecutedOrder(orderID string, o domain.OrderOpened, execAt time.Time) domain.Order {
	return domain.Order{
		ID:            orderID,
		AccountID:     o.AccountID,
		Source:        o.Source,
		BrokerOrderID: o.BrokerOrderID,
		Operation: domain.OrderOp{
			PriceUSD:      o.EntryPriceUSD,
			OrderValueUSD: o.OrderValueUSD,
			MarginSats:    o.InitialMarginSats,
			Leverage:      int(o.Leverage),
			Side:          o.Side,
			Status:        domain.StatusExecuted,
			LiqPriceUSD:   o.LiqPriceUSD,
			ExecutedAt:    execAt.Format(time.RFC3339Nano),
		},
	}
}

func (s *OpenedService) handleOrderRepo(ctx context.Context, ord domain.Order, execAt time.Time) error {
	pipe := s.r.Rdb().Pipeline()
	s.r.SaveOrder(ctx, ord, pipe)
	s.r.UnindexEntry(ctx, ord.AccountID, string(ord.Operation.Side), ord.ID, pipe)
	s.r.IndexOrderForListing(ctx, ord.AccountID, ord.ID, execAt)
	_, err := pipe.Exec(ctx)
	return err
}

func ChooseProfitSet(orderCfgSet, accountCfgSet domain.ProfitCfgSet, profitType string) (domain.ProfitCfgSet, bool) {
	var chosenCfg domain.ProfitCfg

	switch profitType {
	case string(domain.ProfitTakeProfit):
		if orderCfgSet.TakeProfit.Enabled {
			chosenCfg = orderCfgSet.TakeProfit
		} else if accountCfgSet.TakeProfit.Enabled {
			chosenCfg = accountCfgSet.TakeProfit
		}
	case string(domain.ProfitStopGain):
		if orderCfgSet.StopGain.Enabled {
			chosenCfg = orderCfgSet.StopGain
		} else if accountCfgSet.StopGain.Enabled {
			chosenCfg = accountCfgSet.StopGain
		}
	default:
		return domain.ProfitCfgSet{}, false
	}

	// Se não habilitado, retorna falso
	if !chosenCfg.Enabled {
		return domain.ProfitCfgSet{}, false
	}

	// Retorna um ProfitCfgSet com o tipo correto preenchido
	return domain.ProfitCfgSet{
		TakeProfit: ifThen(profitType == string(domain.ProfitTakeProfit), chosenCfg),
		StopGain:   ifThen(profitType == string(domain.ProfitStopGain), chosenCfg),
	}, true
}


func ChooseMargin(orderCfg, accountCfg domain.MarginCfg) (domain.MarginCfg, bool) {
	if orderCfg.Enabled {
		return orderCfg, true
	}
	if accountCfg.Enabled {
		return accountCfg, true
	}
	return domain.MarginCfg{}, false
}

func ifThen[T any](cond bool, val T) T {
	var zero T
	if cond {
		return val
	}
	return zero
}

func ChooseBestProfit(ordSet, accSet domain.ProfitCfgSet) (domain.ProfitCfgSet, bool) {
	if ordSet.StopGain.Enabled || accSet.StopGain.Enabled {
		return domain.ProfitCfgSet{
			TakeProfit: domain.ProfitCfg{},
			StopGain:   firstNonZeroProfit(ordSet.StopGain, accSet.StopGain),
		}, true
	}

	if ordSet.TakeProfit.Enabled || accSet.TakeProfit.Enabled {
		return domain.ProfitCfgSet{
			TakeProfit: firstNonZeroProfit(ordSet.TakeProfit, accSet.TakeProfit),
			StopGain:   domain.ProfitCfg{},
		}, true
	}

	return domain.ProfitCfgSet{}, false
}

func firstNonZeroProfit(a, b domain.ProfitCfg) domain.ProfitCfg {
	if a.Enabled {
		return a
	}
	if b.Enabled {
		return b
	}
	return domain.ProfitCfg{}
}