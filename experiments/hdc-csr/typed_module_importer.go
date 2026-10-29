package hdccsr

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

// Resolve actual compiled package export data through the module-aware
// Go command, then supply it to the standard-library gc importer.
// Unlike importer.Default, no $GOPATH/pkg archive is required.
//
// Reuse this importer for the post-mutation typecheck so the expensive
// go list operation runs only once per package load. For no-import
// fixtures we avoid go list, allowing isolated temp directories.
func newModuleAwareImporter(dir string,fset *token.FileSet,files []*ast.File)(types.Importer,error){
 hasImports:=false
 for _,f:=range files{if len(f.Imports)>0{hasImports=true;break}}
 if !hasImports{return importer.Default(),nil}

 cmd:=exec.Command("go","list","-mod=readonly","-deps","-export","-json",".")
 cmd.Dir=dir
 var stdout,stderr bytes.Buffer
 cmd.Stdout=&stdout
 cmd.Stderr=&stderr
 if err:=cmd.Run();err!=nil{
  return nil,fmt.Errorf("go list -deps -export in %s failed: %w: %s",
   dir,err,strings.TrimSpace(stderr.String()))
 }
 var exports=make(map[string]string)
 decoder:=json.NewDecoder(bytes.NewReader(stdout.Bytes()))
 type listing struct{
  ImportPath string
  Export string
  Error *struct{Err string}
  DepsErrors []struct{Err string}
 }
 for{
  var record listing
  if err:=decoder.Decode(&record);err!=nil{
   if errors.Is(err,io.EOF){break}
   return nil,fmt.Errorf("decode module export metadata: %w",err)
  }
  if record.Error!=nil{return nil,fmt.Errorf("go list %s: %s",record.ImportPath,record.Error.Err)}
  if len(record.DepsErrors)>0{return nil,fmt.Errorf("go list %s dependencies: %s",record.ImportPath,record.DepsErrors[0].Err)}
  if record.ImportPath!=""&&record.Export!=""{exports[record.ImportPath]=record.Export}
 }
 if len(exports)==0{
  return nil,fmt.Errorf("go list returned no export archives in %s",dir)
 }
 lookup:=func(importPath string)(io.ReadCloser,error){
  file,ok:=exports[importPath]
  if !ok{return nil,fmt.Errorf("package %q has no module export archive (from %s)",importPath,dir)}
  reader,err:=os.Open(file)
  if err!=nil{return nil,fmt.Errorf("open module export for %q: %w",importPath,err)}
  return reader,nil
 }
 return importer.ForCompiler(fset,"gc",lookup),nil
}
