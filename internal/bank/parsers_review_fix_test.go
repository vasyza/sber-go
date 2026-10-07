package bank

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// Validate the original document, not strings/maps already repaired by Go's
// decoder. Distinct opaque IDs must not collapse and duplicate properties must
// not erase available history or replace malformed core amounts.
func TestDecodeJSONRejectsLossyOrAmbiguousDocuments(t *testing.T) {
	cases := map[string][]byte{
		"high_surrogate_id":           []byte(`{"body":{"operations":[{"uohId":"\ud800"}]}}`),
		"low_surrogate_id":            []byte(`{"body":{"operations":[{"uohId":"\udfff"}]}}`),
		"reversed_surrogate_pair":     []byte(`{"body":{"operations":[{"uohId":"\ude00\ud83d"}]}}`),
		"unpaired_surrogate_property": []byte(`{"\ud800":1}`),
		"invalid_utf8_id":             append(append([]byte(`{"body":{"operations":[{"uohId":"`), 0xff), []byte(`"}]}}`)...),
		"invalid_utf8_property":       append(append([]byte(`{"`), 0xff), []byte(`":1}`)...),
		"duplicate_history":           []byte(`{"body":{"operations":[{"uohId":"known"}],"operations":[]}}`),
		"escaped_duplicate_history":   []byte(`{"body":{"operations":[{"uohId":"known"}],"\u006fperations":[]}}`),
		"duplicate_operation_id":      []byte(`{"body":{"operations":[{"uohId":"first","\u0075ohId":"second"}]}}`),
		"duplicate_nested_money":      []byte(`{"body":{"operations":[{"operationAmount":{"amount":true,"\u0061mount":"1.00"}}]}}`),
		"duplicate_root_body":         []byte(`{"body":{"operations":[{"uohId":"known"}]},"body":{"operations":[]}}`),
		"trailing_document":           []byte(`{"body":{"operations":[]}} {}`),
		"trailing_garbage":            []byte(`{"body":{"operations":[]}}garbage`),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := DecodeJSON(bytes.NewReader(raw))
			var parseError *ParseError
			if got != nil || !errors.As(err, &parseError) || parseError.Field() != "JSON" {
				t.Fatalf("unusable original document accepted or unsafely reported: got=%v err=%v", got, err)
			}
		})
	}
}

func TestDecodeJSONRetainsValidIDsNumberLexemesAndKnownEmptyHistory(t *testing.T) {
	p := mustDecodeDomain(t, `{"body":{"operations":[{"uohId":"\ud83d\ude00","operationAmount":{"amount":1.2300E-02}},{"uohId":"\ufffd","operationAmount":{"amount":-0.00}},{"uohId":"\\ud800","operationAmount":{"amount":0.010000000000000000000000001}},{"uohId":9007199254740993}]}}`)
	raw := p["body"].(map[string]any)["operations"].([]any)
	for i, want := range []string{"1.2300E-02", "-0.00", "0.010000000000000000000000001"} {
		amount := raw[i].(map[string]any)["operationAmount"].(map[string]any)["amount"]
		if n, ok := amount.(json.Number); !ok || n.String() != want {
			t.Fatalf("number lexeme changed: got=%v want=%s", amount, want)
		}
	}
	ops, err := ParseOperations(p, "card:synthetic")
	if err != nil || len(ops) != 4 {
		t.Fatalf("valid Unicode history: %v %v", ops, err)
	}
	for i, want := range []string{"😀", "�", `\ud800`, "9007199254740993"} {
		if ops[i].ID != want {
			t.Fatalf("opaque ID changed: got=%q want=%q", ops[i].ID, want)
		}
	}
	if len(BuildCardLedger(ops)) != 4 {
		t.Fatal("distinct valid IDs collapsed")
	}
	p, err = DecodeJSON(strings.NewReader(" \n{\"body\":{\"operations\":[]}}\t "))
	if err != nil {
		t.Fatal(err)
	}
	ops, err = ParseOperations(p)
	if err != nil || ops == nil || len(ops) != 0 {
		t.Fatalf("known empty history changed: %v %v", ops, err)
	}
}

