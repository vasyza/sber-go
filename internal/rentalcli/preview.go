// Package rentalcli provides a native offline ledger preview, never delivery.
package rentalcli

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"time"

	"github.com/vasyza/sber-go/internal/strictjson"
	"github.com/vasyza/sber-go/rental"
)

// MaximumInputBytes bounds noncredential preview input before allocation.
const MaximumInputBytes = 1 << 20

func fail(diagnostics io.Writer, code int, message string) int {
	if diagnostics != nil {
		_, _ = io.WriteString(diagnostics, message+"\n")
	}
	return code
}

// Run reads explicit noncredential ledger JSON and emits decision metadata.
// It never obtains a bank session, infers a contract or contacts a tenant.
func Run(input io.Reader, output, diagnostics io.Writer) int {
	if input == nil || output == nil {
		return fail(diagnostics, 2, "The preview input or output is not valid.")
	}
	data, err := io.ReadAll(io.LimitReader(input, MaximumInputBytes+1))
	if err != nil || len(data) > MaximumInputBytes || strictjson.Validate(data) != nil {
		return fail(diagnostics, 3, "The preview JSON is not valid.")
	}
	var in rental.Input
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var shape any
	if decoder.Decode(&shape) != nil || !exactShape(shape, reflect.TypeOf(in)) {
		return fail(diagnostics, 3, "The preview input format is not valid.")
	}
	if json.Unmarshal(data, &in) != nil {
		return fail(diagnostics, 3, "The preview input format is not valid.")
	}
	evaluation, err := rental.Evaluate(in)
	if err != nil {
		return fail(diagnostics, 4, "The rental ledger is not valid.")
	}
	value := map[string]any{
		"bank_authorization_checked": false, "reminders_enabled": false,
		"as_of": evaluation.AsOf(), "periods": evaluation.Periods(),
		"allocations": evaluation.Allocations(), "credits": evaluation.Credits(),
		"totals": evaluation.Totals(), "owner_review": evaluation.Review(),
		"candidate_decisions": evaluation.Candidates(),
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fail(diagnostics, 5, "The command cannot prepare the preview output.")
	}
	encoded = append(encoded, '\n')
	if n, err := output.Write(encoded); err != nil || n != len(encoded) {
		return fail(diagnostics, 5, "The command cannot write the preview.")
	}
	return 0
}

// JSON's case-distinct keys are not distinct to Go's struct decoder. Enforce
// canonical field spelling before decode so aliases cannot overwrite proof.
func exactShape(value any, typ reflect.Type) bool {
	if typ == reflect.TypeOf(time.Time{}) {
		text, ok := value.(string)
		return ok && validTimestamp(text)
	}
	switch typ.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		if typ == reflect.TypeOf(rental.CollectionEvidence{}) {
			// Missing negative proof assertions are unknown, not false. Reject
			// malformed supplied proof without changing the typed engine API.
			for _, name := range []string{"HasGaps", "Truncated", "PageUncertain"} {
				if _, present := object[name].(bool); !present {
					return false
				}
			}
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath == "" {
				fields[field.Name] = field.Type
			}
		}
		for key, item := range object {
			field, ok := fields[key]
			if !ok || !exactShape(item, field) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if value == nil {
			return true
		}
		items, ok := value.([]any)
		if !ok {
			return false
		}
		for _, item := range items {
			if !exactShape(item, typ.Elem()) {
				return false
			}
		}
		return true
	case reflect.String:
		_, ok := value.(string)
		return ok
	case reflect.Bool:
		_, ok := value.(bool)
		return ok
	case reflect.Int64:
		_, ok := value.(json.Number)
		return ok
	default:
		return false
	}
}
