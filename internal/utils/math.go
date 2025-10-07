// internal/utils/finance.go
package utils

import (
	"math"
	"strings"
	"time"
)

// ======== Configs padrão de taxa ========
// Percentuais em pontos percentuais (%). Ex.: 0.1 = 0.1%
const (
	DefaultEntradaPct = 0.1  // 0.1% na entrada
	DefaultSaidaPct   = 0.1  // 0.1% na saída
	DefaultDailyPct   = 0.03 // 0.03% por dia
)

func Round(v float64, places int) float64 {
	if places == 0 {
		return math.Round(v)
	}
	f := math.Pow(10, float64(places))
	return math.Round(v*f) / f
}

// normalize side
func isSell(side string) bool { return strings.ToLower(strings.TrimSpace(side)) == "sell" }
func isBuy(side string) bool  { return strings.ToLower(strings.TrimSpace(side)) == "buy" }

// CalcTakeProfitValue calcula o alvo de take profit aplicando:
// - lucro desejado (profitPct)
// - taxa de entrada (DefaultEntradaPct)
// - taxa de saída (DefaultSaidaPct)
// - taxa diária acumulada (DefaultDailyPct * diasPassados)
// BUY  => base * (1 + pctTotal)
// SELL => base * (1 - pctTotal)
// Retorna arredondado para 0 casas.
func CalcTakeProfitValue(baseValue float64, profitPct float64, diasPassados int, side string) float64 {
	entradaPct := DefaultEntradaPct
	saidaPct := DefaultSaidaPct
	taxaDiariaPct := DefaultDailyPct * float64(diasPassados)

	pctTotal := profitPct + entradaPct + saidaPct + taxaDiariaPct

	var target float64
	if isSell(side) {
		target = baseValue * (1 - pctTotal/100.0)
	} else {
		target = baseValue * (1 + pctTotal/100.0)
	}
	return Round(target, 0)
}

// CalcStopGainTargets (para estratégia stopgain):
// - base = CalcTakeProfitValue(entryPrice, profitPct, dias, side)
// - stopLoss = base
// - target   = base ± intervalUSD  (BUY: +interval, SELL: -interval)
// Ambos retornados arredondados para 0 casas.
func CalcStopGainTargets(entryPrice, profitPct, intervalUSD float64, side string, diasPassados int) (targetUSD, stopLossUSD float64) {
	base := CalcTakeProfitValue(entryPrice, profitPct, diasPassados, side)
	stopLossUSD = base

	if isSell(side) {
		targetUSD = Round(base-intervalUSD, 0)
	} else {
		targetUSD = Round(base+intervalUSD, 0)
	}
	return targetUSD, stopLossUSD
}

// ======== Helpers de datas ========

func DaysSinceCreated(createdAt string, now time.Time) int {
	t, ok := parseRFC3339Flex(createdAt)
	if !ok {
		return 0
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	days := int(now.Sub(t).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

func DaysSinceNow(createdAt string) int {
	return DaysSinceCreated(createdAt, time.Now().UTC())
}

func parseRFC3339Flex(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

// calcMarginTriggerDirectional computes the score used in Redis (no decimals),
// matching your streaming queries:
//  - BUY  => trigger when price <= score  → score = floor(liq * (1 + thr%))
//  - SELL => trigger when price >= score  → score = ceil (liq * (1 - thr%))
func CalcMarginTriggerDirectional(liq float64, side string, thrPct float64) float64 {
	if liq <= 0 || thrPct <= 0 || math.IsNaN(liq) || math.IsInf(liq, 0) {
		return 0
	}
	a := thrPct / 100.0
	if a >= 0.999 { // evita div/0 e absurdos
		a = 0.999
	}
	switch side {
	case "buy":
		// price <= T, dist = (price - liq)/price = a => T = liq/(1-a)
		return math.Ceil(liq / (1.0 - a))
	case "sell":
		// price >= T, dist = (liq - price)/price = a => T = liq/(1+a)
		return math.Floor(liq / (1.0 + a))
	default:
		return 0
	}
}


func AlmostEqual(a, b float64) bool {
	const eps = 1e-9
	return math.Abs(a-b) <= eps
}