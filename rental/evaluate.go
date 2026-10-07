// Package rental evaluates explicitly configured rental periods and receipts.
// It has no transport, wall-clock, messaging, scheduling or owner-profile access.
package rental

import (
	"slices"
	"sort"
	"time"
)

// Money is an exact amount in integer minor units. No conversion is performed.
type Money struct {
	Minor    int64
	Currency string
}

// Tenant identifies an owner-defined ledger, not a payment sender's name.
type Tenant struct {
	ID          string
	Currency    string
	LedgerStart time.Time
}

// Period is an explicit, anchored contractual obligation; End is exclusive.
type Period struct {
	ID       string
	TenantID string
	Start    time.Time
	End      time.Time
	DueAt    time.Time
	Price    Money
}

type Method string

const (
	Cash     Method = "cash"
	Transfer Method = "transfer"
)

// Receipt contains only explicit owner mapping and confirmation metadata.
type Receipt struct {
	ID         string
	TenantID   string
	ReceivedAt time.Time
	Method     Method
	Confirmed  bool
	Amount     Money
	// PossibleTenantIDs scopes owner review only; even one hint is not exact mapping.
	// Empty TenantID and no hints is fully unmapped and may affect every ledger.
	PossibleTenantIDs []string
}

// CollectionEvidence is an explicit owner assertion about all payment channels,
// including cash/opening prepayments, not merely an empty bank response.
type CollectionEvidence struct {
	TenantID        string
	CoverageStart   time.Time
	CoverageThrough time.Time
	ObservedAt      time.Time
	Complete        bool
	OwnerReconciled bool
	HasGaps         bool
	Truncated       bool
	PageUncertain   bool
}

// Input is a value snapshot. Evaluate does not mutate it.
type Input struct {
	AsOf     time.Time
	Tenants  []Tenant
	Periods  []Period
	Receipts []Receipt
	Evidence []CollectionEvidence
}

type State string

const (
	Paid    State = "PAID"
	Unknown State = "UNKNOWN"
	Due     State = "DUE"
	NotDue  State = "NOT_DUE"
)

// Reason is compact decision metadata suitable for an owner-only diagnostic.
type Reason string

const (
	MissingEvidence            Reason = "MISSING_EVIDENCE"
	EvidenceIncomplete         Reason = "EVIDENCE_INCOMPLETE"
	OwnerReconciliationMissing Reason = "OWNER_RECONCILIATION_MISSING"
	EvidenceBoundariesMissing  Reason = "EVIDENCE_BOUNDARIES_MISSING"
	EvidenceCoverageGap        Reason = "EVIDENCE_COVERAGE_GAP"
	EvidenceStale              Reason = "EVIDENCE_STALE"
	EvidenceHasGaps            Reason = "EVIDENCE_HAS_GAPS"
	EvidenceTruncated          Reason = "EVIDENCE_TRUNCATED"
	EvidencePageUncertain      Reason = "EVIDENCE_PAGE_UNCERTAIN"
	UnresolvedFunds            Reason = "UNRESOLVED_FUNDS"
	ReceiptUnmapped            Reason = "RECEIPT_UNMAPPED"
	ReceiptUnconfirmed         Reason = "RECEIPT_UNCONFIRMED"
	ReceiptAmbiguous           Reason = "RECEIPT_AMBIGUOUS"
	ReceiptAfterAsOf           Reason = "RECEIPT_AFTER_AS_OF"
)

type PeriodResult struct {
	Period    Period
	Applied   Money
	Remaining Money
	State     State
	Reasons   []Reason
}

type Allocation struct {
	TenantID  string
	ReceiptID string
	PeriodID  string
	Amount    Money
}

type Credit struct {
	TenantID  string
	ReceiptID string
	Amount    Money
}

type TenantTotal struct {
	TenantID        string
	Currency        string
	ReceivedMinor   int64
	AppliedMinor    int64
	CreditMinor     int64
	ObligationMinor int64
	RemainingMinor  int64
}

// ReviewItem retains unresolved receipt metadata without sender/source history.
type ReviewItem struct {
	ReceiptID         string
	Amount            Money
	ReceivedAt        time.Time
	AffectedTenantIDs []string
	Reasons           []Reason
}

