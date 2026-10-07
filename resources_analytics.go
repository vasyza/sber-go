package sber

import (
	"context"
)

const PFMAmountsPath = "/pfpv_alf_mb/v1.00/alf/amounts"

// When options are supplied all booleans are explicit. Begin with the default
// constructor when changing just one of the source's default flags.
type AnalyticsOptions struct {
	IncomeType                                            string
	BetweenOwn, OpenBanking, ShowCategories, ShowProducts bool
}

func DefaultAnalyticsOptions() AnalyticsOptions {
	return AnalyticsOptions{IncomeType: "outcome", BetweenOwn: true, ShowCategories: true, ShowProducts: true}
}

type AnalyticsAPI struct{ requester BusinessRequester }

func NewAnalyticsAPI(requester BusinessRequester) *AnalyticsAPI { return &AnalyticsAPI{requester} }
func (a *AnalyticsAPI) Amounts(ctx context.Context, from, to string, options ...AnalyticsOptions) (PFMAmounts, error) {
	o := DefaultAnalyticsOptions()
	if len(options) > 1 {
		return PFMAmounts{}, NewParseError("options")
	}
	if len(options) == 1 {
		o = options[0]
	}
	if o.IncomeType != "income" && o.IncomeType != "outcome" {
		return PFMAmounts{}, NewParseError("income_type")
	}
	if from == "" || to == "" {
		return PFMAmounts{}, NewParseError("time_filter")
	}
	bounds, err := NewTimeFilter(from, to)
	if err != nil {
		return PFMAmounts{}, err
	}
	// The reference deliberately suffixes +03:00 even for historical naive
	// Moscow dates. Preserve its wire contract instead of changing endpoint input.
	f := bounds.From.In(domainMoscow).Format("2006-01-02T15:04:05") + "+03:00"
	t := bounds.To.In(domainMoscow).Format("2006-01-02T15:04:05") + "+03:00"
	toggle := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	body := map[string]any{"filter": map[string]any{"from": f, "to": t, "incomeType": o.IncomeType, "betweenOwnFilter": toggle(o.BetweenOwn), "openBankingFilter": toggle(o.OpenBanking), "productFilters": []any{map[string]any{"type": "CARD", "filter": "custom"}, map[string]any{"type": "CT_ACCOUNT", "filter": "custom"}, map[string]any{"type": "MANUAL", "filter": "custom"}}}, "display": map[string]any{"showCategoryAmounts": o.ShowCategories, "showProductAmounts": o.ShowProducts}}
	payload, err := a.requester.PostRead(ctx, PFMAmountsPath, body)
	if err != nil {
		return PFMAmounts{}, err
	}
	return ParsePFMAmounts(payload)
}