func TestProductsPropagateSourceErrorEnvelopes(t *testing.T) {
	fixture := readDomainGolden(t, "resources", "all_account_kinds_ambiguous_parent")
	for name, marker := range map[string]func(map[string]any){
		"success_false":          func(p map[string]any) { p["success"] = false },
		"root_source_error":      func(p map[string]any) { p["sourceErrorResponse"] = map[string]any{"code": "synthetic-error"} },
		"root_null_source_error": func(p map[string]any) { p["sourceErrorResponse"] = nil },
		"body_source_error": func(p map[string]any) {
			p["body"].(map[string]any)["sourceErrorResponse"] = map[string]any{"code": "synthetic-error"}
		},
		"body_empty_source_error": func(p map[string]any) { p["body"].(map[string]any)["sourceErrorResponse"] = map[string]any{} },
	} {
		t.Run(name, func(t *testing.T) {
			p := mustDecodeDomain(t, string(fixture.Data.Input))
			marker(p)
			products, err := ParseProducts(p)
			var parseError *ParseError
			if !errors.As(err, &parseError) || products.Accounts != nil || products.Cards != nil {
				t.Fatalf("source-error data became available products: %v %v", products, err)
			}
		})
	}
	for _, raw := range []string{
		`{"success":false,"error":{"code":"synthetic-error"}}`,
		`{"sourceErrorResponse":{"code":"synthetic-error"}}`,
		`{"body":{"sourceErrorResponse":{"code":"synthetic-error"}}}`,
	} {
		products, err := ParseProducts(mustDecodeDomain(t, raw))
		if err == nil || products.Accounts != nil || products.Cards != nil {
			t.Fatalf("error-only response became known empty products: %v %v", products, err)
		}
	}
}

func TestDomainEnvelopeRejectsErrorOnlyResponses(t *testing.T) {
	for name, parse := range map[string]func(map[string]any) error{
		"products":   func(p map[string]any) error { _, err := ParseProducts(p); return err },
		"operations": func(p map[string]any) error { _, err := ParseOperations(p); return err },
		"details":    func(p map[string]any) error { _, err := ParseOperationDetails(p); return err },
		"card_info":  func(p map[string]any) error { _, err := ParseCardInfo(p); return err },
		"pfm":        func(p map[string]any) error { _, err := ParsePFMAmounts(p); return err },
	} {
		t.Run(name, func(t *testing.T) {
			for _, raw := range []string{
				`{"error":{"code":"synthetic-error"}}`,
				`{"error":{"code":"synthetic-error"},"body":{"operations":[]}}`,
				`{"error":null,"body":{"operations":[]}}`,
			} {
				var parseError *ParseError
				if err := parse(mustDecodeDomain(t, raw)); !errors.As(err, &parseError) || parseError.Field() != "error" {
					t.Fatalf("error-only response did not propagate its envelope error: %v", err)
				}
			}
		})
	}
}

func TestOperationsRejectExplicitMalformedCoreMoneyShapes(t *testing.T) {
	// Source _money previously treated nonobjects as absent. The native strict
	// core contract intentionally rejects explicitly supplied malformed shapes.
	for _, value := range []string{`true`, `false`, `"NaN"`, `[]`, `""`, `0`, `[{}]`, `{"amount":null}`, `{"amount":true}`, `{"amount":"NaN"}`, `{"amount":""}`} {
		t.Run(value, func(t *testing.T) {
			p := mustDecodeDomain(t, `{"body":{"operations":[{"uohId":"synthetic","operationAmount":`+value+`}]}}`)
			ops, err := ParseOperations(p)
			var parseError *ParseError
			if ops != nil || !errors.As(err, &parseError) {
				t.Fatalf("malformed present core amount became absent: %v %v", ops, err)
			}
		})
	}
}

