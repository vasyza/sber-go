package bank

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	"log"
	"reflect"
	"strings"
	"testing"
)

// Exercise actual fmt fallbacks, including private wrapper fields where fmt
// cannot invoke methods. Formats are dynamic so intentional bad verbs do not
// manufacture go-vet errors. No unsafe or explicit backing-pointer traversal.
func opaqueAssertDiagnostics[T any](t *testing.T, value, zero T, markers ...string) {
	t.Helper()
	copied := value
	var nilPointer *T
	var boxed any = value
	var privateBox = struct{ value T }{value}
	cases := []struct {
		name  string
		value any
	}{
		{"value", value}, {"pointer", &value}, {"copied", copied},
		{"zero", zero}, {"zero_pointer", &zero}, {"nil_pointer", nilPointer},
		{"slice", []T{value, copied}}, {"pointer_slice", []*T{&value, &copied}},
		{"array", [2]T{value, copied}},
		{"map", map[string]T{"opaque": value}},
		{"interfaces", []any{value, &value, []any{value}}},
		{"nested_map", map[string]any{"opaque": []any{[]T{value}, map[string]T{"copy": copied}}}},
		{"exported_wrapper", struct{ Value T }{value}},
		{"private_wrapper", privateBox}, {"private_wrapper_pointer", &privateBox},
		{"reflect_value", reflect.ValueOf(value)}, {"reflect_pointer", reflect.ValueOf(&value)},
		{"reflect_interface", reflect.ValueOf(&boxed).Elem()},
		{"reflect_private_field", reflect.ValueOf(privateBox).Field(0)},
	}
	formats := []string{"%v", "%+v", "%#v", "%s", "%q", "%p", "%#p", "%+p", "%w", "%+w", "%#w", "%20w", "%.7w", "%d", "%x", "%t", "%J", "prefix=%w suffix", "", "%*v", "%.*v", "%[2]v", "%[0]w", "%w %w"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("ordinary_JSON_and_export", func(t *testing.T) {
				for _, marshal := range []func(any) ([]byte, error){json.Marshal, ExportJSON} {
					data, err := marshal(tc.value)
					if err != nil {
						t.Fatal("opaque ordinary JSON failed", err)
					}
					for _, marker := range markers {
						if marker != "" && strings.Contains(string(data), marker) {
							t.Errorf("ordinary JSON disclosed marker %q: %s", marker, data)
						}
					}
				}
			})
			for _, format := range formats {
				t.Run(format, func(t *testing.T) {
					var buf bytes.Buffer
					log.New(&buf, "", 0).Printf(format, tc.value)
					err := fmt.Errorf(format, tc.value)
					texts := []string{fmt.Sprintf(format, tc.value), buf.String(), err.Error()}
					// Errorf's fallback/wrapper is then itself ordinary logged data.
					for _, nestedFormat := range []string{"%v", "%#v", "%w"} {
						texts = append(texts, fmt.Sprintf(nestedFormat, err))
					}
					for i, text := range texts {
						for _, marker := range markers {
							if marker != "" && strings.Contains(text, marker) {
								t.Errorf("diagnostic %d disclosed marker %q: %s", i, marker, text)
							}
						}
					}
				})
			}
		})
	}
}

func TestOpaqueFrontendConfigDiagnostics(t *testing.T) {
	config := sdkSession.NewFrontendConfig("fixture-opaque-base", "fixture-opaque-process", 7, "fixture-opaque-N", "fixture-opaque-G", true, true)
	opaqueAssertDiagnostics(t, config, sdkSession.FrontendConfig{}, "fixture-opaque-base", "fixture-opaque-process", "fixture-opaque-N", "fixture-opaque-G")
}

