package bank

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

const ChangeProductNamePath = "/ufs-productdetail/rest/v1/changeProductName"

var ErrResourceMutationsDisabled = errors.New("sber: resource mutations disabled")
var resourceCardName = regexp.MustCompile(`^[A-Za-zА-Яа-яЁё0-9 ,.\-]{1,56}$`)

func resourceValidateText(value, name string, allowEmpty bool) error {
	if !utf8.ValidString(value) || (!allowEmpty && strings.TrimSpace(value) == "") {
		return NewParseError(name)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return NewParseError(name)
		}
	}
	return nil
}

// Rename makes a single attempt. Definite rejection remains APIRejected;
// any other post-attempt error is uncertain and blocks further renames of this
// ID on this API. There is no automatic retry, reauth or compensating write.
func (a *CardsAPI) Rename(ctx context.Context, cardID any, name string) error {
	id, err := resourceNumericProductID(cardID)
	if err != nil {
		return err
	}
	if err = resourceValidateText(name, "name", false); err != nil {
		return err
	}
	if !resourceCardName.MatchString(name) {
		return NewParseError("name")
	}
	if !a.options.AllowMutations {
		return ErrResourceMutationsDisabled
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	a.renameMu.Lock()
	if a.renameAttempts == nil {
		a.renameAttempts = map[int64]bool{}
	}
	if a.renameAttempts[id] {
		a.renameMu.Unlock()
		return &sdkErrs.MutationUncertain{Message: "card rename already attempted; reread cards before deciding any further write"}
	}
	a.renameAttempts[id] = true
	a.renameMu.Unlock()
	payload, err := a.requester.Mutate(ctx, ChangeProductNamePath, map[string]any{"id": id, "name": name, "type": "card"}, map[string]string{}, "/app/cards/details/"+strconv.FormatInt(id, 10), false)
	if err == nil {
		err = domainEnvelope(payload)
	}
	var rejected *sdkErrs.APIRejected
	if err == nil || errors.As(err, &rejected) {
		a.renameMu.Lock()
		delete(a.renameAttempts, id)
		a.renameMu.Unlock()
		return err
	}
	return &sdkErrs.MutationUncertain{Message: "card rename result unknown; do not replay"}
}
