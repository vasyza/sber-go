package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestReviewDeviceprintExplicitEmptyPreservesZeroValue(t *testing.T) {
	device, err := GenerateAntifraudDeviceprint("")
	if err != nil || device != (Deviceprint{}) || device.Value() != "" {
		t.Fatal("explicit empty deviceprint no longer equals the readable zero value")
	}
}

func TestReviewDeviceprintDiagnosticRedaction(t *testing.T) {
	encoded, err := GenerateAntifraudDeviceprint("synthetic-device-secret-guard")
	if err != nil || encoded.Value() != "synthetic-device-secret-guard" {
		t.Fatal("synthetic explicit deviceprint access failed")
	}
	generated, err := generateDeviceprint(bytes.NewReader(make([]byte, 64)))
	if err != nil || generated.Value() == "" {
		t.Fatal("synthetic deviceprint generation failed")
	}
	for _, device := range []struct {
		name   string
		value  Deviceprint
		marker string
	}{
		{"encoded", encoded, "synthetic-device-secret-guard"},
		{"generated", generated, "version=5.3.0"},
	} {
		cloned := device.value
		if cloned.Value() != device.value.Value() || (Deviceprint{}).Value() != "" {
			t.Fatal("copy or zero-value explicit deviceprint access changed")
		}
		for _, shape := range []struct {
			name  string
			value any
		}{
			{"value", device.value},
			{"pointer", &cloned},
			{"slice", []Deviceprint{device.value}},
			{"map", map[string]Deviceprint{"device": device.value}},
			{"nested", struct{ Device any }{device.value}},
			{"reflect_value", reflect.ValueOf(device.value)},
		} {
			for _, verb := range "vTtbcdoOxXUeEfFgGsqpjw" {
				for _, flags := range []string{"", "+", "#", "020.3"} {
					format := "device=%" + flags + string(verb)
					t.Run(device.name+"/"+shape.name+"/"+flags+string(verb), func(t *testing.T) {
						var buf bytes.Buffer
						logger := log.New(&buf, "", 0)
						logger.Printf(format, shape.value)
						for _, text := range []string{fmt.Sprintf(format, shape.value), fmt.Errorf(format, shape.value).Error(), buf.String()} {
							if strings.Contains(text, device.marker) || strings.Contains(text, device.value.Value()) || strings.Contains(text, device.marker[:3]) {
								t.Fatal("unsupported fmt/error/log diagnostic exposed the deviceprint")
							}
						}
					})
				}
			}
		}
	}
}

func TestParity_CallableSberUnofficialDeviceprintGenerateDeviceprintL26(t *testing.T) {
	dp, err := GenerateDeviceprint()
	if err != nil {
		t.Fatal(err)
	}
	source := dp.Value()
	fixed := "version=5.3.0&os=Windows&osVersion=10.0&browser=Chrome&browserVersion=146.0.0.0&platform=Win32&screen=1920x1080&colorDepth=24&timezone=-180&language=ru-RU&cpuCores=8&"
	if !strings.HasPrefix(source, fixed) {
		t.Fatal("audited default browser layout differs")
	}
	suffix := strings.TrimPrefix(source, fixed)
	if !regexp.MustCompile(`^canvas=[a-f0-9]{32}&webgl=[a-f0-9]{32}&fonts=[a-f0-9]{16}&audio=[a-f0-9]{16}&uuid=[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`).MatchString(suffix) {
		t.Fatal("fingerprint entropy or UUID shape differs")
	}
	second, err := GenerateDeviceprint()
	if err != nil || second.Value() == source {
		t.Fatal("new device not independently generated")
	}
}
func TestParity_CallableSberUnofficialDeviceprintGenerateAntifraudDeviceprintL49(t *testing.T) {
	dp, err := GenerateDeviceprint()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := GenerateAntifraudDeviceprint(dp.Value())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encoded.Value(), "&") || !strings.Contains(encoded.Value(), "%26") {
		t.Fatal("raw separators not encoded")
	}
	decoded, err := url.PathUnescape(encoded.Value())
	if err != nil || decoded != dp.Value() {
		t.Fatal("antifraud encoding changes device")
	}
	fresh, err := GenerateAntifraudDeviceprint()
	if err != nil || fresh.Value() == encoded.Value() {
		t.Fatal("omitted input did not mint independent device")
	}
	// Arbitrary captured array/object/hash extras are encoded intact, not parsed,
	// normalized or replaced with a narrower synthetic shape.
	extra := `array=["рус",1]&object={"x":true}&hash=a/b c+!~_.-`
	encoded, err = GenerateAntifraudDeviceprint(extra)
	if err != nil {
		t.Fatal(err)
	}
	want := `array%3D%5B%22%D1%80%D1%83%D1%81%22%2C1%5D%26object%3D%7B%22x%22%3Atrue%7D%26hash%3Da%2Fb%20c%2B%21~_.-`
	if encoded.Value() != want {
		t.Fatal("Python quote(safe='') compatibility differs")
	}
	empty, err := GenerateAntifraudDeviceprint("")
	if err != nil || empty.Value() != "" {
		t.Fatal("explicit empty source is not the same as omitted source")
	}
	if _, err := GenerateAntifraudDeviceprint("one", "two"); err == nil {
		t.Fatal("multiple sources accepted")
	}
}
func TestDeviceprintEntropyFailureAndRedaction(t *testing.T) {
	dp, err := generateDeviceprint(bytes.NewReader(make([]byte, 64)))
	if err != nil || !strings.HasSuffix(dp.Value(), "uuid=00000000-0000-4000-8000-000000000000") {
		t.Fatal("synthetic entropy tracer differs")
	}
	for _, reader := range []io.Reader{bytes.NewReader(nil), bytes.NewReader(make([]byte, 63)), failingDeviceEntropy{}} {
		got, err := generateDeviceprint(reader)
		if err == nil || got.Value() != "" || errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "secret-entropy") {
			t.Fatal("entropy failure emitted partial fingerprint or sensitive cause")
		}
	}
	raw, err := json.Marshal(dp)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{string(raw), fmt.Sprintf("%v %+v %#v %s %q", dp, dp, dp, dp, dp)} {
		if strings.Contains(text, "version=") || strings.Contains(text, "00000000-0000") {
			t.Fatal("fingerprint leaked through formatting or JSON")
		}
	}
}

type failingDeviceEntropy struct{}

func (failingDeviceEntropy) Read([]byte) (int, error) { return 0, errors.New("secret-entropy") }
