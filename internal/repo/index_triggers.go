// internal/repo/index_triggers.go
package repo

import (
	"context"
	"log"
	"math"
	"time"

	"pointersvc/internal/domain"

	"github.com/redis/go-redis/v9"
)

/* ========== ENTRY (grid) ========== */

// Remove só dos ZSETs de ENTRY (grid)
func (r *RedisRepo) UnindexEntry(ctx context.Context, accountID, side, orderID string, pipe redis.Pipeliner) error {
	var p redis.Pipeliner
	owned := false

	if pipe == nil {
		p = r.rdb.Pipeline() // NÃO usar :=
		owned = true
	} else {
		p = pipe
	}

	// Remoções idempotentes
	p.ZRem(ctx, KeyEntryTriggerZ(accountID, side), orderID)
	p.ZRem(ctx, KeyEntryTriggerAll(side), orderID)

	if owned {
		if _, err := p.Exec(ctx); err != nil {
			log.Printf("❌ Erro ao remover trigger de entrada. account=%s order=%s side=%s err=%v", accountID, orderID, side, err)
			return err
		}
		log.Printf("✅ Trigger de entrada removido. account=%s order=%s side=%s", accountID, orderID, side)
	}
	return nil
}

// Adiciona/atualiza só os ZSETs de ENTRY (grid).
// Usa score = price arredondado (0 casas)
func (r *RedisRepo) ReindexEntry(ctx context.Context, ord domain.Order, pipe redis.Pipeliner) error {
	side := string(ord.Operation.Side)
	price := ord.Operation.PriceUSD
	score := math.Round(price)
	if pipe == nil {
		pipe := r.rdb.Pipeline()
		pipe.ZAdd(ctx, KeyEntryTriggerZ(ord.AccountID, side), redis.Z{Score: score, Member: ord.ID})
		pipe.ZAdd(ctx, KeyEntryTriggerAll(side), redis.Z{Score: score, Member: ord.ID})
		if _, err := pipe.Exec(ctx); err != nil {
			log.Printf("❌ Erro ao incluir o trigger de entrada. account=%s order=%s side=%s price=%s", ord.AccountID, ord.ID, side, ord.Operation.PriceUSD)
			return err
		}
		log.Printf("➡️ Trigger de entrada incluido com sucesso. account=%s order=%s side=%s price=%s", ord.AccountID, ord.ID, side, ord.Operation.PriceUSD)
		return nil
	}
	pipe.ZAdd(ctx, KeyEntryTriggerZ(ord.AccountID, side), redis.Z{Score: score, Member: ord.ID})
	pipe.ZAdd(ctx, KeyEntryTriggerAll(side), redis.Z{Score: score, Member: ord.ID})
	log.Printf("➡️ Trigger de entrada incluido. account=%s order=%s price=%s", ord.AccountID, ord.ID, ord.Operation.PriceUSD)
	return nil
}

/* ========== STOPGAIN ========== */

// Remove só dos ZSETs de STOPGAIN
// Observação: mantém suporte a “all por side” se você usa KeyStopGainAllSide(side).
func (r *RedisRepo) UnindexStopGain(ctx context.Context, accountID, side, orderID string, pipe redis.Pipeliner) error {
	if pipe == nil {
		pipe := r.rdb.Pipeline()
		pipe.ZRem(ctx, KeyStopGainZ(accountID), orderID)
		pipe.ZRem(ctx, KeyStopGainAll(), orderID)
		if side != "" {
			pipe.ZRem(ctx, KeyStopGainAllSide(side), orderID)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			log.Printf("❌ Erro ao remover o trigger de stop gain. account=%s order=%s", accountID, orderID)
			return err
		}
		log.Printf("💸 Trigger do stop gain removido com sucesso. account=%s order=%s", accountID, orderID)
		return nil
	}
	pipe.ZRem(ctx, KeyStopGainZ(accountID), orderID)
	pipe.ZRem(ctx, KeyStopGainAll(), orderID)
	log.Printf("💸 Trigger do stop gain removido com sucesso (batched). account=%s order=%s", accountID, orderID)

	return nil
}

