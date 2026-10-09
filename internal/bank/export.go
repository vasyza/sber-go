// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

import (
	"bytes"
	"encoding"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/vasyza/sber-sdk/internal/strictjson"
)

// JSONable converts native models/containers to the source JSON shape. Decimal
// and Money amounts remain exact strings; typed financial identifiers and codes
// retain literal text, while display strings are PAN-redacted.
// Custom marshalers (including credential-bearing SDK values) retain their
// own default redaction contract rather than being bypassed via reflection.
// Their output must be one full JSON document with valid Unicode and unique
// decoded object keys; numeric lexemes are preserved, not parsed as float64.
// A failing finite streaming float retains the established deferred encoder-
// error contract as a zero-state static failure marker, never its raw receiver.
func JSONable(value any) (any, error) {
	return domainJSONValue(reflect.ValueOf(value), map[domainJSONVisit]bool{})
}

type domainJSONVisit struct {
	typ    reflect.Type
	ptr    uintptr
	length int // Only slices: a shared start address is not a shared view.
}

func domainJSONAddress(v reflect.Value) reflect.Value {
	if v.CanAddr() {
		return v.Addr()
	}
	return reflect.Value{}
}

// A failed finite streaming scalar used to reach ExportJSON's final encoder.
// Retain that narrow JSONable contract with no source value or private cause.
// Successful normalization never returns the original named primitive.
type domainJSONDeferredError struct{}

func (domainJSONDeferredError) MarshalJSON() ([]byte, error) {
	return nil, NewParseError("json_export")
}

// Per-call streaming provenance contains only already-normalized plain data,
// not a raw receiver, credential implementation or arbitrary source pointer.
// It is consumed by the walker; JSONable never returns this private carrier.
type domainJSONNormalized struct{ value any }

// Use the package entry point, not a direct MarshalJSONTo call: Go handles
// ErrUnsupported fallback, rejects mutation before fallback and enforces a
// singular value. Strict syntax options prevent replacement/duplicate loss
// before our complete-document check sees the output.
func domainJSONStreaming(value any, visits map[domainJSONVisit]bool) (any, error) {
	// Record native financial values only when the package actually marshals
	// those types/schemas (including a streamer's documented native fallback). Raw
	// custom JSON strings do not gain this exemption from display PAN masking.
	financial := map[jsontext.Pointer]any{}
	marshalers := jsonv2.JoinMarshalers(
		jsonv2.MarshalToFunc(func(e *jsontext.Encoder, m Money) error {
			wire := struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			}{m.Amount.String(), m.Currency}
			if err := jsonv2.MarshalEncode(e, wire); err != nil {
				return err
			}
			financial[e.StackPointer()] = m
			return nil
		}),
		jsonv2.MarshalToFunc(func(e *jsontext.Encoder, d Decimal) error {
			if err := e.WriteToken(jsontext.String(d.String())); err != nil {
				return err
			}
			financial[e.StackPointer()] = d
			return nil
		}),
		jsonv2.MarshalToFunc(func(e *jsontext.Encoder, value any) error {
			// Interface hooks receive a nonnil pointer to the concrete value.
			// Inspect only its exact type, without walking foreign fields. The
			// official encoder retains custom-method/fallback precedence for
			// every unadmitted type; return Unsupported without writing a token.
			value = reflect.ValueOf(value).Elem().Interface()
			v := reflect.ValueOf(value)
			if v.Kind() == reflect.Pointer && v.IsNil() {
				return errors.ErrUnsupported
			}
			if snapshot, owned := domainJSONEntitySnapshot(value); owned {
				value = snapshot
			} else if !domainJSONIsNativeFinancial(value) {
				return errors.ErrUnsupported
			}
			out, err := domainJSONValue(reflect.ValueOf(value), visits)
			if err != nil {
				return err
			}
			if err := jsonv2.MarshalEncode(e, out); err != nil {
				return err
			}
			financial[e.StackPointer()] = domainJSONNormalized{out}
			return nil
		}),
	)
	data, err := jsonv2.Marshal(value, json.DefaultOptionsV1(),
		jsontext.AllowInvalidUTF8(false), jsontext.AllowDuplicateNames(false),
		jsonv2.WithMarshalers(marshalers))
	if err != nil || strictjson.Validate(data) != nil {
		return nil, NewParseError("json_export")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, NewParseError("json_export")
	}
	out = domainJSONRestoreFinancial(out, "", financial)
	return domainJSONValue(reflect.ValueOf(out), visits)
}