func TestOperationsKeepSourceAbsentNullEmptyAndSoftMoneySemantics(t *testing.T) {
	// These are actual source-compatible absence cases, not zero amounts.
	for _, field := range []string{``, `,"operationAmount":null`, `,"operationAmount":{}`, `,"operationAmount":{"currencyCode":"RUB"}`} {
		p := mustDecodeDomain(t, `{"body":{"operations":[{"uohId":"synthetic"`+field+`}]}}`)
		ops, err := ParseOperations(p, "card:synthetic-card")
		if err != nil || len(ops) != 1 || ops[0].Amount != nil {
			t.Fatalf("absent/null/empty money changed: %v %v", ops, err)
		}
		ledger := BuildCardLedger(ops)
		if len(ledger) != 1 || ledger[0].Direction != "unknown" || ledger[0].Amount != nil {
			t.Fatal("unknown amount became zero or acquired a guessed direction")
		}
	}
	for _, value := range []string{`null`, `{}`, `true`, `false`, `"NaN"`, `[]`, `{"amount":null}`, `{"amount":true}`, `{"amount":"NaN"}`, `{"amount":""}`} {
		p := mustDecodeDomain(t, `{"body":{"operations":[{"uohId":"synthetic","operationAmount":{"amount":"0.00","currencyCode":"RUB"},"billingAmount":`+value+`,"nationalAmount":`+value+`,"commission":`+value+`,"tips":`+value+`}]}}`)
		ops, err := ParseOperations(p)
		if err != nil || len(ops) != 1 {
			t.Fatalf("malformed optional metadata discarded page: %v %v", ops, err)
		}
		op := ops[0]
		if op.Amount == nil || op.Amount.Amount.String() != "0.00" || op.BillingAmount != nil || op.NationalAmount != nil || op.Commission != nil || op.Tips != nil || op.BalanceAfter != nil {
			t.Fatalf("core zero/soft metadata changed: %+v", op)
		}
	}
	p := mustDecodeDomain(t, `{"body":{"operations":[{"operationAmount":{"amount":0.010000000000000000000000001,"currencyCode":null,"currency":{"code":"EUR"}},"billingAmount":{"id":"synthetic-card","amount":true}}]}}`)
	ops, err := ParseOperations(p)
	if err != nil || len(ops) != 1 || ops[0].Amount == nil || ops[0].Amount.Amount.String() != "0.010000000000000000000000001" || ops[0].Amount.Currency != "EUR" || ops[0].BalanceAfter != nil || ops[0].BalanceAfterResourceID == nil || *ops[0].BalanceAfterResourceID != "synthetic-card" {
		t.Fatalf("exact core amount or optional resource metadata changed: %v %v", ops, err)
	}
}

func TestProductsRejectExplicitMalformedCoreMoneyShapes(t *testing.T) {
	for _, section := range []string{"ctaccounts", "sharingCtAccounts", "accounts", "cardsInWallet", "linked_card"} {
		t.Run(section, func(t *testing.T) {
			for _, value := range []string{`true`, `false`, `"NaN"`, `[]`, `""`, `0`} {
				t.Run(value, func(t *testing.T) {
					data := map[string]any{}
					field := "balance"
					key := section
					if section == "cardsInWallet" {
						field = "availableLimit"
					} else if section == "linked_card" {
						key, field = "cardsInWallet", "availableTotalLimit"
						data["ctaccounts"] = map[string]any{"data": []any{map[string]any{"id": "synthetic-account", "number": "synthetic-link"}}}
					}
					raw := mustDecodeDomain(t, `{"value":`+value+`}`)["value"]
					data[key] = map[string]any{"data": []any{map[string]any{"id": "synthetic-product", "isCTA": section == "linked_card", "cardAccount": "synthetic-link", field: raw}}}
					p := map[string]any{"body": map[string]any{"sections": map[string]any{"technicalSection": map[string]any{"sectionProductData": data}}}}
					products, err := ParseProducts(p)
					var parseError *ParseError
					if !errors.As(err, &parseError) || products.Accounts != nil || products.Cards != nil {
						t.Fatalf("malformed core product money became absence: %v %v", products, err)
					}
				})
			}
		})
	}
}

