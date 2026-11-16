package main

import (
	"encoding/json"
	"sync"
	"unicode"
)

// diagRecord is the only diagnostic payload this plugin emits.
// It carries a stable rule id, the protocol scope, and a change count.
type diagRecord struct {
	RuleID string
	Scope  string
	Count  int
}

type diagHookFunc func(diagRecord)

var (
	diagHookMu sync.Mutex
	diagHook   diagHookFunc
)

func setDiagHook(hook diagHookFunc) {
	diagHookMu.Lock()
	diagHook = hook
	diagHookMu.Unlock()
}

func currentDiagHook() diagHookFunc {
	diagHookMu.Lock()
	defer diagHookMu.Unlock()
	return diagHook
}

// emitDiagnostics records applied rewrites when diagnostics are enabled.
// The default is off. Nothing here includes prompt text, the matched span,
// the replacement, headers, credentials, paths, or the request body.
func emitDiagnostics(cfg config, records []diagRecord) {
	if !cfg.diagnostics {
		return
	}
	hook := currentDiagHook()
	for _, rec := range records {
		safe, ok := sanitizeDiag(rec)
		if !ok {
			continue
		}
		if hook != nil {
			hook(safe)
			continue
		}
		sendHostLog(safe)
	}
}

func aggregateDiag(applied []textEdit, scope textScope) []diagRecord {
	type key struct {
		rule  string
		scope string
	}
	var order []key
	counts := make(map[key]int)
	for _, edit := range applied {
		k := key{rule: edit.ruleID, scope: string(scope)}
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}
	out := make([]diagRecord, 0, len(order))
	for _, k := range order {
		out = append(out, diagRecord{RuleID: k.rule, Scope: k.scope, Count: counts[k]})
	}
	return out
}

func mergeDiag(records []diagRecord) []diagRecord {
	type key struct {
		rule  string
		scope string
	}
	var order []key
	counts := make(map[key]int)
	for _, rec := range records {
		k := key{rule: rec.RuleID, scope: rec.Scope}
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k] += rec.Count
	}
	out := make([]diagRecord, 0, len(order))
	for _, k := range order {
		out = append(out, diagRecord{RuleID: k.rule, Scope: k.scope, Count: counts[k]})
	}
	return out
}

func sanitizeDiag(rec diagRecord) (diagRecord, bool) {
	if rec.Count <= 0 || !safeDiagToken(rec.RuleID) || !safeDiagToken(rec.Scope) {
		return diagRecord{}, false
	}
	switch rec.Scope {
	case string(scopeInitial), string(scopeSubsequentSystem), string(scopeToolDescription):
	default:
		return diagRecord{}, false
	}
	return diagRecord{RuleID: rec.RuleID, Scope: rec.Scope, Count: rec.Count}, true
}

func safeDiagToken(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if r > unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_') {
			return false
		}
	}
	return true
}

// diagnosticPayload is the host.log body. The fields object has exactly three keys.
func diagnosticPayload(rec diagRecord) ([]byte, error) {
	safe, ok := sanitizeDiag(rec)
	if !ok {
		return nil, errJSON
	}
	body := struct {
		Level   string `json:"level"`
		Message string `json:"message"`
		Fields  struct {
			RuleID string `json:"rule_id"`
			Scope  string `json:"scope"`
			Count  int    `json:"count"`
		} `json:"fields"`
	}{}
	body.Level = "info"
	body.Message = "identity_filter"
	body.Fields.RuleID = safe.RuleID
	body.Fields.Scope = safe.Scope
	body.Fields.Count = safe.Count
	return json.Marshal(body)
}