func domainJSONRestoreFinancial(value any, path jsontext.Pointer, financial map[jsontext.Pointer]any) any {
	if original, ok := financial[path]; ok {
		return original
	}
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = domainJSONRestoreFinancial(item, path.AppendToken(key), financial)
		}
	case []any:
		for i, item := range v {
			v[i] = domainJSONRestoreFinancial(item, path.AppendToken(strconv.Itoa(i)), financial)
		}
	}
	return value
}

func domainJSONMoney(m Money) (any, error) {
	if !utf8.ValidString(m.Currency) {
		return nil, NewParseError("json_export")
	}
	// Currency is source literal financial metadata, not display prose.
	return map[string]any{"amount": m.Amount.String(), "currency": m.Currency}, nil
}

// Resolve authoritative text methods in an actual object-name context, not a
// value context (where JSON methods have different priority). Only an inert map
// value is encoded; custom key implementation fields are never pre-walked.
// Keep source-compatible conversion for comparable keys without text methods.
func domainJSONMapKey(v reflect.Value) (string, error) {
	for v.Kind() == reflect.Interface {
		if v.IsNil() {
			// A nil interface has no key-facing method to call. Retain the
			// source's nil-to-empty name, even for a text-interface map type.
			return "", nil
		}
		v = v.Elem()
	}
	if v.Type().Implements(reflect.TypeFor[encoding.TextAppender]()) || v.Type().Implements(reflect.TypeFor[encoding.TextMarshaler]()) {
		keyOnly := reflect.MakeMapWithSize(reflect.MapOf(v.Type(), reflect.TypeFor[bool]()), 1)
		keyOnly.SetMapIndex(v, reflect.ValueOf(true))
		data, err := jsonv2.Marshal(keyOnly.Interface(), json.DefaultOptionsV1(),
			jsontext.AllowInvalidUTF8(false), jsontext.AllowDuplicateNames(false))
		if err != nil || strictjson.Validate(data) != nil {
			return "", NewParseError("json_export")
		}
		var names map[string]json.RawMessage
		if json.Unmarshal(data, &names) != nil || len(names) != 1 {
			return "", NewParseError("json_export")
		}
		for name := range names {
			return RedactPAN(name), nil
		}
	}
	if !domainJSONNativeKeyValid(v, map[domainJSONVisit]bool{}) {
		return "", NewParseError("json_export")
	}
	key := domainID(v.Interface())
	if !utf8.ValidString(key) {
		return "", NewParseError("json_export")
	}
	return RedactPAN(key), nil
}

// Check native string identity before fmt-based source-compatible map-key
// conversion can repair malformed text. Exported composite key fields are part
// of that identity; private implementation fields stay behind custom boundaries.
func domainJSONNativeKeyValid(v reflect.Value, visits map[domainJSONVisit]bool) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.String:
		return utf8.ValidString(v.String())
	case reflect.Interface:
		return v.IsNil() || domainJSONNativeKeyValid(v.Elem(), visits)
	case reflect.Pointer:
		if v.IsNil() {
			return true
		}
		visit := domainJSONVisit{typ: v.Type(), ptr: v.Pointer()}
		if visits[visit] {
			return false
		}
		visits[visit] = true
		defer delete(visits, visit)
		return domainJSONNativeKeyValid(v.Elem(), visits)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !domainJSONNativeKeyValid(v.Index(i), visits) {
				return false
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath == "" && !domainJSONNativeKeyValid(v.Field(i), visits) {
				return false
			}
		}
	}
	return true
}

