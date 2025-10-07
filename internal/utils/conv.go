// internal/utils/conv.go
package utils

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func BoolTo01(b bool) int { if b { return 1 }; return 0 }
func Str01(s string) bool { return s == "1" || s == "true" || s == "TRUE" || s == "True" }
func ParseF(s string) float64 { if s == "" { return 0 }; v, _ := strconv.ParseFloat(s, 64); return v }
func NowRFC3339Nano() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// GridFingerprint produces a stable hash of grid-shaping fields.
// Includes `amount` so changing 500 -> 200 forces a new fingerprint.
func GridFingerprint(
	side string,
	enabled bool,
	initUSD float64,
	intervalUSD float64,
	amountUSD float64, // <- new
	qty int,
) string {
	// Normalize to avoid tiny float drift (use 0 decimals if your grid is integer-based)
	init := fmt.Sprintf("%.0f", math.Round(initUSD))
	step := fmt.Sprintf("%.0f", math.Round(intervalUSD))
	amt  := fmt.Sprintf("%.0f", math.Round(amountUSD))

	// Add a version tag so V2 hashes never collide with V1
	data := fmt.Sprintf("gfp:v2|%s|%s|%s|%s|%d|%t",
		strings.ToLower(side), init, step, amt, qty, enabled)

	sum := sha1.Sum([]byte(data))
	return hex.EncodeToString(sum[:])
}

// Sign returns +1 for "buy" and -1 for "sell"
func Sign(side string) float64 {
	if side == "buy" { return 1 }
	return -1
}

func CoalesceFlag(raw map[string]string, key string, def bool) bool {
    v, ok := raw[key]
    if !ok || v == "" { return def }
    return v == "1" || v == "true" || v == "TRUE" || v == "True"
}