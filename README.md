# go-pointer-svc (control plane)

HTTP API to manage **entry automation**, **profit management (takeprofit/stopgain)**, and **margin rules** via Redis pointers.
This control-plane also provides **account-level feature flags** (master + per-resource enable).

## Feature flags (uniform enable/disable)

Redis HASH: `features:{account}`
- `all_enabled` → 0|1 (master switch)
- `entry_enabled` → 0|1
- `profit_enabled` → 0|1
- `margin_enabled` → 0|1

**Effective enable** for a resource = `all_enabled && {resource}_enabled && {resource_config.enabled}`

### Endpoints
- `PUT /accounts/{account_id}/features`
- `PATCH /accounts/{account_id}/features/toggle`
- `GET /accounts/{account_id}/features`

## Quick start

```bash
export REDIS_ADDR=localhost:6379
go mod tidy
go run ./cmd/api
```

Or with Docker:

```bash
docker compose up --build
```

## Endpoints

### POST /entry-automation
Body:
```json
{
  "account_id": "0586fa40-3540-4946-aa66-b21a16ea3a65",
  "is_active": true,
  "operation": {
    "type": "buy",
    "initial_price_usd": 113420,
    "order_value_usd": 1,
    "order_margin_sats": 9500,
    "contracts_quantity": 3,
    "leverage": 10,
    "price_interval_usd": 37
  }
}
```

### POST /profit
**takeprofit**
```json
{ "account_id": "0586fa40-3540-4946-aa66-b21a16ea3a65", "profit_pct": 5.0, "type": "takeprofit" }
```
**stopgain**
```json
{ "account_id": "0586fa40-3540-4946-aa66-b21a16ea3a65", "profit_pct": 5.0, "type": "stopgain", "interval_usd": 200 }
```

### POST /margin-rule
```json
{
  "account_id": "0586fa40-3540-4946-aa66-b21a16ea3a65",
  "enabled": true,
  "threshold_val_pct": 5,
  "extra_margin_pct": 10
}
```


## Entry grid versioning (grid_epoch)

- `cfg:entry:{account}.grid_epoch` is incremented automatically when structural fields change:
  `side`, `initial_price_usd`, `price_interval_usd`, `contracts_quantity`.
- New pointers are written to `entry:{account}:{side}:e{epoch}` and each `order:{id}` stores `epoch`.
- Worker should read `grid_epoch` (or cache) and consume from the epoched ZSET only.


### PUT /orders/{order_id}/opened
Idempotent upsert hook called by worker after opening an order to persist runtime state and create protection pointers (liq_guard; stopgain/takeprofit per config).

Example:
```json
{
  "account_id": "0586fa40-3540-4946-aa66-b21a16ea3a65",
  "position_uid": "pos-0586...",
  "broker_order_id": "LN-123",
  "side": "buy",
  "entry_price_usd": 113420.0,
  "liq_price_usd": 100000.0,
  "qty": 3,
  "leverage": 10,
  "initial_margin_sats": 9500
}
```


### 

**EffectiveEnabled = all_enabled && feature_flag && cfg.enabled** (plus per-order overrides).


### PUT /orders/{order_id}/profit
Upsert per-order profit config. Body:
```json
{ "enabled": true, "type": "takeprofit", "profit_pct": 4.2 }
```

### PUT /orders/{order_id}/margin
Upsert per-order margin protection. Body:
```json
{ "enabled": true, "threshold_val_pct": 6, "extra_margin_pct": 12 }
```


### PUT /accounts/{id}/profit
Upsert da estratégia de lucro a nível de conta (uma única **feature** com modos `takeprofit` ou `stopgain`).

Body (ex. stopgain):
```json
{ "enabled": true, "type": "stopgain", "profit_pct": 5.0, "interval_usd": 200 }
```

Body (ex. takeprofit):
```json
{ "enabled": true, "type": "takeprofit", "profit_pct": 5.0 }
```


### PUT /accounts/{id}/margin
Upsert da proteção de margem a nível de conta (inclui `enabled`).

Body:
```json
{ "enabled": true, "threshold_val_pct": 5, "extra_margin_pct": 10 }
```