// Literal roles are trusted only on these exact native financial schemas.
// Foreign layouts (even embedded snapshots or identical tags) do not acquire
// this provenance, and untyped custom JSON never passes through this boundary.
func domainJSONIsNativeFinancial(value any) bool {
	switch value.(type) {
	case Account, Card, Resource, Operation, CardLedgerEntry, OperationDetail, CardInfo, CategoryAmount:
		return true
	}
	return false
}

// The annotated native schema uses only string, optional string and string-list
// identity roles. Normalize into plain values, checking original text before
// the encoder can replace bytes. Preserve null/empty distinction and ordering.
func domainJSONLiteral(value any) (any, error) {
	switch v := value.(type) {
	case string:
		if utf8.ValidString(v) {
			return v, nil
		}
	case *string:
		if v == nil {
			return nil, nil
		}
		return domainJSONLiteral(*v)
	case []string:
		if v == nil {
			return nil, nil
		}
		out := make([]any, len(v))
		for i, item := range v {
			x, err := domainJSONLiteral(item)
			if err != nil {
				return nil, err
			}
			out[i] = x
		}
		return out, nil
	}
	return nil, NewParseError("json_export")
}

// Match the selected V1 encoder's omitempty rules, which are based on native
// kind/length rather than IsZero methods or a container's nilness alone.
func domainJSONEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Interface, reflect.Pointer:
		return v.IsZero()
	}
	return false
}