func TestOpaqueFrontendConfigRetainsEveryFieldAndZero(t *testing.T) {
	for _, flags := range [][2]bool{{false, false}, {false, true}, {true, false}, {true, true}} {
		config := sdkSession.NewFrontendConfig("raw-base", "raw-process", -7, "raw-N", "raw-G", flags[0], flags[1])
		copied := config
		config = sdkSession.FrontendConfig{}
		if copied.BaseURL() != "raw-base" || copied.ProcessID() != "raw-process" || copied.PINLength() != -7 || copied.NHex() != "raw-N" || copied.GHex() != "raw-G" || copied.SeamlessWeb() != flags[0] || copied.RedirectPost() != flags[1] {
			t.Fatal("constructor/copy changed an explicitly supplied field or validated/synthesized a runtime")
		}
		if copied == (sdkSession.FrontendConfig{}) || config != (sdkSession.FrontendConfig{}) {
			t.Fatal("copy or zero identity changed")
		}
		independent := sdkSession.NewFrontendConfig("raw-base", "raw-process", -7, "raw-N", "raw-G", flags[0], flags[1])
		if copied == independent {
			t.Fatal("independent nonzero constructors unexpectedly interned identities")
		}
	}
	zero := sdkSession.FrontendConfig{}
	if zero.BaseURL() != "" || zero.ProcessID() != "" || zero.PINLength() != 0 || zero.NHex() != "" || zero.GHex() != "" || zero.SeamlessWeb() || zero.RedirectPost() || sdkSession.NewFrontendConfig("", "", 0, "", "", false, false) != zero {
		t.Fatal("zero/explicit-empty construction changed")
	}
	partial := sdkSession.NewFrontendConfig("", "", 0, "", "", false, true)
	if partial.BaseURL() != "" || partial.NHex() != "" || partial.GHex() != "" || partial.PINLength() != 0 || partial.SeamlessWeb() || !partial.RedirectPost() {
		t.Fatal("partial synthetic flags gained hidden crypto or network defaults")
	}
	b, err := partial.MarshalJSON()
	if err != nil || string(b) != `"\u003credacted\u003e"` {
		t.Fatal("default JSON policy changed")
	}
}

func TestOpaqueParseErrorDiagnostics(t *testing.T) {
	pe := NewParseError("fixture-opaque-schema-field")
	t.Run("native", func(t *testing.T) { opaqueAssertDiagnostics(t, *pe, ParseError{}, "fixture-opaque-schema-field") })
	t.Run("error", func(t *testing.T) { opaqueAssertDiagnostics(t, pe, (*ParseError)(nil), "fixture-opaque-schema-field") })
}

func TestOpaqueParseErrorFieldIdentityAndJSON(t *testing.T) {
	pe := NewParseError("fixture-opaque-field-access")
	copied := *pe
	if pe.Field() != "fixture-opaque-field-access" {
		t.Fatal("schema field lost")
	}
	format := "context: %w"
	wrapped := fmt.Errorf(format, pe)
	var found *ParseError
	if !errors.As(wrapped, &found) || found != pe || !errors.Is(wrapped, pe) || errors.Unwrap(pe) != nil {
		t.Fatal("typed error identity/cause behavior changed")
	}
	pe.SDKError()
	if pe.Error() != "sber: invalid domain data" {
		t.Fatal("static parser error changed")
	}
	for _, value := range []any{pe, copied, []any{pe, copied}, struct{ Error any }{copied}} {
		b, err := json.Marshal(value)
		if err != nil || strings.Contains(string(b), "fixture-opaque-field-access") {
			t.Fatal("ordinary native/pointer JSON disclosed field")
		}
	}
	for _, zero := range []*ParseError{{}, nil} {
		read, ok := any(zero).(interface{ Field() string })
		if !ok || read.Field() != "" || zero.Error() != pe.Error() {
			t.Fatal("zero/nil error field is not explicitly readable")
		}
	}
}

func TestOpaqueTransferResourceDiagnostics(t *testing.T) {
	resource := NewTransferResource("fixture-opaque-resource", "card", "fixture-opaque-name", "RUB")
	opaqueAssertDiagnostics(t, resource, TransferResource{}, "fixture-opaque-resource", "fixture-opaque-name")
}

func TestOpaqueTransferResourceRetainsEveryField(t *testing.T) {
	resource := NewTransferResource("raw-resource", "card", "Карта 4111 1111 1111 1111", "RUB")
	read, ok := any(resource).(interface {
		ID() string
		Kind() string
		Name() string
		Currency() string
	})
	if !ok || read.ID() != "raw-resource" || read.Kind() != "card" || read.Name() != "Карта 4111 1111 1111 1111" || read.Currency() != "RUB" {
		t.Fatal("explicit resource access lost an ID, kind, unmasked name or currency")
	}
	copied := resource
	resource = TransferResource{}
	if copied == resource {
		t.Fatal("overwriting a caller wrapper mutated its copy")
	}
	b, err := json.Marshal(copied)
	var got map[string]any
	if err != nil || json.Unmarshal(b, &got) != nil || !reflect.DeepEqual(got, map[string]any{"id": "<redacted>", "kind": "card", "name": "<redacted>", "currency": "RUB"}) {
		t.Fatal("resource JSON policy changed")
	}
	zero, ok := any(resource).(interface {
		ID() string
		Kind() string
		Name() string
		Currency() string
	})
	if !ok || zero.ID() != "" || zero.Kind() != "" || zero.Name() != "" || zero.Currency() != "" {
		t.Fatal("resource zero value no longer reads empty")
	}
}

