package impact

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "go/importer"
 "go/token"
 "go/types"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "sort"
 "strings"
 "time"
)

// moduleExports is one coherent Go toolchain snapshot for the entire
// selected module. Do not cache it between calls: export paths and build
// tags may change when the user's source/module/configuration changes.
type moduleExports struct {
 packages []goListPackage
 files map[string]string
 importer types.Importer
 fset *token.FileSet
}

// loadModuleExports uses exactly ONE external go list subprocess, unlike
// loading and re-resolving exports for every individual package.
func loadModuleExports(root string)(*moduleExports,error){
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute)
 defer cancel()
 cmd:=exec.CommandContext(ctx,"go","list","-mod=readonly","-deps","-export","-json","./...")
 cmd.Dir=root
 var stdout,stderr bytes.Buffer
 cmd.Stdout=&stdout
 cmd.Stderr=&stderr
 if err:=cmd.Run();err!=nil{
  if ctx.Err()!=nil{return nil,fmt.Errorf("module go list timed out: %w",ctx.Err())}
  return nil,fmt.Errorf("module exports via go list: %w: %s",err,strings.TrimSpace(stderr.String()))
 }
 type item struct{
  ImportPath string
  Dir string
  Export string
  DepOnly bool
  Error *struct{Err string}
  DepsErrors []struct{Err string}
 }
 snapshot:=&moduleExports{files:map[string]string{},fset:token.NewFileSet()}
 seen:=map[string]bool{}
 decoder:=json.NewDecoder(bytes.NewReader(stdout.Bytes()))
 for{
  var e item
  if err:=decoder.Decode(&e);err!=nil{
   if errors.Is(err,io.EOF){break}
   return nil,fmt.Errorf("decode unified go list export JSON: %w",err)
  }
  if e.Error!=nil{return nil,fmt.Errorf("go list %s: %s",e.ImportPath,e.Error.Err)}
  if len(e.DepsErrors)>0{
   return nil,fmt.Errorf("go list %s: %s",e.ImportPath,e.DepsErrors[0].Err)
  }
  if e.ImportPath!=""&&e.Export!=""{
   snapshot.files[e.ImportPath]=e.Export
  }
  // -deps includes the standard library and third-party dependencies.
  // Only packages explicitly selected by ./... become graph nodes.
  if e.DepOnly{continue}
  if e.Dir==""||e.ImportPath==""{
   return nil,fmt.Errorf("selected Go package has no directory/import path")
  }
  abs,err:=filepath.Abs(e.Dir)
  if err!=nil{return nil,err}
  if seen[e.ImportPath]{
   return nil,fmt.Errorf("duplicate selected package %q",e.ImportPath)
  }
  seen[e.ImportPath]=true
  snapshot.packages=append(snapshot.packages,goListPackage{ImportPath:e.ImportPath,Dir:abs})
  if len(snapshot.packages)>2048{
   return nil,fmt.Errorf("module exceeds 2048 Go packages")
  }
 }
 if len(snapshot.packages)==0{return nil,fmt.Errorf("no build-selected Go packages in %s",root)}
 sort.Slice(snapshot.packages,func(i,j int)bool{
  return snapshot.packages[i].ImportPath<snapshot.packages[j].ImportPath
 })
 // The shared importer caches type packages within this one module load.
 // It is consumed sequentially against the single shared token.FileSet.
 lookup:=func(path string)(io.ReadCloser,error){
  file,ok:=snapshot.files[path]
  if !ok{return nil,fmt.Errorf("missing Go compiler export for %q",path)}
  opened,err:=os.Open(file)
  if err!=nil{return nil,fmt.Errorf("read Go export for %s: %w",path,err)}
  return opened,nil
 }
 snapshot.importer=importer.ForCompiler(snapshot.fset,"gc",lookup)
 return snapshot,nil
}

// ModuleLoadStats reports measured wall times. These are timings, not
// guarantees. GoListCalls counts only list subprocesses launched by this
// graph loader; it excludes any future explicit compiler-witness builds.
type ModuleLoadStats struct {
 GoListCalls int `json:"go_list_calls"`
 ExportArchives int `json:"export_archives"`
 Packages int `json:"packages"`
 Symbols int `json:"symbols"`
 Edges int `json:"edges"`
 DiscoveryMilliseconds float64 `json:"discovery_ms"`
 TypecheckMilliseconds float64 `json:"typecheck_ms"`
 LinkMilliseconds float64 `json:"link_ms"`
 TotalMilliseconds float64 `json:"total_ms"`
}

func durationMillis(duration time.Duration)float64{
 return float64(duration)/float64(time.Millisecond)
}
