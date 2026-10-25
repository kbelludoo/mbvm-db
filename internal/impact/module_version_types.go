package impact

import (
 "crypto/sha256"
 "encoding/hex"
 "fmt"
 "io"
 "io/fs"
 "os"
 "path/filepath"
 "sort"
 "strings"
)

// RealVersionWitness checks a real new-version provider with unmodified
// old-version client sources supplied via Go build overlays.
type RealVersionWitness struct{
 Package string `json:"package"`
 OldBaselinePassed bool `json:"old_baseline_passed"`
 NewBaselinePassed bool `json:"new_baseline_passed"`
 OldClientWithNewProviderPassed bool `json:"old_client_with_new_provider_passed"`
 ConfirmedCompileErrors []string `json:"confirmed_compile_errors"`
 Diagnostics []string `json:"diagnostics"`
 Status string `json:"status"` // broken | compatible_in_test | inconclusive
 Reason string `json:"reason,omitempty"`
}
type RealModuleDiff struct{
 Schema string `json:"schema"`
 Symbol string `json:"symbol"`
 OldContract string `json:"old_contract"`
 NewContract string `json:"new_contract"`
 ContractChanged bool `json:"contract_changed"`
 SourceFilesUnmodified bool `json:"source_files_unmodified"`
 OldSourceSHA256 string `json:"old_source_sha256"`
 NewSourceSHA256 string `json:"new_source_sha256"`
 ProviderBaselinePassed bool `json:"provider_baseline_passed"`
 OldDirectConsumers []string `json:"old_direct_consumers"`
 ConfirmedBrokenConsumers []string `json:"confirmed_broken_consumers"`
 CompatibleInTest []string `json:"compatible_in_test"`
 InconclusiveConsumers []string `json:"inconclusive_consumers"`
 Consumers []RealVersionWitness `json:"consumers"`
 Warnings []string `json:"warnings"`
}

// digestGoSourceTree covers all Go sources and go.mod/sum/work files, not
// only the files selected by a single build configuration.
func digestGoSourceTree(root string)(string,error){
 files:=[]string{}
 err:=filepath.WalkDir(root,func(path string,d fs.DirEntry,err error)error{
  if err!=nil{return err}
  if d.IsDir() {
   if d.Name()==".git"{return filepath.SkipDir}
   return nil
  }
  if !d.Type().IsRegular(){return nil}
  name:=d.Name()
  if strings.HasSuffix(name,".go")||name=="go.mod"||name=="go.sum"||
   name=="go.work"||name=="go.work.sum"{files=append(files,path)}
  return nil
 })
 if err!=nil{return "",err}
 sort.Strings(files)
 h:=sha256.New()
 for _,file:=range files{
  rel,err:=filepath.Rel(root,file);if err!=nil{return "",err}
  io.WriteString(h,filepath.ToSlash(rel));h.Write([]byte{0})
  f,err:=os.Open(file);if err!=nil{return "",err}
  _,copyErr:=io.Copy(h,f)
  closeErr:=f.Close()
  if copyErr!=nil{return "",copyErr}
  if closeErr!=nil{return "",closeErr}
  h.Write([]byte{0})
 }
 return hex.EncodeToString(h.Sum(nil)),nil
}

func snapshotPackageDirs(root string)(map[string]string,error){
 pkgs,err:=discoverModulePackages(root);if err!=nil{return nil,err}
 found:=map[string]string{}
 for _,p:=range pkgs{
  rel,err:=filepath.Rel(root,p.Dir)
  if err!=nil||(!filepath.IsLocal(rel)&&rel!="."){
   return nil,fmt.Errorf("package outside snapshot module %s",p.Dir)
  }
  if _,exists:=found[p.ImportPath];exists{return nil,fmt.Errorf("duplicate package %q",p.ImportPath)}
  found[p.ImportPath]=p.Dir
 }
 return found,nil
}
