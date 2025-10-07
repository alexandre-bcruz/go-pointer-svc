// internal/service/entry.go
package service

import (
	"context"
	"log"

	"github.com/shopspring/decimal"

	"pointersvc/internal/domain"
	"pointersvc/internal/repo"
	"pointersvc/internal/utils"
)

type EntryService struct{ r *repo.RedisRepo }

func NewEntryService(r *repo.RedisRepo) *EntryService { return &EntryService{r: r} }

// PutCfg: mantém GS executed, apaga GS pending em todos os índices e recria faltantes.
func (s *EntryService) PutCfg(ctx context.Context, req domain.EntryUpsertCfg, accountID string) (domain.EntryPutResult, error) {
	var out domain.EntryPutResult
	side := string(req.Side)

	prevCfg, _ := s.r.GetEntryCfgBySide(ctx, accountID, string(req.Side))
	newFp := utils.GridFingerprint(side, req.Enabled, req.InitialPriceUSD, req.PriceIntervalUSD, req.OrderValueUSD,req.ContractsQuantity)
	req.Fingerprint = newFp

	if err := s.r.UpsertEntryCfg(ctx, accountID, req); err != nil {
		return out, err
	}

	// Regra principal: rebuild se fingerprint mudou
	needRebuild := (prevCfg.Fingerprint == "" || prevCfg.Fingerprint != newFp)

	// ✅ Regra adicional: rebuild se estiver faltando ponteiro (pending + executed < qty)
	pendingIDs, _ := s.r.ListAccountOrderIDsBySideAndStatus(ctx, accountID, side, string(domain.StatusPending))
	executedIDs, _ := s.r.ListAccountOrderIDsBySideAndStatus(ctx, accountID, side, string(domain.StatusExecuted))

	total := 0
	for _, id := range append(pendingIDs, executedIDs...) {
		order, err := s.r.GetOrder(ctx, id)
		if err != nil || order.Source != domain.SourceGS {
			continue
		}
		total++
	}
	if total < req.ContractsQuantity {
		log.Printf("⚠️  Rebuild forçado: %d ordens GS existentes (pending+executed), esperado: %d", total, req.ContractsQuantity)
		needRebuild = true
	}

	// Resposta (metadados da grade)
	minPrice, maxPrice := req.InitialPriceUSD, req.InitialPriceUSD
	if req.ContractsQuantity > 0 && req.PriceIntervalUSD >= 0 {
		step := req.PriceIntervalUSD
		// if req.Side == domain.SideBuy {
		minPrice = req.InitialPriceUSD - step*float64(req.ContractsQuantity-1)
		maxPrice = req.InitialPriceUSD
		// } else {
		// 	minPrice = req.InitialPriceUSD
		// 	maxPrice = req.InitialPriceUSD + step*float64(req.ContractsQuantity-1)
		// }
	}

	out.Fingerprint = newFp
	out.Rebuild = needRebuild
	out.Grid.Count = req.ContractsQuantity
	out.Grid.InitialPriceUSD = req.InitialPriceUSD
	out.Grid.IntervalUSD = req.PriceIntervalUSD
	out.Grid.MinPriceUSD = minPrice
	out.Grid.MaxPriceUSD = maxPrice

	if !needRebuild || req.ContractsQuantity <= 0 {
		return out, nil
	}

	// --- REBUILD ---
	levels := makeGridLevels(req.Side, req.InitialPriceUSD, req.PriceIntervalUSD, req.ContractsQuantity)

	occ, _, execGS, err := s.r.RebuildEntryGridSweepV2(ctx, accountID, side, levels, req.PriceIntervalUSD)
	if err != nil {
		return out, err
	}

	needed := req.ContractsQuantity - execGS
	if needed <= 0 {
		return out, nil
	}

	created := 0
	for i, price := range levels {
		if _, taken := occ[i]; taken {
			continue
		}

		order, err := s.r.NewEntryOrder(accountID, domain.OrderOp{
			PriceUSD:      price,
			OrderValueUSD: req.OrderValueUSD,
			MarginSats:    req.OrderMarginSats,
			Leverage:      req.Leverage,
			Side:          req.Side,
			Status:        domain.StatusPending,
		})
		if err != nil {
			return out, err
		}

		profitCfg, marginCfg := s.loadAccountFlags(ctx, accountID)
		order.EnrichProfit(profitCfg)
		order.EnrichMargin(marginCfg)
		order.Entry.Enabled = req.Enabled
		order.Entry.ContractsQuantity = req.ContractsQuantity

		if err := s.r.SaveOrderAndIndex(ctx, order); err != nil {
			return out, err
		}

		created++
		if created >= needed {
			break
		}
	}

	return out, nil
}

// helpers

func (s *EntryService) loadAccountFlags(ctx context.Context, acct string) (domain.ProfitCfgSet, domain.MarginCfg) {
	profitCfg, _ := s.r.GetProfitCfg(ctx, acct)
	marginCfg, _ := s.r.GetMarginCfg(ctx, acct)
	return profitCfg, marginCfg
}

func makeGridLevels(side domain.Side, initial, step float64, qty int) []float64 {
	if qty <= 0 || step <= 0 {
		return nil
	}
	levels := make([]float64, qty)
	base := decimal.NewFromFloat(initial)
	dstep := decimal.NewFromFloat(step)
	for i := 0; i < qty; i++ {
		var lvl decimal.Decimal
		lvl = base.Sub(dstep.Mul(decimal.NewFromInt(int64(i))))
		levels[i] = lvl.InexactFloat64()
	}
	return levels
}
