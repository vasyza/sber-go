package sber

import (
	"context"
	"regexp"
)

const OperationDetailsPath = "/uoh-bh/v1/operation/details"

var resourceOpaqueID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func (a *OperationsAPI) Details(ctx context.Context, uohID string) (OperationDetail, error) {
	if !resourceOpaqueID.MatchString(uohID) {
		return OperationDetail{}, NewParseError("uoh_id")
	}
	payload, err := a.requester.PostRead(ctx, OperationDetailsPath, map[string]any{"uohId": uohID})
	if err != nil {
		return OperationDetail{}, err
	}
	return ParseOperationDetails(payload)
}
