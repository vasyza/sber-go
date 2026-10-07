package mcptools

import (
	"strconv"
	"strings"
)

// integralNumber classifies a valid JSON number without float conversion or
// materializing decimal powers. The caller has already checked JSON grammar.
func integralNumber(s string) bool {
	mantissa, exponent := s, ""
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mantissa, exponent = s[:i], s[i+1:]
	}
	zero := true
	for _, ch := range mantissa {
		if ch >= '1' && ch <= '9' {
			zero = false
			break
		}
	}
	if zero {
		return true
	}
	fractional := 0
	if i := strings.IndexByte(mantissa, '.'); i >= 0 {
		fractional = len(mantissa) - i - 1
	}
	trailing := 0
	for i := len(mantissa) - 1; i >= 0; i-- {
		if mantissa[i] == '.' {
			continue
		}
		if mantissa[i] != '0' {
			break
		}
		trailing++
	}
	power := int64(0)
	if exponent != "" {
		var err error
		power, err = strconv.ParseInt(exponent, 10, 64)
		if err != nil {
			if e, ok := err.(*strconv.NumError); ok && e.Err == strconv.ErrRange {
				return exponent[0] != '-'
			}
			return false
		}
	}
	return power >= int64(fractional-trailing)
}
