package repo

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"strings"

	"github.com/redis/go-redis/v9"
	"pointersvc/internal/domain"
)

// GetEntryCfgBySide fetches the per-side entry configuration (returns zero value if not found).
func (r *RedisRepo) GetEntryCfgBySide(ctx context.Context, accountID, side string) (domain.EntryUpsertCfg, error) {
	var out domain.EntryUpsertCfg
	side = strings.ToLower(strings.TrimSpace(side))

	b, err := r.rdb.Get(ctx, KeyEntryCfg(accountID, side)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return out, nil // not found
		}
		return out, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (r *RedisRepo) UpsertEntryCfg(ctx context.Context, accountID string, cfg domain.EntryUpsertCfg) error {
	side := strings.ToLower(strings.TrimSpace(string(cfg.Side)))
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return r.rdb.Set(ctx, KeyEntryCfg(accountID, side), b, 0).Err()
}

// RebuildEntryGridSweepV2:
// - calculates occupied grid levels by EXECUTED GS orders (within grid)
// - removes PENDING GS of the given side/account (from zkeys and order:idx:{acct})
// Returns:
// - occ: indices of grid levels occupied by EXECUTED GS (that land inside the grid)
// - removed: IDs of PENDING GS removed
// - execGS: total GS EXECUTED for side/account (regardless of falling inside the grid)
func (r *RedisRepo) RebuildEntryGridSweepV2(
	ctx context.Context,
	accountID string,
	side string,
	levels []float64,
	step float64,
) (map[int]struct{}, []string, int, error) {

	side = strings.ToLower(strings.TrimSpace(side))

	occ := make(map[int]struct{}, len(levels))
	removed := make([]string, 0, 64)
	execGS := 0

	// 1) Count EXECUTED GS and mark occupied levels (uses account order index; may be truncated, which is fine for occupancy)
	if len(levels) > 0 && step > 0 {
		ids, _ := r.ListAccountOrderIDs(ctx, accountID)
		for _, id := range ids {
			ord, err := r.GetOrder(ctx, id)
			if err != nil || ord.ID == "" {
				continue
			}
			if ord.AccountID != accountID || string(ord.Operation.Side) != side {
				continue
			}
			if ord.Operation.Status == domain.StatusExecuted && (ord.Source == "" || ord.Source == domain.SourceGS) {
				execGS++
				price := ord.Operation.PriceUSD
				if price > 0 {
					if i, ok := nearestLevelIndexRepo(levels, step, price); ok {
						occ[i] = struct{}{}
					}
				}
			}
		}
	}

	// 2) Build candidate set using full ZSET scans (captures old triggers) + optional order index complement
	kAcct := KeyEntryTriggerZ(accountID, side)
	kAll := KeyEntryTriggerAll(side)

	gridAcct, _ := r.scanZSetAll(ctx, kAcct)
	gridAll, _ := r.scanZSetAll(ctx, kAll)
	fromIdx, _ := r.ListAccountOrderIDs(ctx, accountID) // can be truncated; used only as complement

	seen := make(map[string]struct{}, len(gridAcct)+len(gridAll)+len(fromIdx))
	candidates := make([]string, 0, len(gridAcct)+len(gridAll)+len(fromIdx))
	push := func(list []string) {
		for _, id := range list {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			candidates = append(candidates, id)
		}
	}
	push(gridAcct)
	push(gridAll)
	push(fromIdx)

	// 3) Remove only GS PENDING for this account/side
	for _, id := range candidates {
		ord, err := r.GetOrder(ctx, id)
		if err != nil || ord.ID == "" {
			continue
		}
		if ord.AccountID != accountID || string(ord.Operation.Side) != side {
			continue
		}
		if (ord.Source == "" || ord.Source == domain.SourceGS) && (ord.Operation.Status == domain.StatusPending) {
			if err := r.RemoveOrder(ctx, id, accountID, side); err == nil {
				removed = append(removed, id)
			}
		}
	}

	log.Printf("[grid:sweep] acct=%s side=%s gridAcct=%d gridAll=%d idx=%d merged=%d execGS=%d removed=%d",
		accountID, side, len(gridAcct), len(gridAll), len(fromIdx), len(candidates), execGS, len(removed))

	return occ, removed, execGS, nil
}

// scanZSetAll iterates an entire ZSET via ZSCAN (streaming; avoids huge ZRANGE windows).
func (r *RedisRepo) scanZSetAll(ctx context.Context, key string) ([]string, error) {
	out := make([]string, 0, 1024)
	var cursor uint64
	for {
		res, next, err := r.rdb.ZScan(ctx, key, cursor, "*", 1000).Result()
		if err != nil {
			return nil, err
		}
		// res: [member1, score1, member2, score2, ...]
		for i := 0; i+1 < len(res); i += 2 {
			out = append(out, res[i])
		}
		cursor = next
		if cursor == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}
	}
	return out, nil
}

// ---- helpers internos ----

// mesma tolerância do seu helper de service
func nearestLevelIndexRepo(levels []float64, step, price float64) (int, bool) {
	if len(levels) == 0 || step <= 0 {
		return -1, false
	}
	idx := int(math.Round((price - levels[0]) / step))
	if idx < 0 || idx >= len(levels) {
		return -1, false
	}
	if math.Abs(levels[idx]-price) <= step/2.0 {
		return idx, true
	}
	return -1, false
}