package main

import (
 "encoding/json"
 "fmt"
 "io"

 "github.com/kbelludoo/mbvm-db/internal/impact"
)

// realDiffDispatch preserves the existing impact-diff one-package
// contract while making module-wide real-version checks explicit.
func realDiffDispatch(oldDir,newDir,repo,oldRef,newRef,packageRel,symbol string,out io.Writer,asJSON bool)error{
 if symbol==""{return fmt.Errorf("-module requires one fully qualified -symbol")}
 if packageRel!="."{return fmt.Errorf("-package is incompatible with -module (use module root)")}
 refs:=oldRef!=""||newRef!=""
 var report impact.RealModuleDiff
 var err error
 if refs{
  if oldRef==""||newRef==""||oldDir!=""||newDir!=""{
   return fmt.Errorf("choose both -old-ref/-new-ref or both -old/-new module directories")
  }
  report,err=impact.CompareRealModuleGitRefs(repo,oldRef,newRef,symbol)
 }else{
  if oldDir==""||newDir==""||repo!="."{
   return fmt.Errorf("directory mode requires -old and -new module roots without -repo")
  }
  report,err=impact.CompareRealModuleDirs(oldDir,newDir,symbol)
 }
 if err!=nil{return err}
 if asJSON{
  encoder:=json.NewEncoder(out);encoder.SetIndent("","  ")
  return encoder.Encode(report)
 }
 fmt.Fprintf(out,"Symbol: %s\nReal contract changed: %t\n",report.Symbol,report.ContractChanged)
 fmt.Fprintf(out,"Old contract: %s\nNew contract: %s\n",report.OldContract,report.NewContract)
 fmt.Fprintf(out,"New provider baseline compiled: %t\n",report.ProviderBaselinePassed)
 fmt.Fprintf(out,"Old direct consumer packages: %d\n",len(report.OldDirectConsumers))
 fmt.Fprintf(out,"Confirmed broken: %d\nCompatible in this compilation: %d\nInconclusive: %d\n",
  len(report.ConfirmedBrokenConsumers),len(report.CompatibleInTest),len(report.InconclusiveConsumers))
 for _,c:=range report.Consumers{
  fmt.Fprintf(out,"  %s: %s",c.Package,c.Status)
  if c.Reason!=""{fmt.Fprintf(out," (%s)",c.Reason)}
  fmt.Fprintln(out)
 }
 return nil
}
