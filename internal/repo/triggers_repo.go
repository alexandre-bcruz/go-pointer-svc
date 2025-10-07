package repo

import (
	"context"
	"fmt"
	"pointersvc/internal/domain"

	"github.com/redis/go-redis/v9"
)

// ---------- Listagens ----------
func (r *RedisRepo) ListEntryTriggerIDsMerged(ctx context.Context, side, accountID string) ([]string, error) {
    k1 := KeyEntryTriggerAll(side)
    k2 := KeyEntryTriggerZ(accountID, side)

    // cheapest: client side merge
    pipe := r.rdb.Pipeline()
    r1 := pipe.ZRange(ctx, k1, 0, -1)
    r2 := pipe.ZRange(ctx, k2, 0, -1)
    if _, err := pipe.Exec(ctx); err != nil { return nil, err }

    seen := make(map[string]struct{}, len(r1.Val())+len(r2.Val()))
    out := make([]string, 0, len(seen))
    for _, id := range append(r1.Val(), r2.Val()...) {
        if _, ok := seen[id]; !ok { seen[id] = struct{}{}; out = append(out, id) }
    }
    return out, nil
}

func (r *RedisRepo) ListMarginTriggerIDs(ctx context.Context, accountID *string) ([]string, error) {
	if accountID != nil {

		return r.rdb.ZRange(ctx, KeyRiskZ(*accountID), 0, -1).Result()
	}
	return r.rdb.ZRange(ctx, KeyRiskAll(), 0, -1).Result()
}

func (r *RedisRepo) ListStopGainTriggerIDs(ctx context.Context, accountID *string) ([]string, error) {
	if accountID != nil {

		return r.rdb.ZRange(ctx, KeyStopGainZ(*accountID), 0, -1).Result()
	}
	return r.rdb.ZRange(ctx, KeyStopGainAll(), 0, -1).Result()
}

func (r *RedisRepo) ListPointersFiltered(ctx context.Context, key, minStr, maxStr string, limit int64, desc bool) ([]domain.TriggerItem, error) {

	var (
		zs  []redis.Z
		err error
	)
	if desc {
		zs, err = r.Rdb().ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{Min: minStr, Max: maxStr, Offset: 0, Count: limit}).Result()
	} else {
		zs, err = r.Rdb().ZRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{Min: minStr, Max: maxStr, Offset: 0, Count: limit}).Result()
	}
	if err != nil {
		return nil, err
	}

	items := make([]domain.TriggerItem, 0, len(zs))

	for _, z := range zs {
		items = append(items, domain.TriggerItem{
			ID:    fmt.Sprint(z.Member),
			Score: z.Score,
		})
	}

	return items, nil
}
