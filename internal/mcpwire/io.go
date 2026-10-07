package mcpwire

import "io"

func writeFrame(output io.Writer,frame []byte)(n int,err error){
    defer func(){if recover()!=nil{n,err=0,ErrOutput}}()
    return output.Write(frame)
}

type guardedReader struct{ source io.Reader }
func(r guardedReader)Read(p []byte)(n int,err error){
    defer func(){if recover()!=nil{n,err=0,ErrInput}}()
    return r.source.Read(p)
}

func closeIO(closer io.Closer,code error)(err error){
    defer func(){if recover()!=nil{err=code}}()
    if closer.Close()!=nil{return code}
    return nil
}