// ReminderCandidate is metadata only, not authorization to contact a tenant.
type ReminderCandidate struct {
	TenantID  string
	PeriodID  string
	DueAt     time.Time
	Remaining Money
}

// Evaluation owns its storage. Accessors return independent copies.
type Evaluation struct {
	asOf        time.Time
	periods     []PeriodResult
	allocations []Allocation
	credits     []Credit
	totals      []TenantTotal
	candidates  []ReminderCandidate
	review      []ReviewItem
}

func (e Evaluation) Periods() []PeriodResult {
	out := slices.Clone(e.periods)
	for i := range out {
		out[i].Reasons = slices.Clone(out[i].Reasons)
	}
	return out
}
func (e Evaluation) Allocations() []Allocation { return append([]Allocation(nil), e.allocations...) }
func (e Evaluation) Credits() []Credit         { return append([]Credit(nil), e.credits...) }
func (e Evaluation) Totals() []TenantTotal     { return append([]TenantTotal(nil), e.totals...) }
func (e Evaluation) Candidates() []ReminderCandidate {
	return append([]ReminderCandidate(nil), e.candidates...)
}
func (e Evaluation) AsOf() time.Time { return e.asOf }
func (e Evaluation) Review() []ReviewItem {
	out := slices.Clone(e.review)
	for i := range out {
		out[i].AffectedTenantIDs = slices.Clone(out[i].AffectedTenantIDs)
		out[i].Reasons = slices.Clone(out[i].Reasons)
	}
	return out
}

func sameReceipt(a, b Receipt) bool {
	return a.ID == b.ID && a.TenantID == b.TenantID && a.ReceivedAt.Equal(b.ReceivedAt) && a.Method == b.Method && a.Confirmed == b.Confirmed && a.Amount == b.Amount && slices.Equal(a.PossibleTenantIDs, b.PossibleTenantIDs)
}

