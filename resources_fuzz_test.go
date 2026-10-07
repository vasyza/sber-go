package sber

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func FuzzResourceHistoryNPlusOne(f *testing.F) {
	f.Add(uint8(2), uint8(3), uint16(7), "card:4004")
	f.Add(uint8(100), uint8(101), uint16(0), "")
	f.Add(uint8(0), uint8(0), uint16(1), " card:bad")
	grammar := regexp.MustCompile(`^(?:card|ct-account|account):[A-Za-z0-9_-]{1,128}$`)
	f.Fuzz(func(t *testing.T, limit, count uint8, offset uint16, resource string) {
		if len(resource) > 300 {
			return
		}
		size := int(limit)
		valid := size >= 1 && size <= 100 && (resource == "" || grammar.MatchString(resource))
		r := &resourceScript{t: t}
		ids := []string{}
		if valid {
			n := int(count) % (size + 2)
			for i := 0; i < n; i++ {
				ids = append(ids, "fixture-"+strconv.Itoa(i))
			}
			r.steps = []resourceStep{{Call: resourceHistoryCall(int(offset), size+1, resource, "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse(ids...)}}
		}
		page, err := NewOperationsAPI(r).Page(context.Background(), OperationsPageOptions{Resource: resource, Limit: size, Offset: int(offset), From: "2026-07-01"})
		if !valid {
			if err == nil || len(r.calls) != 0 {
				t.Fatal("invalid page query reached requester")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		n := len(ids)
		want := n
		if want > size {
			want = size
		}
		if len(page.Operations) != want {
			t.Fatal("N+1 slicing")
		}
		if n > size {
			if page.NextOffset == nil || *page.NextOffset != int(offset)+size {
				t.Fatal("N+1 continuation")
			}
		} else if page.NextOffset != nil {
			t.Fatal("invented continuation")
		}
		r.done()
	})
}
func FuzzResourceNumericIDLiteral(f *testing.F) {
	for _, s := range []string{"12345", "001", "9007199254740991", "9007199254740992", " 1", "1\n", "١٢٣", "²"} {
		f.Add(s)
	}
	ascii := regexp.MustCompile(`^[0-9]{1,16}$`)
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 512 {
			return
		}
		id, err := resourceNumericProductID(value)
		if err == nil {
			if id <= 0 || id > 9007199254740991 {
				t.Fatal("numeric ID out of range")
			}
			if strings.TrimSpace(value) != value || strings.ContainsAny(value, "+-.\n\r\x7f") {
				t.Fatal("malformed ID repaired")
			}
		}
		if ascii.MatchString(value) {
			expected, parseErr := strconv.ParseInt(value, 10, 64)
			valid := parseErr == nil && expected > 0 && expected <= 9007199254740991
			if valid != (err == nil) || valid && id != expected {
				t.Fatal("ASCII ID contract mismatch")
			}
		}
	})
}
