package mcpwire

import (
    "encoding/json"
    "strings"
)

// MaxIDBytes bounds the original encoded correlation token, not float precision.
const MaxIDBytes = 256

func validID(raw json.RawMessage) bool {
    if len(raw)==0 || len(raw)>MaxIDBytes { return false }
    if _,ok:=stringValue(raw);ok {return true}
    text:=string(raw)
    if strings.HasPrefix(text,"-") {text=text[1:]}
    if text=="" || (len(text)>1 && text[0]=='0') {return false}
    for _,c:=range []byte(text) {if c<'0'||c>'9'{return false}}
    return true
}

func idKey(raw json.RawMessage) string {
    if text,ok:=stringValue(raw);ok{return "s:"+text}
    text:=string(raw)
    if text=="-0"{text="0"}
    return "i:"+text
}
