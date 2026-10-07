package rental

import "time"

func evidenceReasons(tenant Tenant, asOf time.Time, all []CollectionEvidence) []Reason {
	var evidence CollectionEvidence
	found := false
	for _, item := range all {
		if item.TenantID == tenant.ID {
			evidence, found = item, true
			break
		}
	}
	if !found {
		return []Reason{MissingEvidence}
	}
	var reasons []Reason
	if !evidence.Complete {
		reasons = append(reasons, EvidenceIncomplete)
	}
	if !evidence.OwnerReconciled {
		reasons = append(reasons, OwnerReconciliationMissing)
	}
	if evidence.CoverageStart.IsZero() || evidence.CoverageThrough.IsZero() || evidence.ObservedAt.IsZero() {
		reasons = append(reasons, EvidenceBoundariesMissing)
	} else {
		if evidence.CoverageStart.After(tenant.LedgerStart) || evidence.CoverageThrough.Before(asOf) {
			reasons = append(reasons, EvidenceCoverageGap)
		}
		if evidence.ObservedAt.Before(asOf) || evidence.ObservedAt.Before(evidence.CoverageThrough) {
			reasons = append(reasons, EvidenceStale)
		}
	}
	if evidence.HasGaps {
		reasons = append(reasons, EvidenceHasGaps)
	}
	if evidence.Truncated {
		reasons = append(reasons, EvidenceTruncated)
	}
	if evidence.PageUncertain {
		reasons = append(reasons, EvidencePageUncertain)
	}
	return reasons
}