func TestOpaqueTransferDraftDiagnostics(t *testing.T) {
	source := NewTransferResource("fixture-opaque-draft-source", "card", "fixture-opaque-draft-source-name", "RUB")
	destination := NewTransferResource("fixture-opaque-draft-destination", "account", "fixture-opaque-draft-destination-name", "USD")
	draft := NewTransferDraft("fixture-opaque-draft-PID", "flow", "state", []TransferResource{source}, []TransferResource{destination})
	opaqueAssertDiagnostics(t, draft, TransferDraft{}, "fixture-opaque-draft-PID", "fixture-opaque-draft-source", "fixture-opaque-draft-destination")
}

func TestOpaqueTransferDraftDefensiveSlicesAndFields(t *testing.T) {
	source := NewTransferResource("source-ID", "card", "source-name", "RUB")
	destination := NewTransferResource("destination-ID", "account", "destination-name", "USD")
	sources, destinations := []TransferResource{source, source}, []TransferResource{destination}
	draft := NewTransferDraft("raw-draft-PID", "raw-flow", "raw-state", sources, destinations)
	sources[0], destinations[0] = TransferResource{}, TransferResource{}
	if len(draft.Sources()) != 2 || len(draft.Destinations()) != 1 || draft.Sources()[0].ID() != "source-ID" || draft.Destinations()[0].ID() != "destination-ID" {
		t.Fatal("constructor retained caller slice aliases or lost resource list ordering")
	}
	first, second := draft.Sources(), draft.Destinations()
	first[0], second[0] = TransferResource{}, TransferResource{}
	if draft.Sources()[0].ID() != "source-ID" || draft.Destinations()[0].ID() != "destination-ID" {
		t.Fatal("accessors exposed backing slice aliases")
	}
	copied := draft
	draft = TransferDraft{}
	read, ok := any(copied).(interface {
		PID() string
		Flow() string
		State() string
		Sources() []TransferResource
		Destinations() []TransferResource
	})
	if !ok || read.PID() != "raw-draft-PID" || read.Flow() != "raw-flow" || read.State() != "raw-state" || !reflect.DeepEqual(read.Sources(), []TransferResource{source, source}) || !reflect.DeepEqual(read.Destinations(), []TransferResource{destination}) {
		t.Fatal("copy/getters lost a workflow field or resource")
	}
	b, err := json.Marshal(copied)
	var got map[string]any
	if err != nil || json.Unmarshal(b, &got) != nil || !reflect.DeepEqual(got, map[string]any{"pid": "<redacted>", "flow": "raw-flow", "state": "raw-state", "sources": float64(2), "destinations": float64(1)}) {
		t.Fatal("draft JSON policy changed")
	}
}

func TestOpaqueTransferDraftNilAndEmptyLists(t *testing.T) {
	for _, lists := range [][]TransferResource{nil, {}} {
		draft := NewTransferDraft("", "", "", lists, lists)
		read, ok := any(draft).(interface {
			PID() string
			Flow() string
			State() string
			Sources() []TransferResource
			Destinations() []TransferResource
		})
		if !ok || read.PID() != "" || read.Flow() != "" || read.State() != "" || (read.Sources() == nil) != (lists == nil) || (read.Destinations() == nil) != (lists == nil) || len(read.Sources()) != 0 || len(read.Destinations()) != 0 {
			t.Fatal("zero, nil and explicit-empty resource lists conflated")
		}
	}
}

func TestOpaquePreparedTransferDiagnostics(t *testing.T) {
	amount, err := ParseDecimal("9007199254740993.0100")
	if err != nil {
		t.Fatal(err)
	}
	prepared := NewPreparedTransfer("fixture-opaque-prepared-PID", "flow", "state", "fixture-opaque-source-ID", "fixture-opaque-destination-ID", Money{Amount: amount, Currency: "RUB"}, "fixture-opaque-purpose")
	opaqueAssertDiagnostics(t, prepared, PreparedTransfer{}, "fixture-opaque-prepared-PID", "fixture-opaque-source-ID", "fixture-opaque-destination-ID", "9007199254740993.0100", "90071992547409930100", "fixture-opaque-purpose")
}

