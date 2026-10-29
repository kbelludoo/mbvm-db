package impact

import (
 "context"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "sort"
 "strings"
 "time"
)

type ConsumerBuildWitness struct{
 Package string `json:"package"`
 BaselineBuildPassed bool `json:"baseline_build_passed"`
 ChangedBuildPassed bool `json:"changed_build_passed"`
 ConfirmedConsumerError bool `json:"confirmed_consumer_error"`
 Diagnostics []string `json:"diagnostics"`
 Reason string `json:"reason,omitempty"`
}
type CrossPackageBreakage struct{
 Schema string `json:"schema"`
 Symbol string `json:"symbol"`
 SourceFilesUnmodified bool `json:"source_files_unmodified"`
 Mode string `json:"mode"`
 ProviderBaselineBuildPassed bool `json:"provider_baseline_build_passed"`
 ProviderOverlayBuildPassed bool `json:"provider_overlay_build_passed"`
 AdaptedInternalCallSites int `json:"adapted_internal_call_sites"`
 ProviderDiagnostics []string `json:"provider_diagnostics"`
 ProviderReason string `json:"provider_reason,omitempty"`
 Consumers []ConsumerBuildWitness `json:"consumers"`
 ConfirmedBrokenConsumers []string `json:"confirmed_broken_consumers"`
 InconclusiveConsumers []string `json:"inconclusive_consumers"`
 Warnings []string `json:"warnings"`
}

func (m *ModuleGraph) buildConsumer(ctx context.Context,pkgPath,output string,overlay string)(bool,[]string,error){
 dir,ok:=m.PackageDirs[pkgPath]
 if !ok{return false,nil,fmt.Errorf("consumer package not loaded %q",pkgPath)}
 rel,err:=filepath.Rel(m.Root,dir)
 if err!=nil||(!filepath.IsLocal(rel)&&rel!="."){
  return false,nil,fmt.Errorf("consumer directory outside module root: %s",dir)
 }
 selector:="./"+filepath.ToSlash(rel)
 args:=[]string{"build","-mod=readonly","-o",output}
 if overlay!=""{args=append(args,"-overlay",overlay)}
 args=append(args,selector)
 cmd:=exec.CommandContext(ctx,"go",args...)
 cmd.Dir=m.Root
 // Builds are compile-only, never execute application/test code.
 cmd.Env=append(os.Environ(),"CGO_ENABLED=0")
 combined,err:=cmd.CombinedOutput()
 if ctx.Err()!=nil{return false,nil,ctx.Err()}
 raw:=strings.TrimSpace(string(combined))
 if len(raw)>16384{raw=raw[:16384]+" [truncated]"}
 if err!=nil&&raw==""{raw=err.Error()}
 if raw==""{return err==nil,[]string{},nil}
 return err==nil,strings.Split(raw,"\n"),nil
}
func containsConsumerDiagnostic(lines []string,pkgDir,root string)bool{
 rel,err:=filepath.Rel(root,pkgDir)
 if err!=nil{return false}
 rel=filepath.ToSlash(rel)
 for _,s:=range lines{
  line:=filepath.ToSlash(strings.TrimSpace(s))
  // Do not label an error in the changed callee package as an error
  // of the downstream consumer being analyzed.
  if strings.HasPrefix(line,rel+"/")&&strings.Contains(line,":") {
   return true
  }
  if strings.HasPrefix(line,filepath.ToSlash(pkgDir)+"/")&&strings.Contains(line,":"){
   return true
  }
 }
 return false
}

