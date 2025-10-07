package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/redis/go-redis/v9"

	"pointersvc/internal/domain"
)

/* ===================== Account Profit Config (JSON) ===================== */

func (r *RedisRepo) SetProfitCfg(ctx context.Context, accountID string, cfg domain.ProfitCfgSet) error {
	if accountID == "" {
		return fmt.Errorf("SetProfitCfg: accountID vazio")
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("SetProfitCfg: marshal: %w", err)
	}
	key := KeyProfitCfg(accountID)

	pipe := r.rdb.Pipeline()
	pipe.Del(ctx, key)
	pipe.Set(ctx, key, b, 0)
	_, err = pipe.Exec(ctx)
	return err
}

func (r *RedisRepo) GetProfitCfg(ctx context.Context, accountID string) (domain.ProfitCfgSet, error) {
	var out domain.ProfitCfgSet
	if accountID == "" {
		return out, fmt.Errorf("GetProfitCfg: accountID vazio")
	}
	key := KeyProfitCfg(accountID)

	typ, err := r.rdb.Type(ctx, key).Result()
	if err != nil {
		return out, err
	}
	if typ == "none" {
		return out, redis.Nil
	}
	if typ != "string" {
		return out, fmt.Errorf("GetProfitCfg: WRONGTYPE key=%s type=%s (esperado STRING)", key, typ)
	}

	b, err := r.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("GetProfitCfg: unmarshal: %w", err)
	}
	return out, nil
}

func (r *RedisRepo) SetProfitCfgByType(ctx context.Context, accountID string, typ string, cfg domain.ProfitCfg) error {
	if accountID == "" || (typ != string(domain.ProfitTakeProfit) && typ != string(domain.ProfitStopGain)) {
		return fmt.Errorf("SetProfitCfgByType: parâmetros inválidos")
	}

	existing, err := r.GetProfitCfg(ctx, accountID)
	if err != nil && err != redis.Nil {
		return err
	}

	switch typ {
	case string(domain.ProfitTakeProfit):
		existing.TakeProfit = cfg
	case string(domain.ProfitStopGain):
		existing.StopGain = cfg
	default:
		return fmt.Errorf("estratégia inválida: %s", typ)
	}

	return r.SetProfitCfg(ctx, accountID, existing)
}

func (r *RedisRepo) GetProfitCfgByType(ctx context.Context, accountID string, strategy string) (domain.ProfitCfg, error) {
	cfg, err := r.GetProfitCfg(ctx, accountID)
	if err != nil {
		return domain.ProfitCfg{}, err
	}
	switch strategy {
	case string(domain.ProfitTakeProfit):
		return cfg.TakeProfit, nil
	case string(domain.ProfitStopGain):
		return cfg.StopGain, nil
	default:
		return domain.ProfitCfg{}, fmt.Errorf("estratégia inválida: %s", strategy)
	}
}

func (r *RedisRepo) SetProfitEnabled(ctx context.Context, accountID string, enabled bool, strategy string) error {
	cfg, err := r.GetProfitCfg(ctx, accountID)
	if err != nil {
		return err
	}
	switch strategy {
	case string(domain.ProfitTakeProfit):
		cfg.TakeProfit.Enabled = enabled
	case string(domain.ProfitStopGain):
		cfg.StopGain.Enabled = enabled
	default:
		return fmt.Errorf("SetProfitEnabled: estratégia inválida: %s", strategy)
	}
	return r.SetProfitCfg(ctx, accountID, cfg)
}
func (r *RedisRepo) GetOrderConfigs(ctx context.Context, orderID string) (domain.ProfitCfgSet, domain.MarginCfg) {
	profitCfg, _ := r.GetOrderProfitCfg(ctx, orderID)
	marginCfg, _ := r.GetOrderMarginCfg(ctx, orderID)
	return profitCfg, marginCfg
}

// Agora retorna ProfitCfgSet, e não mais ProfitCfg único
func (r *RedisRepo) GetOrderProfitCfg(ctx context.Context, orderID string) (domain.ProfitCfgSet, error) {
	key := KeyProfitOrder(orderID)
	typ, err := r.rdb.Type(ctx, key).Result()
	if err != nil {
		return domain.ProfitCfgSet{}, err
	}
	if typ != "string" {
		return domain.ProfitCfgSet{}, fmt.Errorf("wrongtype key=%s type=%s (esperado string/json)", key, typ)
	}
	b, err := r.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return domain.ProfitCfgSet{}, err
	}
	var pCfgSet domain.ProfitCfgSet
	if err := json.Unmarshal(b, &pCfgSet); err != nil {
		return domain.ProfitCfgSet{}, fmt.Errorf("json unmarshal: %w", err)
	}
	return pCfgSet, nil
}

func (r *RedisRepo) SetOrderProfitCfg(ctx context.Context, orderID string, cfg domain.ProfitCfgSet) (domain.ProfitCfgSet, error) {
	if orderID == "" {
		return domain.ProfitCfgSet{}, fmt.Errorf("orderID vazio")
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return domain.ProfitCfgSet{}, fmt.Errorf("marshal ProfitCfgSet: %w", err)
	}
	if err := r.rdb.Set(ctx, KeyProfitOrder(orderID), b, 0).Err(); err != nil {
		return domain.ProfitCfgSet{}, fmt.Errorf("redis SET profit cfg: %w", err)
	}
	return cfg, nil
}

/* ===================== StopGain Pointer (Triggers) ===================== */

func (r *RedisRepo) SaveStopGainPointer(ctx context.Context, ord domain.Order, triggerPrice float64) error {
	if ord.ID == "" || ord.AccountID == "" {
		return fmt.Errorf("SaveStopGainPointer: ordem inválida (id/account)")
	}
	side := strings.ToLower(string(ord.Operation.Side))
	if side != "buy" && side != "sell" {
		return fmt.Errorf("SaveStopGainPointer: side inválido: %s", side)
	}
	score := math.Round(triggerPrice)

	b, err := json.Marshal(ord)
	if err != nil {
		return fmt.Errorf("SaveStopGainPointer: marshal ordem: %w", err)
	}

	accZ := KeyStopGainZ(ord.AccountID)
	allZ := KeyStopGainAll()
	sideZ := KeyStopGainAllSide(side)

	pipe := r.rdb.Pipeline()
	pipe.Del(ctx, KeyOrd(ord.ID))
	pipe.Set(ctx, KeyOrd(ord.ID), b, 0)
	pipe.ZAdd(ctx, accZ, redis.Z{Score: score, Member: ord.ID})
	pipe.ZAdd(ctx, allZ, redis.Z{Score: score, Member: ord.ID})
	pipe.ZAdd(ctx, sideZ, redis.Z{Score: score, Member: ord.ID})

	_, err = pipe.Exec(ctx)
	return err
}

func (r *RedisRepo) UpsertOrderStopgainTrigger(ctx context.Context, ord domain.Order, trigger float64) error {
	p := r.rdb.Pipeline()
	if ord.Profit.Enabled {
		if err := r.ReindexStopGain(ctx, ord.AccountID, ord.ID, ord.Profit.TargetUSD, string(ord.Operation.Side), p); err != nil {
			p.Discard()
			return err
		}
	} else {
		if err := r.UnindexStopGain(ctx, ord.AccountID, string(ord.Operation.Side), ord.ID, p); err != nil {
			p.Discard()
			return err
		}
	}
	r.SaveOrder(ctx, ord, p)

	if _, err := p.Exec(ctx); err != nil {
		return fmt.Errorf("UpsertOrderMarginTrigger: exec: %w", err)
	}
	return nil
}