func TestOpaquePreparedTransferFieldsAndExactMoneyCopies(t *testing.T) {
	for _, text := range []string{"0.00", "-0.00", "9007199254740993.0100", "0.010000000000000000000000001", "4111111111111111.00", "1e-9"} {
		t.Run(text, func(t *testing.T) {
			d, err := ParseDecimal(text)
			if err != nil {
				t.Fatal(err)
			}
			money := Money{Amount: d, Currency: "RUB"}
			prepared := NewPreparedTransfer("raw-prepared-PID", "flow", "state", "raw-source-ID", "raw-destination-ID", money, "Назначение 4111 1111 1111 1111")
			money.Amount, money.Currency = Decimal{}, "changed-caller"
			read, ok := any(prepared).(interface {
				PID() string
				Flow() string
				State() string
				SourceID() string
				DestinationID() string
				Amount() Money
				PaymentPurpose() string
			})
			if !ok || read.PID() != "raw-prepared-PID" || read.Flow() != "flow" || read.State() != "state" || read.SourceID() != "raw-source-ID" || read.DestinationID() != "raw-destination-ID" || read.PaymentPurpose() != "Назначение 4111 1111 1111 1111" || read.Amount() != (Money{Amount: d, Currency: "RUB"}) {
				t.Fatal("constructor/getters lost an ID, workflow field, raw purpose, decimal scale/sign or currency")
			}
			got := read.Amount()
			got.Amount, got.Currency = Decimal{}, "changed-getter"
			copied := prepared
			prepared = PreparedTransfer{}
			copyRead := any(copied).(interface{ Amount() Money })
			if copyRead.Amount() != (Money{Amount: d, Currency: "RUB"}) {
				t.Fatal("Money getter/caller wrapper alias mutated immutable prepared copy")
			}
			b, err := json.Marshal(copied)
			var encoded map[string]any
			if err != nil || json.Unmarshal(b, &encoded) != nil || !reflect.DeepEqual(encoded, map[string]any{"pid": "<redacted>", "flow": "flow", "state": "state", "amount": "<redacted>", "currency": "RUB"}) {
				t.Fatal("prepared JSON policy changed or exact amount became public")
			}
		})
	}
	zero := PreparedTransfer{}
	read, ok := any(zero).(interface {
		PID() string
		Flow() string
		State() string
		SourceID() string
		DestinationID() string
		Amount() Money
		PaymentPurpose() string
	})
	if !ok || read.PID() != "" || read.Flow() != "" || read.State() != "" || read.SourceID() != "" || read.DestinationID() != "" || read.Amount() != (Money{}) || read.PaymentPurpose() != "" {
		t.Fatal("prepared zero gained fields or a fabricated amount")
	}
}

func TestOpaqueTransferResultDiagnostics(t *testing.T) {
	document := "fixture-opaque-document-ID"
	result := NewTransferResult("fixture-opaque-result-PID", "flow", "state", &document)
	opaqueAssertDiagnostics(t, result, TransferResult{}, "fixture-opaque-result-PID", "fixture-opaque-document-ID")
}

func TestOpaqueTransferResultDefensiveDocumentAndFields(t *testing.T) {
	document := "raw-document-ID"
	result := NewTransferResult("raw-result-PID", "raw-flow", "raw-state", &document)
	document = "changed-caller"
	if result.DocumentID() == nil || *result.DocumentID() != "raw-document-ID" {
		t.Fatal("constructor retained the caller's document pointer alias")
	}
	first, second := result.DocumentID(), result.DocumentID()
	if first == second || first == &document {
		t.Fatal("document accessors expose a backing/caller pointer alias")
	}
	*first, *second = "changed-getter-1", "changed-getter-2"
	copied := result
	result = TransferResult{}
	read, ok := any(copied).(interface {
		PID() string
		Flow() string
		State() string
		DocumentID() *string
	})
	if !ok || read.PID() != "raw-result-PID" || read.Flow() != "raw-flow" || read.State() != "raw-state" || read.DocumentID() == nil || *read.DocumentID() != "raw-document-ID" {
		t.Fatal("copy/getters lost workflow or document fields")
	}
	b, err := json.Marshal(copied)
	var got map[string]any
	if err != nil || json.Unmarshal(b, &got) != nil || !reflect.DeepEqual(got, map[string]any{"pid": "<redacted>", "flow": "raw-flow", "state": "raw-state", "document_id_present": true}) {
		t.Fatal("result JSON policy changed")
	}
}

