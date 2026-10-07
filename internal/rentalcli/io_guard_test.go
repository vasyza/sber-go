package rentalcli

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/vasyza/sber-go/rental"
)

func TestPreviewRequiredContractTimesKeepNativeMissingAndZeroErrors(t *testing.T) {
	for position := range 6 {
		for _, omit := range []bool{false, true} {
			document := syntheticProofDocument(t)
			field := timestampPositions(document)[position]
			name := field.name + "/zero"
			if omit {
				name = field.name + "/omitted"
			}
			t.Run(name, func(t *testing.T) {
				if omit {
					delete(field.object, field.key)
				} else {
					field.object[field.key] = "0001-01-01T00:00:00Z"
				}
				data := documentBytes(t, document)
				before := bytes.Clone(data)
				var out, diagnostics bytes.Buffer
				if code := Run(bytes.NewReader(data), &out, &diagnostics); code != 4 || out.Len() != 0 || diagnostics.String() != "invalid rental ledger\n" {
					t.Fatalf("required contract time changed native zero policy: exit=%d stderr=%q", code, diagnostics.String())
				}
				if !bytes.Equal(data, before) {
					t.Fatal("failed preview modified input")
				}
			})
		}
	}
}

type countingInput struct {
	reader io.Reader
	read   int
}

func (r *countingInput) Read(data []byte) (int, error) {
	n, err := r.reader.Read(data)
	r.read += n
	return n, err
}

func TestPreviewBoundedReadAndExactSizeBoundary(t *testing.T) {
	data := documentBytes(t, syntheticProofDocument(t))
	for _, extra := range []int{0, 1, MaximumInputBytes} {
		t.Run("bytes-"+strconv.Itoa(MaximumInputBytes+extra), func(t *testing.T) {
			padded := append(bytes.Clone(data), bytes.Repeat([]byte(" "), MaximumInputBytes-len(data)+extra)...)
			reader := &countingInput{reader: bytes.NewReader(padded)}
			var out, diagnostics bytes.Buffer
			code := Run(reader, &out, &diagnostics)
			if extra == 0 {
				if code != 0 || diagnostics.Len() != 0 || reader.read != MaximumInputBytes {
					t.Fatalf("exact-size valid document rejected: exit=%d bytes=%d", code, reader.read)
				}
				assertDecision(t, decodeDecisions(t, out.Bytes()), rental.Paid, 0, 1)
			} else if code != 3 || out.Len() != 0 || diagnostics.String() != "invalid preview JSON\n" || reader.read != MaximumInputBytes+1 {
				t.Fatalf("size/read bound changed: exit=%d bytes=%d stderr=%q", code, reader.read, diagnostics.String())
			}
		})
	}
}

type failingInput struct {
	data []byte
}

func (r failingInput) Read(out []byte) (int, error) {
	return copy(out, r.data), errors.New("SYNTHETIC-private-cause")
}

func TestPreviewIOFailuresRemainStaticAndDecisionFree(t *testing.T) {
	data := documentBytes(t, syntheticProofDocument(t))
	var out, diagnostics bytes.Buffer
	if code := Run(failingInput{data}, &out, &diagnostics); code != 3 || out.Len() != 0 || diagnostics.String() != "invalid preview JSON\n" {
		t.Fatalf("partial read failure became success or disclosed cause: %d %q", code, diagnostics.String())
	}
	out.Reset()
	diagnostics.Reset()
	if code := Run(nil, &out, &diagnostics); code != 2 || out.Len() != 0 || diagnostics.String() != "invalid preview I/O\n" {
		t.Fatalf("nil input did not fail closed: %d", code)
	}
	diagnostics.Reset()
	if code := Run(bytes.NewReader(data), nil, &diagnostics); code != 2 || diagnostics.String() != "invalid preview I/O\n" {
		t.Fatalf("nil output did not fail closed: %d", code)
	}
	if code := Run(strings.NewReader(`{"AsOf":null}`), &out, nil); code != 3 || out.Len() != 0 {
		t.Fatal("nil diagnostics writer changed schema rejection")
	}
}

func TestPreviewNilCollectionsKeepNativeEngineSemantics(t *testing.T) {
	for _, collection := range []string{"Tenants", "Periods", "Receipts", "Evidence"} {
		t.Run(collection, func(t *testing.T) {
			document := syntheticProofDocument(t)
			document[collection] = nil
			if collection == "Tenants" {
				var out, diagnostics bytes.Buffer
				if code := Run(bytes.NewReader(documentBytes(t, document)), &out, &diagnostics); code != 4 || out.Len() != 0 || diagnostics.String() != "invalid rental ledger\n" {
					t.Fatal("nil required tenants became a ledger")
				}
				return
			}
			result := previewDocument(t, document)
			switch collection {
			case "Periods":
				if len(result.Periods) != 0 || len(result.Candidates) != 0 {
					t.Fatal("nil periods invented obligations")
				}
			case "Receipts":
				assertDecision(t, result, rental.Due, 1, 0)
			case "Evidence":
				assertDecision(t, result, rental.Paid, 0, 1)
			}
		})
	}
}
