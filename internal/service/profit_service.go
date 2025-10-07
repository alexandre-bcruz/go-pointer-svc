// internal/service/profit.go
package service

import (
	"context"
	"errors"
	"log"

	"pointersvc/internal/client"
	"pointersvc/internal/domain"
	"pointersvc/internal/repo"
)

var ErrOptimisticLock = errors.New("optimistic_lock_failed")

type ProfitService struct {
	r        *repo.RedisRepo
	clientLN client.LNExchangeClient
	querySvc QueryService
}

func NewProfitService(r *repo.RedisRepo, clientLN *client.LNExchangeClient, querySvc *QueryService) *ProfitService {
	return &ProfitService{
		r:        r,
		clientLN: *clientLN,
		querySvc: *querySvc,
	}
}

/* -------------------------- operações de serviço -------------------------- */

// TakeProfit: grava cfg de take profit por conta (tipado; JSON).
func (s *ProfitService) ApplyTakeProfit(ctx context.Context, takeProfitReq domain.TakeProfitRequest) error {
	err := s.clientLN.ApplyTakeProfit(ctx, takeProfitReq)
	if err != nil {
		log.Printf("Erro ao aplicar take profit nas ordens da LN. %s %v %s ", takeProfitReq.AccountID, takeProfitReq, err)
		return err
	}
	return nil
}



// GetConfig: busca a config de lucro completa (takeprofit + stopgain)
func (s *ProfitService) GetConfig(ctx context.Context, accountID string) domain.ProfitCfgSet {
	takeprofit, _ := s.r.GetProfitCfgByType(ctx, accountID, string(domain.ProfitTakeProfit))
	stopgain, _ := s.r.GetProfitCfgByType(ctx, accountID, string(domain.ProfitStopGain))
	return domain.ProfitCfgSet{
		TakeProfit: takeprofit,
		StopGain:   stopgain,
	}
}
