package sber

import("testing";"strings";"encoding/json")
// Synthetic financial-export seam probe; parent owns the generic model walker.
func TestProbeBoundFinancialExport(t *testing.T){
 p:=NewBankPortfolio(entityFixtureProducts(t),&resourceScript{t:t});card:=p.Cards()[0]
 direct,_:=json.Marshal(card);snapshot,_:=ExportJSON(card.Snapshot());bound,_:=ExportJSON(card)
 t.Logf("marshal=%s snapshot=%s bound=%s",direct,snapshot,bound)
 if !strings.Contains(string(direct),"4111111111111111.50")||!strings.Contains(string(snapshot),"4111111111111111.50"){t.Fatal("raw financial snapshot broken")}
 if !strings.Contains(string(bound),"4111111111111111.50"){t.Fatal("generic ExportJSON on bound custom marshaler reparses Money as display text")}
}
