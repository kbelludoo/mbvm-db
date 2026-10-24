package impact

import (
 "bytes"
 "encoding/json"
 "errors"
 "fmt"
 "go/ast"
 "go/importer"
 "go/token"
 "go/types"
 "io"
 "os"
 "os/exec"
 "strings"
)

// Resolve imported packages using the Go module/workspace build system,
// without the obsolete GOPATH/pkg archive assumption or invented stubs.
// Only compiler export files are opened; no source files are modified.
func moduleImporter(dir string,fset *token.FileSet,files []*ast.File,name string)(types.Importer,string,error){
 imports:=false
 for _,f:=range files{if len(f.Imports)>0{imports=true;break}}
 if !imports {
  return importer.Default(),"typed.local/"+name,nil
 }
 cmd:=exec.Command("go","list","-mod=readonly","-deps","-export","-json",".")
 cmd.Dir=dir
 var stdout,stderr bytes.Buffer
 cmd.Stdout=&stdout;cmd.Stderr=&stderr
 if err:=cmd.Run();err!=nil{
  return nil,"",fmt.Errorf("module exports via go list: %w: %s",err,strings.TrimSpace(stderr.String()))
 }
 type entry struct{
  ImportPath string
  Dir string
  Export string
  Error *struct{Err string}
  DepsErrors []struct{Err string}
 }
 exports:=map[string]string{}
 pkgPath:=""
 decoder:=json.NewDecoder(bytes.NewReader(stdout.Bytes()))
 for{
  var e entry
  if err:=decoder.Decode(&e);err!=nil{
   if errors.Is(err,io.EOF){break}
   return nil,"",fmt.Errorf("invalid go list JSON: %w",err)
  }
  if e.Error!=nil{return nil,"",fmt.Errorf("go list %s: %s",e.ImportPath,e.Error.Err)}
  if len(e.DepsErrors)>0{return nil,"",fmt.Errorf("go list %s: %s",e.ImportPath,e.DepsErrors[0].Err)}
  if e.ImportPath!=""&&e.Export!=""{exports[e.ImportPath]=e.Export}
  if e.Dir==dir{pkgPath=e.ImportPath}
 }
 if pkgPath==""{return nil,"",fmt.Errorf("go list could not identify analyzed package path in %s",dir)}
 if len(exports)==0{return nil,"",fmt.Errorf("go list found no compiled export data in %s",dir)}
 lookup:=func(path string)(io.ReadCloser,error){
  file,ok:=exports[path]
  if !ok{return nil,fmt.Errorf("no Go export file for %q",path)}
  opened,err:=os.Open(file)
  if err!=nil{return nil,fmt.Errorf("open export file %q: %w",path,err)}
  return opened,nil
 }
 return importer.ForCompiler(fset,"gc",lookup),pkgPath,nil
}
