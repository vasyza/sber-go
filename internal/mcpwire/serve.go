package mcpwire

import (
    "bufio"
    "context"
    "errors"
    "io"
    "sync"
)

var ErrCancelled = errors.New("MCP service cancelled")

type incoming struct{ frame []byte; end bool; err error }
type work struct{ req request; result map[string]any; code int; cancel context.CancelFunc; cancelled bool }

// Serve reads and writes only newline-delimited JSON-RPC frames. Blocking I/O
// must be closable (ownership is transferred to Serve), and callbacks must
// honor context cancellation. Arbitrary non-closable I/O cannot be interrupted.
func(server *Server)Serve(ctx context.Context,input io.Reader,output io.Writer)(serveErr error){
    runCtx,cancel:=context.WithCancel(ctx)
    stop:=make(chan struct{})
    frames:=make(chan incoming)
    completed:=make(chan *work)
    var reader,workers sync.WaitGroup
    var inputOnce,outputOnce sync.Once
    var inputCloseErr,outputCloseErr error
    closeInput:=func(){inputOnce.Do(func(){if closer,ok:=input.(io.Closer);ok{inputCloseErr = closeIO(closer,ErrInput)}})}
    closeOutput:=func(){outputOnce.Do(func(){if closer,ok:=output.(io.Closer);ok{outputCloseErr = closeIO(closer,ErrOutput)}})}
    watcherDone:=make(chan struct{})
    go func(){defer close(watcherDone);select{case<-ctx.Done():closeInput();closeOutput();case<-stop:}}()
    reader.Add(1)
    go func(){
        defer reader.Done()
        scanner:=bufio.NewScanner(guardedReader{source:input})
        scanner.Buffer(make([]byte,4096),MaxFrameBytes+2);scanner.Split(splitFrame)
        for scanner.Scan(){
            next:=incoming{frame:append([]byte(nil),scanner.Bytes()...)}
            select{case frames<-next:case<-stop:return}
        }
        err:=scanner.Err()
        if err!=nil&&err!=ErrFrameTooLarge&&err!=ErrTruncatedFrame{err=ErrInput}
        select{case frames<-incoming{end:true,err:err}:case<-stop:}
    }()
    defer func(){
        close(stop);cancel();closeInput();workers.Wait();reader.Wait();<-watcherDone;closeOutput()
        if serveErr==nil{if outputCloseErr!=nil{serveErr=outputCloseErr}else if inputCloseErr!=nil{serveErr=inputCloseErr}}
    }()
    pending:=0
	active:=make(map[string]*work)
    state:=legacyState{}
    for frames!=nil||pending!=0{
        if ctx.Err()!=nil{return ErrCancelled}
        select{
        case<-ctx.Done():return ErrCancelled
        case done:=<-completed:
            pending--
            done.cancel()
            if done.cancelled { delete(active,idKey(done.req.id)); continue }
            if done.code==0&&done.req.modern{done.result=server.modernResult(done.result)}
            if err:=send(output,done.req.id,done.result,done.code,staticMessage(done.code));err!=nil{return err}
			delete(active,idKey(done.req.id))
        case in:=<-frames:
            if in.end{
                if in.err!=nil{return in.err}
                frames=nil;cancel();continue
            }
            req,code,err:=parseRequest(in.frame)
            if err!=nil{return err}
            if req.notification{
                if code==0&&req.method=="notifications/initialized"&&state.initialized{state.ready=true}
                if code==0&&req.method=="notifications/cancelled"&&validCancellation(req){
                    if job:=active[idKey(req.params["requestId"])];job!=nil{
                        job.cancelled=true;job.cancel()
                    }
                }
                continue
            }
			if req.id!=nil&&active[idKey(req.id)]!=nil{
				if err:=send(output,req.id,nil,-32600,staticMessage(-32600));err!=nil{return err}
				continue
			}
            if code!=0{
                if err:=send(output,req.id,nil,code,staticMessage(code));err!=nil{return err};continue
            }
            result,code,call:=server.route(req,&state)
            if !call{
                if err:=send(output,req.id,result,code,staticMessage(code));err!=nil{return err};continue
            }
            if pending >= server.maxInFlight {
                if err:=send(output,req.id,nil,-32603,staticMessage(-32603));err!=nil{return err}
                continue
            }
            pending++;workers.Add(1)
            requestCtx,requestCancel:=context.WithCancel(runCtx)
            job:=&work{req:req,cancel:requestCancel}
			active[idKey(req.id)]=job
            go func(job *work){
                defer workers.Done()
                job.result,job.code=server.call(requestCtx,job.req.params)
                select{case completed<-job:case<-stop:}
            }(job)
        }
    }
    return nil
}
