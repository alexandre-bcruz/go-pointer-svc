// internal/keys/keys.go
package repo

import "fmt"

const (
	// Ponteiro único por ordem (grid/pending)
	keyFmtPtr    = "ptr:%s"
	keyFmtPtrIdx = "ptr:idx:%s"

	// Features / configs (nível de conta)
	keyFmtFeatures   = "features:%s"
	keyFmtEntryCfg   = "cfg:entry:%s:%s"
	keyFmtProfitCfg  = "cfg:profit:%s"
	keyFmtMarginRule = "margin:rules:%s"

	// StopGain / Risk
	keyFmtStopGainZ       = "stopgain:%s"     // por conta
	keyStopGainAll        = "stopgain:all"    // global
	keyFmtStopGainAllSide = "stopgain:all:%s" // global por side (buy/sell)

	keyFmtRiskZ = "risk:%s"
	keyRiskAll  = "risk:all"

	// Entry triggers
	keyFmtEntryTriggerZ   = "trigger:entry:%s:%s" // por conta/side
	keyFmtEntryTriggerAll = "trigger:entry:all:%s" // buy|sell
)

/* ===== Ponteiros / Ordens ===== */

func KeyOrd(id string) string      { return fmt.Sprintf(keyFmtPtr, id) }
func KeyOrdIdx(acct string) string { return fmt.Sprintf(keyFmtPtrIdx, acct) }

/* ===== Features / Configs ===== */

func KeyFeatures(acct string) string   { return fmt.Sprintf(keyFmtFeatures, acct) }
func KeyEntryCfg(acct, side string) string   { return fmt.Sprintf(keyFmtEntryCfg, acct, side) }
func KeyProfitCfg(acct string) string  { return fmt.Sprintf(keyFmtProfitCfg, acct) }
func KeyMarginRule(acct string) string { return fmt.Sprintf(keyFmtMarginRule, acct) }

func KeyProfitOrder(orderID string) string { return "cfg:profit:order:" + orderID }
func KeyMarginOrder(orderID string) string { return "cfg:margin:order:" + orderID }

/* ===== Entry triggers ===== */

func KeyEntryTriggerZ(acct, side string) string { return fmt.Sprintf(keyFmtEntryTriggerZ, acct, side) }
func KeyEntryTriggerAll(side string) string     { return fmt.Sprintf(keyFmtEntryTriggerAll, side) }

// Helpers de conveniência (evitam “consts soltas” em outros pacotes)
func KeyZEntryAllBuy() string  { return KeyEntryTriggerAll("buy") }
func KeyZEntryAllSell() string { return KeyEntryTriggerAll("sell") }

/* ===== StopGain / Risk ===== */

func KeyStopGainZ(acct string) string       { return fmt.Sprintf(keyFmtStopGainZ, acct) }
func KeyStopGainAll() string                { return keyStopGainAll }
func KeyStopGainAllSide(side string) string { return fmt.Sprintf(keyFmtStopGainAllSide, side) }

func KeyRiskZ(acct string) string { return fmt.Sprintf(keyFmtRiskZ, acct) }
func KeyRiskAll() string          { return keyRiskAll }

func KeyOrdIdxBySideAndStatus(accountID, side, status string) string {
	return fmt.Sprintf("ptr:idx:%s:%s:%s", accountID, side, status)
}