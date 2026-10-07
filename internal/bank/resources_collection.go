package bank

import (
	"context"
)

// HistoryMetadata distinguishes pagination termination from proven coverage.
// These endpoints supply no bank-cap proof, so WindowCompleteness remains
// unknown, even after an explicit requested window returns a final/empty page.
// A default 120-day query must never imply all-history completeness.
type HistoryMetadata struct {
	Resource, RequestedFrom, RequestedTo                 string
	ExplicitFrom, ExplicitTo, DefaultWindow              bool
	PagesRead                                            int
	PaginationExhausted, ClientCapReached, BankCapProven bool
	WindowCompleteness                                   string
}
type OperationsCollection struct {
	Operations []Operation
	Metadata   HistoryMetadata
}

func (a *OperationsAPI) walk(ctx context.Context, options []OperationsQuery, yield func(Operation) bool) (HistoryMetadata, error) {
	m := HistoryMetadata{WindowCompleteness: "unknown"}
	original := DefaultOperationsQuery()
	if len(options) == 1 {
		original = options[0]
	}
	m.Resource = original.Resource
	m.ExplicitFrom = original.From != ""
	m.ExplicitTo = original.To != ""
	m.DefaultWindow = !m.ExplicitFrom && !m.ExplicitTo
	q, err := a.queryOptions(options)
	if err != nil {
		return m, err
	}
	f, err := NewSourceTimeFilter(q.From, q.To)
	if err != nil {
		return m, err
	}
	m.RequestedFrom, m.RequestedTo, err = f.SourceRequestBounds()
	if err != nil {
		return m, err
	}
	seen := map[string]bool{}
	offset := 0
	for n := 0; n < q.MaxPages; n++ {
		page, err := a.Page(ctx, OperationsPageOptions{Resource: q.Resource, Offset: offset, Limit: q.Limit, From: q.From, To: q.To})
		if err != nil {
			return m, err
		}
		m.PagesRead++
		for _, op := range page.Operations {
			if !seen[op.ID] {
				seen[op.ID] = true
				if !yield(op) {
					return m, nil
				}
			}
		}
		if page.NextOffset == nil {
			m.PaginationExhausted = true
			return m, nil
		}
		offset = *page.NextOffset
	}
	m.ClientCapReached = true
	return m, &PaginationLimitError{MaxPages: q.MaxPages}
}

// Collect deliberately returns partial data together with an error. Unlike
// List's fail-closed result, this opt-in form carries explicit uncertainty and
// requested-window metadata; it is not evidence of bank history availability.
func (a *OperationsAPI) Collect(ctx context.Context, options ...OperationsQuery) (OperationsCollection, error) {
	result := OperationsCollection{Operations: []Operation{}}
	metadata, err := a.walk(ctx, options, func(op Operation) bool { result.Operations = append(result.Operations, op); return true })
	result.Metadata = metadata
	result.Operations = SortSourceOperations(result.Operations)
	return result, err
}