func TestProductsKeepSourceAbsentNullEmptyCoreMoneySemantics(t *testing.T) {
	for _, field := range []string{``, `,"balance":null`, `,"balance":{}`, `,"balance":{"currencyCode":"RUB"}`} {
		p := mustDecodeDomain(t, `{"body":{"sections":{"technicalSection":{"sectionProductData":{"ctaccounts":{"data":[{"id":"synthetic-account","number":"synthetic-link"`+field+`}]},"cardsInWallet":{"data":[{"id":"synthetic-card","isCTA":true,"cardAccount":"synthetic-link","availableLimit":{"amount":"50.00"}}]}}}}}}`)
		products, err := ParseProducts(p)
		if err != nil || len(products.Accounts) != 1 || len(products.Cards) != 1 || products.Accounts[0].Balance != nil {
			t.Fatalf("source absent/null/empty account money changed: %v %v", products, err)
		}
		card := products.Cards[0]
		if card.AccountID == nil || *card.AccountID != "synthetic-account" || card.AccountBalance != nil || card.Balance != nil || card.BalanceSource != nil {
			t.Fatalf("missing total limit was guessed from available limit/account balance: %+v", card)
		}
	}
	for _, value := range []string{`null`, `{}`, `{"currencyCode":"RUB"}`} {
		p := mustDecodeDomain(t, `{"body":{"sections":{"technicalSection":{"sectionProductData":{"cardsInWallet":{"data":[{"id":"synthetic-card","availableLimit":`+value+`}]}}}}}}`)
		products, err := ParseProducts(p)
		if err != nil || len(products.Cards) != 1 || products.Cards[0].Balance != nil || products.Cards[0].BalanceSource != nil {
			t.Fatalf("source empty card money changed: %v %v", products, err)
		}
	}
}

