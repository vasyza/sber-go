package mcpwire

import (
    "bytes"
    "context"
    "encoding/json"
    "testing"
)

// All protocol fixtures and handlers in this package are synthetic, not live.
func TestPerRequestMetadataNeverFallsBackOrReflectsPrivateVersion(t *testing.T) {
    var calls int
    tool:=syntheticTool("local")
    tool.Handle=func(context.Context,json.RawMessage)(ToolResult,error){calls++;return ToolResult{},nil}
    server,err:=New(Options{Tools:[]Tool{tool}});if err!=nil{t.Fatal(err)}
    cases:=[]struct{name,meta string;code int}{
        {"unsupported-date",`"_meta":{"io.modelcontextprotocol/protocolVersion":"1900-01-01","io.modelcontextprotocol/clientCapabilities":{}}`,-32022},
        {"private-version",`"_meta":{"io.modelcontextprotocol/protocolVersion":"SYNTHETIC-PRIVATE-MARKER","io.modelcontextprotocol/clientCapabilities":{}}`,-32022},
        {"missing-version",`"_meta":{"io.modelcontextprotocol/clientCapabilities":{}}`,-32602},
        {"missing-capabilities",`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}`,-32602},
        {"null-capabilities",`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":null}`,-32602},
        {"legacy-in-modern-meta",`"_meta":{"io.modelcontextprotocol/protocolVersion":"2025-11-25","io.modelcontextprotocol/clientCapabilities":{}}`,-32022},
    }
    for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
        input:=legacyInitialize+legacyInitialized+`{"jsonrpc":"2.0","id":"inline","method":"tools/call","params":{"name":"local","arguments":{},`+tc.meta+`}}`+"\n"
        responses:=serveText(t,server,input)
        if len(responses)!=2{t.Fatalf("want initialize and rejection; got %d",len(responses))}
        requireCode(t,responses[1],tc.code)
        if bytes.Contains(responses[1]["error"],[]byte("SYNTHETIC-PRIVATE-MARKER")){t.Fatal("private version reflected")}
    })}
    if calls!=0{t.Fatalf("want no dispatch; got %d",calls)}
    // Escaped spelling is decoded semantically, not compared as a raw literal.
    responses:=serveText(t,server,`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-2\u0038","io.modelcontextprotocol/clientCapabilities":{}}}}`+"\n")
    if string(resultFields(t,responses[0])["resultType"])!=`"complete"`{t.Fatal("want decoded metadata")}
}

func TestInvalidNotificationsNeverReplyOrInvokeHandlers(t *testing.T) {
    var calls int
    tool:=syntheticTool("local")
    tool.Handle=func(context.Context,json.RawMessage)(ToolResult,error){calls++;return ToolResult{},nil}
    server,err:=New(Options{Tools:[]Tool{tool}});if err!=nil{t.Fatal(err)}
    notifications:=[]string{
        `{"jsonrpc":"1.0","method":"tools/call","params":{`+currentMeta+`,"name":"local"}}`,
        `{"jsonrpc":"2.0","method":"tools/call","params":[]}`,
        `{"jsonrpc":"2.0","method":42}`,
        `{"jsonrpc":"2.0","method":"tools/call","method":"server/discover"}`,
        `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"local","arguments":{"x":1,"\u0078":2},`+currentMeta+`}}`,
        `{"jsonrpc":"2.0","METHOD":"tools/call","params":{`+currentMeta+`,"name":"local"}}`,
        `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":null}}`,
    }
    for _,frame:=range notifications{
        replies:=serveText(t,server,frame+"\n")
        if len(replies)!=0{t.Fatalf("want silent invalid notification; got %d",len(replies))}
    }
    if calls!=0{t.Fatal("invalid notification invoked a handler")}
}

func TestMethodParameterCaseAndUnsupportedFieldsBlockDispatch(t *testing.T) {
    var calls int;tool:=syntheticTool("local")
    tool.Handle=func(context.Context,json.RawMessage)(ToolResult,error){calls++;return ToolResult{},nil}
    mutation:=syntheticTool("mutate");mutation.ReadOnly=false;mutation.Handle=tool.Handle
    server,err:=New(Options{Tools:[]Tool{tool,mutation}});if err!=nil{t.Fatal(err)}
    for _,params:=range []string{
        `"name":"local","arguments":{},"Name":"private",`+currentMeta,
        `"name":"local","arguments":{},"inputResponses":{},`+currentMeta,
        `"name":"mutate","arguments":{},`+currentMeta,
        `"name":"local","arguments":{},"_meta":{"io.modelcontextprotocol/ProtocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`,
    }{
        replies:=serveText(t,server,legacyInitialize+legacyInitialized+`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{`+params+`}}`+"\n")
        requireCode(t,replies[1],-32602)
    }
    if calls!=0{t.Fatal("forged or invalid call reached handler")}
}

func TestLegacyPingAndUnknownMethodsHaveExplicitRoutes(t *testing.T) {
    server,err:=New(Options{});if err!=nil{t.Fatal(err)}
    for _,prefix:=range []string{"",legacyInitialize+legacyInitialized}{
        replies:=serveText(t,server,prefix+`{"jsonrpc":"2.0","id":"unknown","method":"arbitrary"}`+"\n")
        requireCode(t,replies[len(replies)-1],-32601)
    }
    replies:=serveText(t,server,`{"jsonrpc":"2.0","id":"ping","method":"ping"}`+"\n")
    if string(replies[0]["result"])!="{}"{t.Fatal("want legacy empty ping result")}
}

func TestDiscoveryListAndIdentityParametersAreValidated(t *testing.T) {
    server,err:=New(Options{});if err!=nil{t.Fatal(err)}
    cases:=[]string{
        `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"private":true,`+currentMeta+`}}`,
        `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"cursor":"invalid-token",`+currentMeta+`}}`,
        `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{}}}}`,
        `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{}}}`,
    }
    for i,input:=range cases{t.Run(string(rune('a'+i)),func(t *testing.T){
        replies:=serveText(t,server,input+"\n");requireCode(t,replies[0],-32602)
    })}
}

func TestCurrentUnknownMethodCannotBecomeDiscovery(t *testing.T) {
    server, err := New(Options{})
    if err != nil { t.Fatal(err) }
    for _, method := range []string{"arbitrary", "initialize", "resources/list", "tasks/get", "Tools/list"} {
        t.Run(method, func(t *testing.T) {
            responses := serveText(t, server, `{"jsonrpc":"2.0","id":"synthetic","method":`+strconvQuote(method)+`,"params":{`+currentMeta+`}}`+"\n")
            if len(responses)!=1 { t.Fatalf("want one error; got %d",len(responses)) }
            requireCode(t,responses[0],-32601)
        })
    }
}
