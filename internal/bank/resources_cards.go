package bank

import (
	"context"
	"strconv"
	"sync"
	"unicode/utf8"
)

const CardInfoPath = "/ufs-carddetail/rest/card/v1/cardInfo"

// Limits preserves the source helper's first-card selection. Nil means no
// limit snapshot returned, not a proven absence of bank-side limits.
func (a *CardsAPI) Limits(ctx context.Context, cardID any) (*CardLimits, error) {
	cards, err := a.Info(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, nil
	}
	limits := cards[0].Limits
	return &limits, nil
}

type CardsAPI struct {
	requester      BusinessRequester
	options        ResourceOptions
	renameMu       sync.Mutex
	renameAttempts map[int64]bool
	bindingMu      sync.Mutex
	binding        *entityBinding
}

func NewCardsAPI(requester BusinessRequester, options ...ResourceOptions) *CardsAPI {
	return &CardsAPI{requester: requester, options: resourceOptions(options)}
}

// Numeric IDs are checked in their original spelling before canonicalization.
// Decimal Unicode digits follow the source's isdigit/int behavior; spaces,
// signs, floats, bools and overlong IDs are never repaired into valid IDs.
func resourceNumericProductID(value any) (int64, error) {
	var text string
	switch v := value.(type) {
	case string:
		text = v
	case int:
		text = strconv.FormatInt(int64(v), 10)
	case int8:
		text = strconv.FormatInt(int64(v), 10)
	case int16:
		text = strconv.FormatInt(int64(v), 10)
	case int32:
		text = strconv.FormatInt(int64(v), 10)
	case int64:
		text = strconv.FormatInt(v, 10)
	case uint:
		text = strconv.FormatUint(uint64(v), 10)
	case uint8:
		text = strconv.FormatUint(uint64(v), 10)
	case uint16:
		text = strconv.FormatUint(uint64(v), 10)
	case uint32:
		text = strconv.FormatUint(uint64(v), 10)
	case uint64:
		text = strconv.FormatUint(v, 10)
	default:
		return 0, NewParseError("card_id")
	}
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) < 1 || utf8.RuneCountInString(text) > 16 {
		return 0, NewParseError("card_id")
	}
	digits := make([]byte, 0, 16)
	for _, r := range text {
		d, ok := domainDigitValue(r)
		if !ok {
			return 0, NewParseError("card_id")
		}
		digits = append(digits, byte('0'+d))
	}
	id, err := strconv.ParseInt(string(digits), 10, 64)
	if err != nil || id <= 0 || id > 9007199254740991 {
		return 0, NewParseError("card_id")
	}
	return id, nil
}
func (a *CardsAPI) Info(ctx context.Context, cardIDs ...any) ([]CardInfo, error) {
	if len(cardIDs) == 0 {
		return nil, NewParseError("card_ids")
	}
	ids := make([]string, 0, len(cardIDs))
	for _, value := range cardIDs {
		id, err := resourceNumericProductID(value)
		if err != nil {
			return nil, err
		}
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	payload, err := a.requester.PostRead(ctx, CardInfoPath, map[string]any{"cardIds": ids})
	if err != nil {
		return nil, err
	}
	return ParseCardInfo(payload)
}
