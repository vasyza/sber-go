package mcpwire

import (

    "strings"
    "testing"
)

func TestProtocolIDsRejectNonIntegersAndBoundCorrelation(t *testing.T) {
    server, err := New(Options{})
    if err != nil { t.Fatal(err) }
    for _, id := range []string{`null`,`true`,`[]`,`{}`,`1.1`,`1e3`,`-0.0`, strings.Repeat("9",257), strconvQuote(strings.Repeat("x",257))} {
        t.Run(id[:min(len(id),20)],func(t *testing.T){
            responses:=serveText(t,server,`{"jsonrpc":"2.0","id":`+id+`,"method":"server/discover","params":{`+currentMeta+`}}`+"\n")
            if len(responses)!=1 {t.Fatalf("want one error; got %d",len(responses))}
            requireCode(t,responses[0],-32600)
            if len(responses[0]["id"])!=0 && string(responses[0]["id"])!="null" {t.Fatal("invalid ID must not be echoed")}
        })
    }
}

func TestProtocolIDsPreserveExactOriginalSpelling(t *testing.T) {
    server,err:=New(Options{});if err!=nil{t.Fatal(err)}
    for _, id:=range []string{`9007199254740993`,`-0`,`"\u0061"`,`"🚀"`,`""`,`"<>&"`,"\"line\u2028separator\"",strings.Repeat("9",256)} {
        responses:=serveText(t,server,`{"jsonrpc":"2.0","id":`+id+`,"method":"server/discover","params":{`+currentMeta+`}}`+"\n")
        if len(responses)!=1 || string(responses[0]["id"])!=id {t.Fatalf("want exact ID %s; got %v",id,responses)}
        if _,exists:=resultFields(t,responses[0])["resultType"];!exists{t.Fatal("want result")}
    }

}
