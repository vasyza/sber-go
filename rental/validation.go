package rental

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ErrorCode describes a fail-closed schema/identity/arithmetic error.
type ErrorCode string

const (
	InvalidInput       ErrorCode = "INVALID_INPUT"
	ConflictingReceipt ErrorCode = "CONFLICTING_RECEIPT"
	Overflow           ErrorCode = "OVERFLOW"
)

// InputError provides a static field label, never raw source history.
// Any error from Evaluate is accompanied by an empty Evaluation.
type InputError struct {
	Code  ErrorCode
	Field string
}

func (e *InputError) Error() string { return "rental: " + string(e.Code) + ": " + e.Field }

func invalid(field string) error { return &InputError{Code: InvalidInput, Field: field} }

func validID(s string) bool {
	return s != "" && utf8.ValidString(s) && strings.TrimSpace(s) == s && strings.IndexFunc(s, unicode.IsControl) < 0
}

// Currency validation is syntactic, not a live ISO registry lookup.
func validCurrency(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := range s {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}

func validTime(t time.Time) bool {
	return !t.IsZero() && t.Year() >= 1 && t.Year() <= 9999 && t.UTC().Year() >= 1 && t.UTC().Year() <= 9999
}

func validateMoneyTotals(periods []Period, receipts []Receipt) error {
	obligations := make(map[string]int64)
	funds := make(map[string]int64)
	for _, p := range periods {
		if obligations[p.Price.Currency] > math.MaxInt64-p.Price.Minor {
			return &InputError{Code: Overflow, Field: "Period.Price.total"}
		}
		obligations[p.Price.Currency] += p.Price.Minor
	}
	for _, r := range receipts {
		if funds[r.Amount.Currency] > math.MaxInt64-r.Amount.Minor {
			return &InputError{Code: Overflow, Field: "Receipt.Amount.total"}
		}
		funds[r.Amount.Currency] += r.Amount.Minor
	}
	return nil
}

func validateRequired(in Input) error {
	if !validTime(in.AsOf) {
		return invalid("AsOf")
	}
	if len(in.Tenants) == 0 {
		return invalid("Tenants")
	}
	tenants := make(map[string]Tenant, len(in.Tenants))
	for _, t := range in.Tenants {
		if !validID(t.ID) {
			return invalid("Tenant.ID")
		}
		if _, exists := tenants[t.ID]; exists {
			return invalid("Tenant.ID.duplicate")
		}
		if !validCurrency(t.Currency) {
			return invalid("Tenant.Currency")
		}
		if !validTime(t.LedgerStart) {
			return invalid("Tenant.LedgerStart")
		}
		tenants[t.ID] = t
	}
	ids := make(map[string]bool, len(in.Periods))
	for _, p := range in.Periods {
		if !validID(p.ID) || !validID(p.TenantID) {
			return invalid("Period.ID/TenantID")
		}
		if ids[p.ID] {
			return invalid("Period.ID.duplicate")
		}
		ids[p.ID] = true
		t, exists := tenants[p.TenantID]
		if !exists {
			return invalid("Period.TenantID.unknown")
		}
		if !validTime(p.Start) || !validTime(p.End) || !validTime(p.DueAt) {
			return invalid("Period.Start/End/DueAt")
		}
		if !p.Start.Before(p.End) || p.Start.Before(t.LedgerStart) || p.DueAt.Before(t.LedgerStart) {
			return invalid("Period.boundaries")
		}
		if p.Price.Minor <= 0 || !validCurrency(p.Price.Currency) || p.Price.Currency != t.Currency {
			return invalid("Period.Price")
		}
	}
	periods := append([]Period(nil), in.Periods...)
	sort.Slice(periods, func(i, j int) bool {
		if periods[i].TenantID != periods[j].TenantID {
			return periods[i].TenantID < periods[j].TenantID
		}
		return periods[i].Start.Before(periods[j].Start)
	})
	for i := 1; i < len(periods); i++ {
		if periods[i-1].TenantID == periods[i].TenantID && periods[i].Start.Before(periods[i-1].End) {
			return invalid("Period.overlap")
		}
	}
	for _, r := range in.Receipts {
		if !validID(r.ID) {
			return invalid("Receipt.ID")
		}
		if !validTime(r.ReceivedAt) {
			return invalid("Receipt.ReceivedAt")
		}
		if r.Amount.Minor <= 0 || !validCurrency(r.Amount.Currency) {
			return invalid("Receipt.Amount")
		}
		if r.Method != Cash && r.Method != Transfer {
			return invalid("Receipt.Method")
		}
		if r.TenantID != "" && len(r.PossibleTenantIDs) != 0 {
			return invalid("Receipt.mapping-contradictory")
		}
		possibleIDs := make(map[string]bool, len(r.PossibleTenantIDs))
		for _, id := range r.PossibleTenantIDs {
			t, exists := tenants[id]
			if !validID(id) || !exists || possibleIDs[id] {
				return invalid("Receipt.PossibleTenantIDs")
			}
			if t.Currency != r.Amount.Currency {
				return invalid("Receipt.PossibleTenantIDs.currency-mismatch")
			}
			possibleIDs[id] = true
		}
		if r.TenantID != "" {
			t, exists := tenants[r.TenantID]
			if !validID(r.TenantID) || !exists {
				return invalid("Receipt.TenantID.unknown")
			}
			if t.Currency != r.Amount.Currency {
				return invalid("Receipt.Amount.currency-mismatch")
			}
		}
	}
	evidenceIDs := make(map[string]bool, len(in.Evidence))
	for _, e := range in.Evidence {
		if !validID(e.TenantID) {
			return invalid("CollectionEvidence.TenantID")
		}
		if _, exists := tenants[e.TenantID]; !exists {
			return invalid("CollectionEvidence.TenantID.unknown")
		}
		if evidenceIDs[e.TenantID] {
			return invalid("CollectionEvidence.TenantID.duplicate")
		}
		evidenceIDs[e.TenantID] = true
		for _, timestamp := range []time.Time{e.CoverageStart, e.CoverageThrough, e.ObservedAt} {
			if !timestamp.IsZero() && !validTime(timestamp) {
				return invalid("CollectionEvidence.timestamps")
			}
		}
		if !e.CoverageStart.IsZero() && !e.CoverageThrough.IsZero() && e.CoverageStart.After(e.CoverageThrough) {
			return invalid("CollectionEvidence.boundaries")
		}
	}
	return nil
}