func domainJSONValue(v reflect.Value, visits map[domainJSONVisit]bool) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, nil
		}
		return domainJSONValue(v.Elem(), visits)
	}
	if (v.Kind() == reflect.Pointer || v.Kind() == reflect.Map || v.Kind() == reflect.Slice) && v.IsNil() {
		return nil, nil
	}
	if v.CanInterface() {
		switch x := v.Interface().(type) {
		case domainJSONNormalized:
			return x.value, nil
		case Decimal:
			return x.String(), nil
		case *Decimal:
			return x.String(), nil
		case *Money:
			return domainJSONMoney(*x)
		case Money:
			return domainJSONMoney(x)
		case json.Number:
			if !domainJSONNumberPattern.MatchString(string(x)) {
				return nil, NewParseError("json_export")
			}
			return x, nil
		}
		if snapshot, owned := domainJSONEntitySnapshot(v.Interface()); owned {
			return domainJSONValue(reflect.ValueOf(snapshot), visits)
		}
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Map || v.Kind() == reflect.Slice {
		visit := domainJSONVisit{typ: v.Type(), ptr: v.Pointer()}
		if v.Kind() == reflect.Slice {
			visit.length = v.Len()
		}
		if visits[visit] {
			return nil, NewParseError("json_export")
		}
		visits[visit] = true
		defer delete(visits, visit)
	}
	// Money pointers must not take the general custom-serializer path: financial
	// amounts that happen to pass Luhn are not card numbers.
	// Check both method sets on the original addressable value. Streaming JSON
	// has precedence over legacy JSON even when the legacy method is on T and
	// the streaming redactor is only on *T. Both outrank text appending/marshaling.
	var custom any
	for _, candidate := range []reflect.Value{domainJSONAddress(v), v} {
		if !candidate.IsValid() || !candidate.CanInterface() {
			continue
		}
		x := candidate.Interface()
		if _, ok := x.(jsonv2.MarshalerTo); ok {
			custom = x
			break
		}
		if _, ok := x.(json.Marshaler); ok {
			custom = x
			continue
		}
		if _, legacy := custom.(json.Marshaler); legacy {
			continue
		}
		if _, ok := x.(encoding.TextAppender); ok {
			custom = x
			continue
		}
		if _, appender := custom.(encoding.TextAppender); appender {
			continue
		}
		if _, ok := x.(encoding.TextMarshaler); ok {
			custom = x
		}
	}
	if custom != nil {
		if _, streaming := custom.(jsonv2.MarshalerTo); streaming {
			out, err := domainJSONStreaming(custom, visits)
			if err != nil {
				// Preserve JSONable's established deferred encoder-error behavior
				// for finite floating scalars, without retaining the receiver or its
				// private error. The only later method is a static failure marker.
				if (v.Kind() == reflect.Float32 || v.Kind() == reflect.Float64) && !math.IsNaN(v.Float()) && !math.IsInf(v.Float(), 0) {
					return domainJSONDeferredError{}, nil
				}
				return nil, NewParseError("json_export")
			}
			return out, nil
		}
		legacy, ok := custom.(json.Marshaler)
		if !ok {
			// Use the official text-interface precedence/UTF-8 checks too.
			// Text failures do not acquire the streaming-float-only deferred
			// compatibility marker or expose private implementation fields.
			return domainJSONStreaming(custom, visits)
		}
		data, err := legacy.MarshalJSON()
		if err != nil || strictjson.Validate(data) != nil {
			return nil, NewParseError("json_export")
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		var out any
		if err = dec.Decode(&out); err != nil {
			return nil, NewParseError("json_export")
		}
		return domainJSONValue(reflect.ValueOf(out), visits)
	}
	if v.Kind() == reflect.Pointer {
		return domainJSONValue(v.Elem(), visits)
	}
	switch v.Kind() {
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return nil, NewParseError("json_export")
		}
		return RedactPAN(v.String()), nil
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint(), nil
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
			return nil, NewParseError("json_export")
		}
		if v.Kind() == reflect.Float32 {
			return float32(v.Float()), nil
		}
		return v.Float(), nil
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			x, err := domainJSONValue(v.Index(i), visits)
			if err != nil {
				return nil, err
			}
			out[i] = x
		}
		return out, nil
	case reflect.Map:
		out := map[string]any{}
		iter := v.MapRange()
		for iter.Next() {
			key, err := domainJSONMapKey(iter.Key())
			if err != nil {
				return nil, err
			}
			x, err := domainJSONValue(iter.Value(), visits)
			if err != nil {
				return nil, err
			}
			if _, exists := out[key]; exists {
				return nil, NewParseError("json_export")
			}
			out[key] = x
		}
		return out, nil
	case reflect.Struct:
		out := map[string]any{}
		typ := v.Type()
		nativeFinancial := v.CanInterface() && domainJSONIsNativeFinancial(v.Interface())
		for i := 0; i < v.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath != "" {
				continue
			}
			// StructTag.Get unquotes with strconv and can replace invalid raw
			// UTF-8 before the decoded field name is available for validation.
			if !utf8.ValidString(string(field.Tag)) {
				return nil, NewParseError("json_export")
			}
			tag := field.Tag.Get("json")
			name, opts, _ := strings.Cut(tag, ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			if slices.Contains(strings.Split(opts, ","), "omitempty") && domainJSONEmptyValue(v.Field(i)) {
				continue
			}
			if !utf8.ValidString(name) {
				return nil, NewParseError("json_export")
			}
			// Property names are display text regardless of the field's native
			// financial value role. Check collisions after the same masking
			// applied to map names and authoritative custom JSON output.
			name = RedactPAN(name)
			if _, exists := out[name]; exists {
				return nil, NewParseError("json_export")
			}
			var x any
			var err error
			if nativeFinancial && field.Tag.Get("sber") == "literal" {
				x, err = domainJSONLiteral(v.Field(i).Interface())
			} else {
				x, err = domainJSONValue(v.Field(i), visits)
			}
			if err != nil {
				return nil, err
			}
			out[name] = x
		}
		return out, nil
	default:
		return nil, NewParseError("json_export")
	}
}

// ExportJSON is an explicit financial-data export, not a credential dump.
func ExportJSON(value any) ([]byte, error) {
	out, err := JSONable(value)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(out); err != nil {
		// Encoder diagnostics can include raw tokens or custom-marshaler errors.
		return nil, NewParseError("json_export")
	}
	if strictjson.Validate(buf.Bytes()) != nil {
		return nil, NewParseError("json_export")
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// WritePrivateJSON atomically replaces the exact target with a mode-0600 file.
// It replaces a target symlink itself, never writing through it, and removes
// unpublished temporary files on every failure. The parent directory must exist
// and be trusted (not attacker-replaceable). This financial exporter is not the
// enrollment credential writer and makes no no-replace publication guarantee.
func WritePrivateJSON(path string, value any) error {
	data, err := ExportJSON(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
