package rentalcli

import (
	"regexp"
	"time"
)

// Native time decoding accepts some non-RFC3339 spellings and silently drops
// fraction digits beyond nanoseconds. Constrain the spelling before parsing.
var timestampLexeme = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$`)

func validTimestamp(value string) bool {
	if !timestampLexeme.MatchString(value) {
		return false
	}
	// With supported precision and offset ranges established, native parsing
	// checks the actual calendar/clock without normalizing malformed input.
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}