func TestOpaqueTransferResultNilAndEmptyDocument(t *testing.T) {
	empty := ""
	for _, document := range []*string{nil, &empty} {
		result := NewTransferResult("", "", "", document)
		read, ok := any(result).(interface {
			PID() string
			Flow() string
			State() string
			DocumentID() *string
		})
		if !ok || read.PID() != "" || read.Flow() != "" || read.State() != "" || (read.DocumentID() == nil) != (document == nil) {
			t.Fatal("zero/nil/explicit-empty optional IDs conflated")
		}
		if document != nil && *read.DocumentID() != "" {
			t.Fatal("empty document contents changed")
		}
		b, err := json.Marshal(result)
		var got map[string]any
		if err != nil || json.Unmarshal(b, &got) != nil || got["document_id_present"] != (document != nil) {
			t.Fatal("optional document presence changed")
		}
	}
}

// Post-GREEN strengthening: native zero/identity, named type method bypasses
// and concurrent reads of independent getter copies. No extra repair cycle.
func TestOpaqueParseErrorNilMarshalJSONRetained(t *testing.T) {
	// The original pointer-receiver method deliberately did not dereference its
	// receiver. Preserve that nil default, not only json.Marshal's nil shortcut.
	var pe *ParseError
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Error("original nil ParseError.MarshalJSON behavior regressed to a panic")
		}
	}()
	data, err := pe.MarshalJSON()
	if err != nil || string(data) != `{"error":"parse_error"}` {
		t.Fatal("nil direct parser-error JSON policy changed")
	}
	data, err = json.Marshal(pe)
	if err != nil || string(data) != "null" {
		t.Fatal("standard nil parser-error JSON changed")
	}
}

func TestOpaqueNativeZeroAndIdentity(t *testing.T) {
	if sdkSession.NewFrontendConfig("", "", 0, "", "", false, false) != (sdkSession.FrontendConfig{}) ||
		NewTransferResource("", "", "", "") != (TransferResource{}) ||
		NewTransferDraft("", "", "", nil, nil) != (TransferDraft{}) ||
		NewPreparedTransfer("", "", "", "", "", Money{}, "") != (PreparedTransfer{}) ||
		NewTransferResult("", "", "", nil) != (TransferResult{}) ||
		*NewParseError("") != (ParseError{}) {
		t.Fatal("all-empty explicit constructors differ from native zero")
	}
	resource := NewTransferResource("id", "kind", "name", "currency")
	draft := NewTransferDraft("pid", "flow", "state", []TransferResource{resource}, nil)
	prepared := NewPreparedTransfer("pid", "flow", "state", "source", "dest", Money{}, "purpose")
	result := NewTransferResult("pid", "flow", "state", nil)
	copyResource, copyDraft, copyPrepared, copyResult := resource, draft, prepared, result
	if resource != copyResource || draft != copyDraft || prepared != copyPrepared || result != copyResult {
		t.Fatal("value copies lost immutable identity")
	}
	if resource == NewTransferResource("id", "kind", "name", "currency") ||
		draft == NewTransferDraft("pid", "flow", "state", []TransferResource{resource}, nil) ||
		prepared == NewPreparedTransfer("pid", "flow", "state", "source", "dest", Money{}, "purpose") ||
		result == NewTransferResult("pid", "flow", "state", nil) {
		t.Fatal("independent nonzero constructions unexpectedly share/intern identity")
	}
	pe := NewParseError("raw-copy-field")
	copied := *pe
	*pe = ParseError{}
	if copied.Field() != "raw-copy-field" || pe.Field() != "" {
		t.Fatal("overwriting the caller error wrapper changed its immutable copy")
	}
}

