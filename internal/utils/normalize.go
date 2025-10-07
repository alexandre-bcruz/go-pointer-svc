package utils

// import (
// 	"pointersvc/internal/domain"
// 	"strings"
// )

// func NormalizeSource(s string) string {
//     s = strings.TrimSpace(strings.ToLower(s))
//     switch s {
//     case domain.SourceGS, "golden_snitch":
//         return domain.SourceGS
//     case domain.SourceLN, "ln_markets":
//         return domain.SourceLN
//     default:
//         return "" // desconhecido → trate como erro ou default
//     }
// }
// func NormalizeEnabled(m map[string]string) map[string]any {
// 	out := make(map[string]any, len(m))
// 	for k, v := range m {
// 		if k == "enabled" {
// 			out[k] = Str01(v)
// 		} else {
// 			out[k] = v
// 		}
// 	}
// 	return out
// }
