package mcptools

import (
	"bytes"
	"sync"
	"testing"
)

func TestCatalogDefensiveCopiesConcurrentUse(t *testing.T) {
	baseline := Catalog(false)
	want := append([]byte(nil), baseline[0].InputSchema...)
	var wait sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			definitions := Catalog(true)
			definitions[0].Name = "caller-modified"
			definitions[0].InputSchema[0] = '!'
			fresh := Catalog(false)
			if fresh[0].Name != "sber_setup_status" || !bytes.Equal(fresh[0].InputSchema, want) {
				t.Error("caller mutation leaked into later catalog")
			}
			if err := ValidateArguments("sber_operations", []byte(`{"limit":10e-1}`), false); err != nil {
				t.Error("concurrent validation failed")
			}
		}()
	}
	wait.Wait()
	if !bytes.Equal(baseline[0].InputSchema, want) {
		t.Fatal("prior snapshot changed")
	}
}