// Adiciona/atualiza só os ZSETs de STOPGAIN.
// 'triggerPrice' é score (arredondado).
func (r *RedisRepo) ReindexStopGain(ctx context.Context, accountID, orderID string, triggerPrice float64, side string, pipe redis.Pipeliner) error {
	score := math.Round(triggerPrice)
	if pipe == nil {
		pipe := r.rdb.Pipeline()
		pipe.ZAdd(ctx, KeyStopGainZ(accountID), redis.Z{Score: score, Member: orderID})
		pipe.ZAdd(ctx, KeyStopGainAll(), redis.Z{Score: score, Member: orderID})
		if side != "" {
			pipe.ZAdd(ctx, KeyStopGainAllSide(side), redis.Z{Score: score, Member: orderID})
		}
		if _, err := pipe.Exec(ctx); err != nil {
			log.Printf("❌ Erro ao incluir o trigger de stop gain. account=%s order=%s", accountID, orderID)
			return err
		}
		log.Printf("💸 Trigger do stop gain incluido com sucesso. account=%s order=%s", accountID, orderID)
		return nil
	}
	pipe.ZAdd(ctx, KeyStopGainZ(accountID), redis.Z{Score: score, Member: orderID})
	pipe.ZAdd(ctx, KeyStopGainAll(), redis.Z{Score: score, Member: orderID})
	if side != "" {
		pipe.ZAdd(ctx, KeyStopGainAllSide(side), redis.Z{Score: score, Member: orderID})
	}
	log.Printf("💸 Trigger do stop gain incluido (batched). account=%s order=%s", accountID, orderID)
	return nil
}

/* ========== RISK (margem) ========== */
// Remove só dos ZSETs de RISK (risk:{account}, risk:all).
// Se pipe == nil, cria e executa a transação internamente.
func (r *RedisRepo) UnindexRisk(ctx context.Context, accountID, orderID string, pipe redis.Pipeliner) error {
	if pipe == nil {
		p := r.rdb.Pipeline()
		p.ZRem(ctx, KeyRiskZ(accountID), orderID)
		p.ZRem(ctx, KeyRiskAll(), orderID)
		if _, err := p.Exec(ctx); err != nil {
			log.Printf("❌ Erro ao remover o trigger de protecao de margem. account=%s order=%s", accountID, orderID)
			return err

		}
		log.Printf("⛑️ Trigger de protecao de margem removido com sucesso. account=%s order=%s", accountID, orderID)
		return nil
	}
	pipe.ZRem(ctx, KeyRiskZ(accountID), orderID)
	pipe.ZRem(ctx, KeyRiskAll(), orderID)
	log.Printf("⛑️ Trigger de protecao de margem removido (batched). account=%s order=%s", accountID, orderID)
	return nil
}

// Adiciona/atualiza só os ZSETs de RISK. Score = trigger (arredondado).
// Se pipe == nil, cria e executa a transação internamente.
func (r *RedisRepo) ReindexRisk(ctx context.Context, accountID, orderID string, trigger float64, pipe redis.Pipeliner) error {
	score := trigger
	if pipe == nil {
		p := r.rdb.Pipeline()
		p.ZAdd(ctx, KeyRiskZ(accountID), redis.Z{Score: score, Member: orderID})
		p.ZAdd(ctx, KeyRiskAll(), redis.Z{Score: score, Member: orderID})
		if _, err := p.Exec(ctx); err != nil {
			log.Printf("❌ Erro ao incluir o trigger de protecao de margem. account=%s order=%s trigger=%.0f", accountID, orderID, score)
			return err
		}
		log.Printf("⛑️ Index de protecao de margem incluído com sucesso. account=%s order=%s trigger=%.0f", accountID, orderID, score)
		return nil
	}
	pipe.ZAdd(ctx, KeyRiskZ(accountID), redis.Z{Score: score, Member: orderID})
	pipe.ZAdd(ctx, KeyRiskAll(), redis.Z{Score: score, Member: orderID})
	log.Printf("⛑️ Index de protecao de margem incluído (batched). account=%s order=%s trigger=%.0f", accountID, orderID, score)
	return nil
}

/* ========== (opcionais) índice de listagem ========== */

// Índice de listagem por conta (não é “gatilho”, deixei separado)
func (r *RedisRepo) IndexOrderForListing(ctx context.Context, accountID, orderID string, when time.Time) error {
	ts := when
	if ts.IsZero() {
		ts = time.Now()
	}
	score := float64(ts.UnixMilli())
	if err := r.rdb.ZAdd(ctx, KeyOrdIdx(accountID), redis.Z{Score: score, Member: orderID}).Err(); err != nil {
		log.Printf("❌ Erro ao incluir o a ordem no index de conta. account=%s order=%s trigger=%.0f", accountID, orderID, score)
		return err
	}
	return nil
}

func (r *RedisRepo) UnindexOrderFromListing(ctx context.Context, accountID, orderID string) error {
	return r.rdb.ZRem(ctx, KeyOrdIdx(accountID), orderID).Err()
}
