package mcpwire

// Only legacy handshake state belongs to a Serve invocation. Modern requests
// never infer version, identity, or capabilities from it.
type legacyState struct{ initialized, ready bool }

func (server *Server) route(req request,state *legacyState)(map[string]any,int,bool){
    if req.modern{
        version,ok:=stringValue(req.meta["io.modelcontextprotocol/protocolVersion"])
        if !ok||!object(req.meta["io.modelcontextprotocol/clientCapabilities"]){return nil,-32602,false}
        if version!=CurrentVersion{return nil,-32022,false}
        if info,exists:=req.meta["io.modelcontextprotocol/clientInfo"];exists&&!validImplementation(info){return nil,-32602,false}
        switch req.method{
        case "server/discover":
            if !allowedFields(req.params,"_meta"){return nil,-32602,false}
            return server.modernResult(map[string]any{"supportedVersions":[]string{CurrentVersion,LegacyVersion},"capabilities":map[string]any{"tools":map[string]any{}}}),0,false
        case "tools/list":
            if !allowedFields(req.params,"_meta"){return nil,-32602,false}
            return server.modernResult(map[string]any{"tools":server.toolList()}),0,false
        case "tools/call":return nil,0,true
        default:return nil,-32601,false
        }
    }
    if req.method!="initialize"&&req.method!="ping"&&req.method!="tools/list"&&req.method!="tools/call"{return nil,-32601,false}
    switch{
    case req.method=="ping":return map[string]any{},0,false
    case req.method=="initialize":
        version,ok:=stringValue(req.params["protocolVersion"])
        if state.initialized||!ok||version!=LegacyVersion||!object(req.params["capabilities"])||!validImplementation(req.params["clientInfo"]){return nil,-32602,false}
        state.initialized=true
        return map[string]any{"protocolVersion":LegacyVersion,"capabilities":map[string]any{"tools":map[string]any{}},"serverInfo":server.info},0,false
    case !state.ready:return nil,-32602,false
    case req.method=="tools/list":return map[string]any{"tools":server.toolList()},0,false
    case req.method=="tools/call":return nil,0,true
    default:return nil,-32601,false
    }
}

func(server *Server)modernResult(fields map[string]any)map[string]any{
    fields["resultType"]="complete"
    fields["_meta"]=map[string]any{"io.modelcontextprotocol/serverInfo":server.info}
    return fields
}
