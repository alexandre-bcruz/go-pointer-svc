// internal/repo/margin.go
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"pointersvc/internal/domain"
)

// Key geradora já existe no seu pacote de keys:
// func KeyMarginRule(acct string) string

/* ===================== Margin Config (JSON) ===================== */

// SetMarginCfg: persiste a configuração de margem da conta como JSON (STRING).
func (r *RedisRepo) SetMarginCfg(ctx context.Context, accountID string, cfg domain.MarginCfg) error {
	if accountID == "" {
		return fmt.Errorf("SetMarginCfg: accountID vazio")
	}
	key := KeyMarginRule(accountID)

	b, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("SetMarginCfg: marshal: %w", err)
	}

	pipe := r.rdb.Pipeline()
	pipe.Del(ctx, key)       // defensivo contra WRONGTYPE
	pipe.Set(ctx, key, b, 0) // grava JSON como string
	_, err = pipe.Exec(ctx)
	return err
}

// GetMarginCfg: lê a configuração de margem da conta (JSON → struct).
// Retorna redis.Nil se a key não existir.
func (r *RedisRepo) GetMarginCfg(ctx context.Context, accountID string) (domain.MarginCfg, error) {
	var out domain.MarginCfg
	if accountID == "" {
		return out, fmt.Errorf("GetMarginCfg: accountID vazio")
	}

	key := KeyMarginRule(accountID)

	typ, err := r.rdb.Type(ctx, key).Result()
	if err != nil {
		return out, err
	}
	if typ == "none" {
		return out, redis.Nil
	}
	if typ != "string" {
		return out, fmt.Errorf("GetMarginCfg: WRONGTYPE key=%s type=%s (esperado STRING)", key, typ)
	}

	b, err := r.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("GetMarginCfg: unmarshal: %w", err)
	}
	return out, nil
}

// UpsertOrderMarginTrigger atualiza os índices de risco (risk:{account}, risk:all)
// e persiste o margin_trigger dentro do JSON da ordem (ptr:{id}) sem ler do Redis.
// Tudo numa única transação para manter consistência.
func (r *RedisRepo) UpsertOrderMarginTrigger(ctx context.Context, ord domain.Order, trigger float64) error {
	// Uma única transação: (des)indexar risk + SET do JSON
	p := r.rdb.Pipeline()
	if ord.Margin.Enabled {
		if err := r.ReindexRisk(ctx, ord.AccountID, ord.ID, ord.Margin.MarginTrigger, p); err != nil {
			p.Discard()
			return err
		}
	} else {
		if err := r.UnindexRisk(ctx, ord.AccountID, ord.ID, p); err != nil {
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

// SetMarginEnabled: altera apenas a flag Enabled via RMW na cfg JSON.
func (r *RedisRepo) SetMarginEnabled(ctx context.Context, accountID string, enabled bool) error {
	cfg, err := r.GetMarginCfg(ctx, accountID)
	if err != nil {
		return err
	}
	cfg.Enabled = enabled
	return r.SetMarginCfg(ctx, accountID, cfg)
}

// SetOrderMarginCfg grava a config de margem por ORDEM como JSON
// e retorna o próprio objeto salvo.
func (r *RedisRepo) SetOrderMarginCfg(ctx context.Context, orderID string, cfg domain.MarginCfg) (domain.MarginCfg, error) {
	if orderID == "" {
		return domain.MarginCfg{}, fmt.Errorf("orderID vazio")
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return domain.MarginCfg{}, fmt.Errorf("marshal MarginCfg: %w", err)
	}
	if err := r.rdb.Set(ctx, KeyMarginOrder(orderID), b, 0).Err(); err != nil {
		return domain.MarginCfg{}, fmt.Errorf("redis SET margin cfg: %w", err)
	}
	return cfg, nil
}

// GetOrderMarginCfg lê a config de margem por ORDEM como JSON
// e retorna o objeto tipado. Erra explicitamente se não existir.
func (r *RedisRepo) GetOrderMarginCfg(ctx context.Context, orderID string) (domain.MarginCfg, error) {
	var out domain.MarginCfg
	if orderID == "" {
		return out, fmt.Errorf("orderID vazio")
	}
	b, err := r.rdb.Get(ctx, KeyMarginOrder(orderID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return out, fmt.Errorf("margin cfg não encontrada para a ordem %s", orderID)
		}
		return out, fmt.Errorf("redis GET margin cfg: %w", err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("unmarshal MarginCfg: %w", err)
	}
	return out, nil
}