// CheckCrossPackageBuild performs baseline and virtual-mutation go build
// on every DIRECT static consumer in the loaded module. It may update
// the Go build cache, but original Go sources and Git remain unchanged.
func (m *ModuleGraph) CheckCrossPackageBuild(target string)(CrossPackageBreakage,error){
 result:=CrossPackageBreakage{
  Schema:"mbvm-cross-package-breakage-v1",Symbol:target,
  Mode:"go build -mod=readonly -overlay (CGO_ENABLED=0)",
  Consumers:[]ConsumerBuildWitness{},ProviderDiagnostics:[]string{},
  ConfirmedBrokenConsumers:[]string{},
  InconclusiveConsumers:[]string{},
  Warnings:[]string{
   "Only direct statically linked consumers in loaded Go packages are checked.",
   "The baseline provider and self-consistent overlay provider must compile before consumer verification.",
   "Only typed local provider calls are adapted (synthetic int argument 0); original consumer calls are never adapted.",
   "Compiler errors are attributed to consumers only at known static target call-site lines.",
   "Go build may update the compiler build cache and writes its binary only to temporary files.",
   "Interface dispatch, reflection, function values and alternate build tags are not covered.",
  },
 }
 g,id,err:=m.FindSymbol(target);if err!=nil{return result,err}
 originalHashes:=map[*Graph]string{}
 for _,pkgGraph:=range m.Packages{
  hash,err:=pkgGraph.SourceSHA256()
  if err!=nil{return result,err}
  originalHashes[pkgGraph]=hash
 }
 consumers:=m.DirectConsumerPackages(target)
 if len(consumers)==0{
  for pkgGraph,before:=range originalHashes{
   after,err:=pkgGraph.SourceSHA256()
   if err!=nil{return result,err}
   if before!=after{return result,fmt.Errorf("Go source changed in package %s",pkgGraph.PackagePath)}
  }
  result.SourceFilesUnmodified=true
  return result,nil
 }
 tmp,err:=os.MkdirTemp("","mbvm-cross-compile-")
 if err!=nil{return result,err}
 defer os.RemoveAll(tmp)
 prepared,err:=m.PrepareContractOverlay(g,id,tmp)
 if err!=nil{return result,fmt.Errorf("prepare self-consistent contract overlay: %w",err)}
 result.AdaptedInternalCallSites=prepared.AdaptedInternalCalls
 // Validate the provider before its consumers. This prevents a failure in
 // a dependency from masquerading as a confirmed downstream breakage.
 providerInconclusive:=func(reason string,diagnostics []string)(CrossPackageBreakage,error){
  result.ProviderReason=reason
  result.ProviderDiagnostics=diagnostics
  for _,pkg:=range consumers{
   result.Consumers=append(result.Consumers,ConsumerBuildWitness{
    Package:pkg,Diagnostics:[]string{},Reason:"provider build inconclusive: "+reason,
   })
   result.InconclusiveConsumers=append(result.InconclusiveConsumers,pkg)
  }
  for pkgGraph,before:=range originalHashes{
   after,err:=pkgGraph.SourceSHA256()
   if err!=nil{return result,err}
   if before!=after{return result,fmt.Errorf("Go source changed in package %s",pkgGraph.PackagePath)}
  }
  result.SourceFilesUnmodified=true
  return result,nil
 }
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute)
 providerBaseline,providerBaselineDiagnostics,providerBaselineErr:=m.buildConsumer(
  ctx,g.PackagePath,filepath.Join(tmp,"provider-baseline"),"")
 cancel()
 result.ProviderBaselineBuildPassed=providerBaseline
 if providerBaselineErr!=nil||!providerBaseline{
  reason:="baseline provider package does not compile"
  if providerBaselineErr!=nil{reason+=": "+providerBaselineErr.Error()}
  return providerInconclusive(reason,providerBaselineDiagnostics)
 }
 ctx,cancel=context.WithTimeout(context.Background(),2*time.Minute)
 providerOK,providerDiag,providerErr:=m.buildConsumer(
  ctx,g.PackagePath,filepath.Join(tmp,"provider-overlay"),prepared.Path)
 cancel()
 result.ProviderOverlayBuildPassed=providerOK
 if providerErr!=nil||!providerOK{
  reason:="self-consistent virtual provider does not compile"
  if providerErr!=nil{reason+=": "+providerErr.Error()}
  return providerInconclusive(reason,providerDiag)
 }
 for idx,pkg:=range consumers{
  check:=ConsumerBuildWitness{Package:pkg,Diagnostics:[]string{}}
  ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute)
  baseline,basediags,baseErr:=m.buildConsumer(ctx,pkg,filepath.Join(tmp,fmt.Sprintf("base-%d",idx)),"")
  cancel()
  check.BaselineBuildPassed=baseline
  if baseErr!=nil||!baseline{
   check.Reason="baseline Go build failed; cannot attribute changed build errors to this contract"
   if baseErr!=nil{check.Reason+=": "+baseErr.Error()}
   check.Diagnostics=basediags
   result.InconclusiveConsumers=append(result.InconclusiveConsumers,pkg)
   result.Consumers=append(result.Consumers,check)
   continue
  }
  ctx,cancel=context.WithTimeout(context.Background(),2*time.Minute)
  passed,diagnostics,changedErr:=m.buildConsumer(ctx,pkg,filepath.Join(tmp,fmt.Sprintf("changed-%d",idx)),prepared.Path)
  cancel()
  check.ChangedBuildPassed=passed
  check.Diagnostics=diagnostics
  if changedErr!=nil{
   check.Reason="mutated Go build could not complete: "+changedErr.Error()
   result.InconclusiveConsumers=append(result.InconclusiveConsumers,pkg)
  }else if !passed&&m.matchesTargetCallDiagnostic(pkg,target,diagnostics){
   check.ConfirmedConsumerError=true
   result.ConfirmedBrokenConsumers=append(result.ConfirmedBrokenConsumers,pkg)
  }else if !passed{
   check.Reason="mutated build failed without a diagnostic at a known typed target call site in this consumer"
   result.InconclusiveConsumers=append(result.InconclusiveConsumers,pkg)
  }
  result.Consumers=append(result.Consumers,check)
 }
 for pkgGraph,before:=range originalHashes{
  after,err:=pkgGraph.SourceSHA256()
  if err!=nil{return result,err}
  if before!=after{return result,fmt.Errorf("Go source changed during overlay compilation in package %s",pkgGraph.PackagePath)}
 }
 result.SourceFilesUnmodified=true
 sort.Strings(result.ConfirmedBrokenConsumers)
 sort.Strings(result.InconclusiveConsumers)
 return result,nil
}