func TestOpaqueNamedTypesAndPrivateLayout(t *testing.T) {
	type frontendNative sdkSession.FrontendConfig
	type resourceNative TransferResource
	type draftNative TransferDraft
	type preparedNative PreparedTransfer
	type resultNative TransferResult
	type parseNative ParseError
	frontend := sdkSession.NewFrontendConfig("fixture-layout-base", "fixture-layout-process", 7, "fixture-layout-N", "fixture-layout-G", true, true)
	resource := NewTransferResource("fixture-layout-resource", "card", "fixture-layout-name", "RUB")
	draft := NewTransferDraft("fixture-layout-draft", "flow", "state", []TransferResource{resource}, nil)
	amount, _ := ParseDecimal("9007199254740993.0100")
	prepared := NewPreparedTransfer("fixture-layout-prepared", "flow", "state", "fixture-layout-source", "fixture-layout-dest", Money{Amount: amount, Currency: "RUB"}, "fixture-layout-purpose")
	document := "fixture-layout-document"
	result := NewTransferResult("fixture-layout-result", "flow", "state", &document)
	pe := NewParseError("fixture-layout-field")
	for _, value := range []any{frontend, resource, draft, prepared, result, *pe} {
		typ := reflect.TypeOf(value)
		if typ.NumField() != 1 || typ.Field(0).PkgPath == "" || typ.Field(0).Type.Kind() != reflect.Pointer {
			t.Fatal("native layout exposes mutable/raw fields", typ)
		}
		terminal := typ.Field(0).Type.Elem().Kind()
		if terminal != reflect.Pointer && terminal != reflect.String {
			t.Fatal("fmt can dereference the terminal private pointer into raw composite storage", typ)
		}
	}
	for _, value := range []any{frontendNative(frontend), resourceNative(resource), draftNative(draft), preparedNative(prepared), resultNative(result), parseNative(*pe)} {
		for _, format := range []string{"%v", "%#v", "%p", "%w", "%s", "%q", "%J"} {
			text := fmt.Sprintf(format, value)
			if strings.Contains(text, "fixture-layout-") || strings.Contains(text, "90071992547409930100") {
				t.Fatal("named type bypassed opaque storage", text)
			}
		}
		data, err := json.Marshal(value)
		if err != nil || strings.Contains(string(data), "fixture-layout-") {
			t.Fatal("named native JSON leaked raw private storage")
		}
	}
}

func TestOpaqueDefaultLoggerFormatting(t *testing.T) {
	writer, flags, prefix := log.Writer(), log.Flags(), log.Prefix()
	t.Cleanup(func() { log.SetOutput(writer); log.SetFlags(flags); log.SetPrefix(prefix) })
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")
	document := "fixture-default-log-document"
	values := []any{
		sdkSession.NewFrontendConfig("fixture-default-log-base", "fixture-default-log-process", 5, "fixture-default-log-N", "fixture-default-log-G", true, true),
		NewParseError("fixture-default-log-field"),
		NewTransferResource("fixture-default-log-resource", "card", "fixture-default-log-name", "RUB"),
		NewTransferDraft("fixture-default-log-draft", "flow", "state", nil, nil),
		NewPreparedTransfer("fixture-default-log-prepared", "flow", "state", "fixture-default-log-source", "fixture-default-log-dest", Money{}, "fixture-default-log-purpose"),
		NewTransferResult("fixture-default-log-result", "flow", "state", &document),
	}
	for _, value := range values {
		for _, format := range []string{"%v", "%p", "%w", "%#w", "%J"} {
			buf.Reset()
			log.Printf(format, value)
			if strings.Contains(buf.String(), "fixture-default-log-") {
				t.Fatal("default logger disclosed raw storage", buf.String())
			}
		}
	}
}

func TestOpaqueConcurrentGetterCopies(t *testing.T) {
	resource := NewTransferResource("source", "card", "name", "RUB")
	sources := []TransferResource{resource}
	document := "document"
	draft := NewTransferDraft("pid", "flow", "state", sources, sources)
	result := NewTransferResult("pid", "flow", "state", &document)
	d, _ := ParseDecimal("9007199254740993.0100")
	money := Money{Amount: d, Currency: "RUB"}
	prepared := NewPreparedTransfer("pid", "flow", "state", "source", "destination", money, "purpose")
	for i := 0; i < 16; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			for j := 0; j < 20; j++ {
				list, destinations := draft.Sources(), draft.Destinations()
				id, amount := result.DocumentID(), prepared.Amount()
				if list[0].ID() != "source" || destinations[0].ID() != "source" || id == nil || *id != "document" || amount != (Money{Amount: d, Currency: "RUB"}) {
					t.Fatal("concurrent getters lost immutable content")
				}
				list[0], destinations[0] = TransferResource{}, TransferResource{}
				*id = "changed-reader"
				amount.Amount, amount.Currency = Decimal{}, "changed-reader"
			}
		})
	}
	// Deferred parallel readers see retained snapshots, not the mutated inputs.
	sources[0], document, money = TransferResource{}, "changed-caller", Money{}
}