// Evaluate allocates confirmed, exactly mapped receipts to explicit periods.
func Evaluate(in Input) (Evaluation, error) {
	if err := validateRequired(in); err != nil {
		return Evaluation{}, err
	}
	out := Evaluation{asOf: in.AsOf}
	periods := append([]Period(nil), in.Periods...)
	sort.Slice(periods, func(i, j int) bool {
		if periods[i].TenantID != periods[j].TenantID {
			return periods[i].TenantID < periods[j].TenantID
		}
		return periods[i].Start.Before(periods[j].Start)
	})
	for _, p := range periods {
		out.periods = append(out.periods, PeriodResult{Period: p, Remaining: p.Price, Applied: Money{Currency: p.Price.Currency}, State: Unknown})
	}
	receipts := make([]Receipt, 0, len(in.Receipts))
	seen := make(map[string]Receipt)
	for _, r := range in.Receipts {
		// Receipt identity is semantic by instant, not caller timezone/monotonic representation.
		r.ReceivedAt = r.ReceivedAt.Round(0).UTC()
		r.PossibleTenantIDs = append([]string(nil), r.PossibleTenantIDs...)
		sort.Strings(r.PossibleTenantIDs)
		if prior, ok := seen[r.ID]; ok {
			if !sameReceipt(prior, r) {
				return Evaluation{}, &InputError{Code: ConflictingReceipt, Field: "Receipt.ID"}
			}
			continue
		}
		seen[r.ID] = r
		receipts = append(receipts, r)
	}
	if err := validateMoneyTotals(periods, receipts); err != nil {
		return Evaluation{}, err
	}
	sort.Slice(receipts, func(i, j int) bool {
		if !receipts[i].ReceivedAt.Equal(receipts[j].ReceivedAt) {
			return receipts[i].ReceivedAt.Before(receipts[j].ReceivedAt)
		}
		return receipts[i].ID < receipts[j].ID
	})
	for _, r := range receipts {
		if !r.Confirmed || r.TenantID == "" || r.ReceivedAt.After(in.AsOf) {
			continue
		}
		remaining := r.Amount.Minor
		for i := range out.periods {
			p := &out.periods[i]
			if p.Period.TenantID != r.TenantID || p.Period.Price.Currency != r.Amount.Currency {
				continue
			}
			applied := min(remaining, p.Remaining.Minor)
			if applied <= 0 {
				continue
			}
			p.Applied.Minor += applied
			p.Remaining.Minor -= applied
			remaining -= applied
			out.allocations = append(out.allocations, Allocation{TenantID: r.TenantID, ReceiptID: r.ID, PeriodID: p.Period.ID, Amount: Money{Minor: applied, Currency: r.Amount.Currency}})
			if p.Remaining.Minor == 0 {
				p.State = Paid
			}
		}
		if remaining > 0 {
			out.credits = append(out.credits, Credit{TenantID: r.TenantID, ReceiptID: r.ID, Amount: Money{Minor: remaining, Currency: r.Amount.Currency}})
		}
	}
	tenants := append([]Tenant(nil), in.Tenants...)
	sort.Slice(tenants, func(i, j int) bool { return tenants[i].ID < tenants[j].ID })
	for _, t := range tenants {
		total := TenantTotal{TenantID: t.ID, Currency: t.Currency}
		for _, r := range receipts {
			if r.Confirmed && r.TenantID == t.ID && !r.ReceivedAt.After(in.AsOf) {
				total.ReceivedMinor += r.Amount.Minor
			}
		}
		for _, p := range out.periods {
			if p.Period.TenantID == t.ID {
				total.ObligationMinor += p.Period.Price.Minor
				total.AppliedMinor += p.Applied.Minor
				total.RemainingMinor += p.Remaining.Minor
			}
		}
		for _, c := range out.credits {
			if c.TenantID == t.ID {
				total.CreditMinor += c.Amount.Minor
			}
		}
		out.totals = append(out.totals, total)
	}
	blocked := make(map[string]bool, len(tenants))
	for _, r := range receipts {
		var reasons []Reason
		afterAsOf := r.ReceivedAt.After(in.AsOf)
		if afterAsOf {
			reasons = append(reasons, ReceiptAfterAsOf)
		}
		if r.TenantID == "" {
			if len(r.PossibleTenantIDs) > 0 {
				reasons = append(reasons, ReceiptAmbiguous)
			} else {
				reasons = append(reasons, ReceiptUnmapped)
			}
		}
		if !r.Confirmed {
			reasons = append(reasons, ReceiptUnconfirmed)
		}
		if len(reasons) == 0 {
			continue
		}
		var affected []string
		if r.TenantID != "" {
			affected = []string{r.TenantID}
		} else if len(r.PossibleTenantIDs) > 0 {
			affected = append([]string(nil), r.PossibleTenantIDs...)
			sort.Strings(affected)
		} else {
			for _, t := range tenants {
				affected = append(affected, t.ID)
			}
		}
		if !afterAsOf {
			for _, id := range affected {
				blocked[id] = true
			}
		}
		out.review = append(out.review, ReviewItem{ReceiptID: r.ID, Amount: r.Amount, ReceivedAt: r.ReceivedAt, AffectedTenantIDs: affected, Reasons: reasons})
	}
	proof := make(map[string][]Reason, len(tenants))
	for _, t := range tenants {
		proof[t.ID] = evidenceReasons(t, in.AsOf, in.Evidence)
		if blocked[t.ID] {
			proof[t.ID] = append(proof[t.ID], UnresolvedFunds)
		}
	}
	for i := range out.periods {
		p := &out.periods[i]
		if p.Remaining.Minor == 0 {
			continue
		}
		p.Reasons = append([]Reason(nil), proof[p.Period.TenantID]...)
		if len(p.Reasons) != 0 {
			p.State = Unknown
		} else if in.AsOf.Before(p.Period.DueAt) {
			p.State = NotDue
		} else {
			p.State = Due
			out.candidates = append(out.candidates, ReminderCandidate{TenantID: p.Period.TenantID, PeriodID: p.Period.ID, DueAt: p.Period.DueAt, Remaining: p.Remaining})
		}
	}
	return out, nil
}
