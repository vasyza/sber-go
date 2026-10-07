package sber

import (
	"bytes"
	"fmt"
	"log"
	"reflect"
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
		copy := device.value
		if copy.Value() != device.value.Value() || (Deviceprint{}).Value() != "" {
			t.Fatal("copy or zero-value explicit deviceprint access changed")
		}
		for _, shape := range []struct {
			name  string
			value any
		}{
			{"value", device.value},
			{"pointer", &copy},
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
