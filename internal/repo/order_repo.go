package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"pointersvc/internal/domain"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ===== Builder: cria a ordem canônica (sem persistir) =====

func (r *RedisRepo) NewEntryOrder(accountID string, op domain.OrderOp) (domain.Order, error) {
	side := strings.ToLower(string(op.Side))
	if accountID == "" || (side != "buy" && side != "sell") {
		return domain.Order{}, fmt.Errorf("NewEntryOrder: accountID/side inválidos")
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Mantém status pendente no grid
	op.Status = domain.StatusPending

	ord := domain.Order{
		ID:            id,
		AccountID:     accountID,
		Source:        domain.SourceGS, // origem "gs" (grade)
		BrokerOrderID: "",
		Operation:     op,
		Profit:        domain.ProfitSt{}, // zero-value (desabilitado)
		Margin:        domain.MarginSt{}, // zero-value (desabilitado)
		Entry:         domain.EntrySt{},  // zero-value (desabilitado)
		CreatedAt:     now,
	}
	return ord, nil
}

// SaveOrderP: se 'pipe' for nil, cria Pipeline e EXEC; caso contrário,
// apenas enfileira DEL+SET no pipeline fornecido (sem executar aqui).
func (r *RedisRepo) SaveOrder(ctx context.Context, ord domain.Order, pipe redis.Pipeliner) error {
	if ord.ID == "" {
		return fmt.Errorf("SaveOrder: ordem sem ID")
	}
	key := KeyOrd(ord.ID)

	b, err := json.Marshal(ord)
	if err != nil {
		return fmt.Errorf("SaveOrder: marshal: %w", err)
	}

	if pipe == nil {
		p := r.rdb.Pipeline()
		p.Del(ctx, key)       // defensivo contra WRONGTYPE
		p.Set(ctx, key, b, 0) // JSON como STRING

		if _, err := p.Exec(ctx); err != nil {
			return fmt.Errorf("SaveOrder: exec: %w", err)
		}
		return nil
	}

	// Modo batched: quem chamou dá o Exec()
	pipe.Del(ctx, key)
	pipe.Set(ctx, key, b, 0)
	return nil
}

// ===== Indexação do ponteiro de entrada (grid) =====
func (s *RedisRepo) SaveOrderAndIndex(ctx context.Context, ord domain.Order) error {
	pipe := s.Rdb().Pipeline()
	s.SaveOrder(ctx, ord, pipe)
	s.ReindexEntry(ctx, ord, pipe)
	s.IndexOrderForListing(ctx, ord.AccountID, ord.ID, time.Now())
	pipe.SAdd(ctx, KeyOrdIdxBySideAndStatus(ord.AccountID, string(ord.Operation.Side), string(ord.Operation.Status)), ord.ID)

	_, err := pipe.Exec(ctx)
	return err
}

// ===== Leitura da ordem JSON =====

func (r *RedisRepo) GetOrder(ctx context.Context, id string) (domain.Order, error) {
	var ord domain.Order
	key := KeyOrd(id)

	typ, err := r.rdb.Type(ctx, key).Result()
	if err != nil {
		return ord, err
	}
	if typ != "string" {
		return ord, fmt.Errorf("wrongtype key=%s type=%s (esperado STRING)", key, typ)
	}

	b, err := r.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return ord, err
	}
	if err := json.Unmarshal(b, &ord); err != nil {
		return ord, fmt.Errorf("GetOrder: unmarshal: %w", err)
	}
	return ord, nil
}

func (r *RedisRepo) ListAccountOrderIDs(ctx context.Context, accountID string) ([]string, error) {
	return r.rdb.ZRevRange(ctx, KeyOrdIdx(accountID), 0, -1).Result()
}

func (r *RedisRepo) GetOrdersByIDs(ctx context.Context, ids []string) ([]domain.Order, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	pipe := r.rdb.Pipeline()
	cmds := make([]*redis.StringCmd, len(ids))
	for i, id := range ids {
		cmds[i] = pipe.Get(ctx, KeyOrd(id))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}

	out := make([]domain.Order, 0, len(ids))
	for _, cmd := range cmds {
		b, err := cmd.Bytes()
		if err != nil {
			continue
		}
		var o domain.Order
		if json.Unmarshal(b, &o) == nil && o.ID != "" {
			out = append(out, o)
		}
	}
	return out, nil
}

// ---------- per-order overrides (profit/margin) ----------

func (r *RedisRepo) RemoveOrder(ctx context.Context, orderID, side, accountID string) error {
    p := r.rdb.Pipeline()

    p.Del(ctx, KeyOrd(orderID))

    if err := r.UnindexEntry(ctx, accountID, side, orderID, p); err != nil {
        p.Discard()
        return err
    }

    p.SRem(ctx, KeyOrdIdxBySideAndStatus(accountID, side, "pending"), orderID)

    if _, err := p.Exec(ctx); err != nil {
        log.Printf("❌ Erro ao remover order e índices. account=%s order=%s side=%s err=%v", accountID, orderID, side, err)
        return err
    }
    log.Printf("✅ Order e índices removidos. account=%s order=%s side=%s", accountID, orderID, side)
    return nil
}

// UpdateOrderJSON: sobrescreve a ordem como JSON (sem mexer em índices)
func (r *RedisRepo) UpdateOrder(ctx context.Context, ord domain.Order) error {
	if ord.ID == "" {
		return fmt.Errorf("UpdateOrderJSON: id vazio")
	}
	b, err := json.Marshal(ord)
	if err != nil {
		return fmt.Errorf("UpdateOrderJSON: marshal: %w", err)
	}
	pipe := r.rdb.Pipeline()
	pipe.Del(ctx, KeyOrd(ord.ID))
	pipe.Set(ctx, KeyOrd(ord.ID), b, 0)
	_, err = pipe.Exec(ctx)
	return err
}

func (r *RedisRepo) ListAccountOrderIDsBySideAndStatus(
	ctx context.Context,
	accountID, side, status string,
) ([]string, error) {
	key := KeyOrdIdxBySideAndStatus(accountID, side, status)
	ids, err := r.rdb.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return ids, nil
}
