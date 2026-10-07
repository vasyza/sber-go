package sber

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestClientBindingCycleDoesNotExpandFormattingOrExportPrivateState(t *testing.T) {
	c, _ := clientBindingSyntheticClient(t, resourcePortfolioResponse())
	p := clientBindingPortfolio(t, c, false)
	values := []any{c.Products(), *c.Products(), c.Operations(), *c.Operations(), c.Accounts(), *c.Accounts(), c.Cards(), *c.Cards(), c.Transfers(), *c.Transfers(), c.Analytics(), *c.Analytics(), c.Session(), *c.Session(), p, *p, p.Cards()[0], *p.Cards()[0], p.Accounts()[0], *p.Accounts()[0]}
	for _, value := range values {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%p", "%w", "%t"} {
			out := fmt.Sprintf(verb, value)
			if len(out) > 8192 || strings.Contains(out, "session-binding-access") || strings.Contains(out, "token-binding-access") || strings.Contains(out, "version=1.7.3") {
				t.Fatalf("bound graph formatting traversed private requester state: type=%T verb=%s length=%d session=%v token=%v deviceprint=%v", value, verb, len(out), strings.Contains(out, "session-binding-access"), strings.Contains(out, "token-binding-access"), strings.Contains(out, "version=1.7.3"))
			}
			if strings.Contains(fmt.Errorf("diagnostic "+verb, value).Error(), "token-binding-access") {
				t.Fatal("error formatting leaked bound client")
			}
		}
		raw, err := json.Marshal(value)
		if err != nil || strings.Contains(string(raw), "session-binding-access") || strings.Contains(string(raw), "token-binding-access") {
			t.Fatal("bound graph JSON exported private requester")
		}
	}
	if _, err := c.Products().Get(context.Background(), false); err != nil {
		t.Fatal(err)
	}
}