func TestProductsKeepSourceCompatibleKnownEmptySections(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{"body":{}}`,
		`{"body":{"sections":{"technicalSection":{"sectionProductData":{}}}}}`,
		`{"success":true,"body":{"sections":{"technicalSection":{"sectionProductData":{"ctaccounts":{"data":[]},"accounts":{"data":null},"sharingCtAccounts":{},"cardsInWallet":{"data":[]}}}}}}`,
		`{"success":"true","error":null,"body":{"sections":{"technicalSection":{"sectionProductData":{}}}}}`,
	} {
		products, err := ParseProducts(mustDecodeDomain(t, raw))
		if err != nil || products.Accounts == nil || products.Cards == nil || len(products.Accounts) != 0 || len(products.Cards) != 0 {
			t.Fatalf("ordinary empty products changed: %v %v", products, err)
		}
	}
}

func TestTimeFilterRejectsMoscowConversionOverflow(t *testing.T) {
	// Aware sorting keeps a valid source datetime in its own zone; only request
	// bounds convert to Moscow and must reject conversion outside years 1..9999.
	for _, text := range []string{"9999-12-31T23:59:59Z", "9999-12-31T23:59:59-23:59:59", "0001-01-01T00:00:00+23:59:59", "0001-01-01T00:00:00+03:00"} {
		t.Run(text, func(t *testing.T) {
			if key := OperationSortKey(text); key.IsZero() || !(TimeFilter{}).Contains(text) {
				t.Fatalf("valid aware sorting datetime rejected before conversion: %v", key)
			}
			for _, bounds := range [][2]string{{text, ""}, {"", text}} {
				filter, err := NewTimeFilter(bounds[0], bounds[1])
				var parseError *ParseError
				if !errors.As(err, &parseError) || filter.From != nil || filter.To != nil {
					t.Fatalf("Moscow conversion overflow published a request bound: %+v %v", filter, err)
				}
			}
		})
	}
}

func TestTimeFilterRetainsInclusiveSourceUpperSecondAtYear9999(t *testing.T) {
	filter, err := NewTimeFilter("9999-12-31", "9999-12-31")
	if err != nil {
		t.Fatal(err)
	}
	from, to := filter.ISOBounds()
	if from != "9999-12-31T00:00:00+03:00" || to != "9999-12-31T23:59:59+03:00" {
		t.Fatalf("source date-only bounds changed: %s %s", from, to)
	}
	for _, text := range []string{"9999-12-31T00:00:00", "9999-12-31T23:59:59+03:00", "9999-12-31T20:59:59Z"} {
		if !filter.Contains(text) {
			t.Fatalf("inclusive source upper second excluded: %s", text)
		}
	}
	for _, text := range []string{"9999-12-31T23:59:59.000001+03:00", "9999-12-31T21:00:00Z", "10000-01-01T00:00:00+03:00"} {
		if filter.Contains(text) {
			t.Fatalf("beyond source upper second included: %s", text)
		}
	}
	filter, err = NewTimeFilter("0001-01-01", "9999-12-31T20:59:59Z")
	if err != nil || filter.From.Year() != 1 || filter.To.Year() != 9999 {
		t.Fatalf("valid source range after Moscow conversion rejected: %+v %v", filter, err)
	}
}

func TestISODatetimeSpaceSeparatedAwareForms(t *testing.T) {
	// Confirmed by source datetime.fromisoformat and _request_moment, including
	// fractional seconds and historical Moscow offset seconds.
	for _, text := range []string{
		"2026-08-01 12:30:00+03:00", "2026-08-01 12:30:00Z",
		"2026-08-01 12:30:00+0300", "2026-08-01 12:30:00+03",
		"1900-07-01 12:00:00+02:30:17", "2026-08-01 12:30:00.123456+03:00",
		"2026-08-01 12:30:00,123456+03:00",
	} {
		t.Run(text, func(t *testing.T) {
			canonical := strings.Replace(text, " ", "T", 1)
			want := OperationSortKey(canonical)
			if got := OperationSortKey(text); got.IsZero() || !got.Equal(want) {
				t.Fatalf("space-separated aware sort changed: got=%v want=%v", got, want)
			}
			filter, err := NewTimeFilter(text, text)
			if err != nil || filter.From == nil || filter.To == nil || !filter.From.Equal(want) || !filter.To.Equal(want) || !filter.Contains(canonical) || !filter.Contains(text) {
				t.Fatalf("space-separated aware constructor/filter changed: %+v %v", filter, err)
			}
			if _, offset := filter.From.Zone(); offset == 0 {
				t.Fatal("aware request bound was not converted to Moscow")
			}
		})
	}
}

func TestISODatetimeRetainsSourceReducedClockAndNaiveForms(t *testing.T) {
	for text, canonical := range map[string]string{
		"2026-08-01":                 "2026-08-01T00:00:00+03:00",
		"2026-08-01T12:30:00":        "2026-08-01T12:30:00+03:00",
		"2026-08-01 12:30:00":        "2026-08-01T12:30:00+03:00",
		"2026-08-01T12:30":           "2026-08-01T12:30:00+03:00",
		"2026-08-01 12:30":           "2026-08-01T12:30:00+03:00",
		"2026-08-01T12":              "2026-08-01T12:00:00+03:00",
		"2026-08-01 12":              "2026-08-01T12:00:00+03:00",
		"2026-08-01T12:30:00.123456": "2026-08-01T12:30:00.123456+03:00",
		"2026-08-01 12:30:00,123456": "2026-08-01T12:30:00.123456+03:00",
		"2026-08-01T12:30+03:00":     "2026-08-01T12:30:00+03:00",
		"2026-08-01 12+03:00":        "2026-08-01T12:00:00+03:00",
		"2026-08-01 12:30Z":          "2026-08-01T12:30:00Z",
		"2026-08-01 12Z":             "2026-08-01T12:00:00Z",
	} {
		t.Run(text, func(t *testing.T) {
			want := OperationSortKey(canonical)
			if got := OperationSortKey(text); got.IsZero() || !got.Equal(want) {
				t.Fatalf("source ISO clock form changed: got=%v want=%v", got, want)
			}
			filter, err := NewTimeFilter(text, text)
			if err != nil || filter.From == nil || !filter.From.Equal(want) || !filter.Contains(canonical) || !filter.Contains(text) {
				t.Fatalf("source clock constructor/filter changed: %+v %v", filter, err)
			}
		})
	}
}

func TestISODatetimeRejectsInvalidSourceYearAndOffsetRange(t *testing.T) {
	for _, text := range []string{
		"0000-01-01", "0000-01-01T00:00:00Z", "0000-01-01 00:00:00",
		"10000-01-01", "2026-08-01T00:00:00+24:00", "2026-08-01T00:00:00-24:00",
		"2026-08-01T00:00:00+24:00:00", "2026-08-01T00:00:00-24:00:00",
		"2026-08-01T00:00:00+23:60",
	} {
		t.Run(text, func(t *testing.T) {
			if key := OperationSortKey(text); !key.Equal(time.Time{}) || key.Location() != time.UTC {
				t.Fatalf("invalid date missed oldest UTC sentinel: %v", key)
			}
			if (TimeFilter{}).Contains(text) {
				t.Fatal("unbounded filter accepted an invalid source datetime")
			}
			for _, bounds := range [][2]string{{text, ""}, {"", text}} {
				filter, err := NewTimeFilter(bounds[0], bounds[1])
				var parseError *ParseError
				if !errors.As(err, &parseError) || filter.From != nil || filter.To != nil {
					t.Fatalf("invalid constructor bound accepted or partially published: %+v %v", filter, err)
				}
			}
		})
	}
}

func TestISODatetimeAcceptsSourceYearAndHistoricalOffsetSecondsBounds(t *testing.T) {
	for _, text := range []string{"0001-01-01", "9999-12-31", "9999-12-31T23:59:59+03:00", "1900-07-01T12:00:00+02:30:17", "2026-08-01T00:00:00+23:59:59", "2026-08-01T00:00:00-23:59:59"} {
		t.Run(text, func(t *testing.T) {
			key, err := domainISOMoment(text)
			if err != nil || key.Year() < 1 || key.Year() > 9999 || !(TimeFilter{}).Contains(text) {
				t.Fatalf("source-supported datetime rejected: %v %v", key, err)
			}
			if _, err := NewTimeFilter(text, text); err != nil {
				t.Fatalf("source-supported constructor bounds rejected: %v", err)
			}
		})
	}
}

func FuzzDomainDatetimeBoundarySafety(f *testing.F) {
	for _, text := range []string{"0000-01-01", "0001-01-01", "9999-12-31", "9999-12-31T23:59:59Z", "2026-08-01T00:00:00+24:00", "2026-08-01 12:30:00+03:00", "1900-07-01T12:00:00+02:30:17", "2026-08-01 12"} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		moment, err := domainISOMoment(text)
		filter, boundError := NewTimeFilter(text, text)
		if err != nil {
			if !OperationSortKey(text).Equal(time.Time{}) || (TimeFilter{}).Contains(text) {
				t.Fatal("invalid datetime acquired a meaningful key or matched history")
			}
			// Empty strings intentionally mean unset constructor bounds.
			if text != "" && boundError == nil {
				t.Fatal("invalid datetime acquired a request bound")
			}
			return
		}
		_, offset := moment.Zone()
		if moment.Year() < 1 || moment.Year() > 9999 || offset <= -24*60*60 || offset >= 24*60*60 || !(TimeFilter{}).Contains(text) || !OperationSortKey(text).Equal(moment) {
			t.Fatal("accepted datetime violated source range/offset contract")
		}
		if boundError != nil {
			if filter.From != nil || filter.To != nil {
				t.Fatal("failed conversion partially published request bounds")
			}
			return
		}
		if filter.From == nil || filter.To == nil || filter.From.Year() < 1 || filter.To.Year() > 9999 || !filter.Contains(text) {
			t.Fatal("successful constructor published unusable or out-of-range bounds")
		}
	})
}
