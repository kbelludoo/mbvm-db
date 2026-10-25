package impact

import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "go/ast"
 "go/format"
 "go/types"
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
 Consumers []ConsumerBuildWitness `json:"consumers"`
 ConfirmedBrokenConsumers []string `json:"confirmed_broken_consumers"`
 InconclusiveConsumers []string `json:"inconclusive_consumers"`
 Warnings []string `json:"warnings"`
}

// Write a virtual replacement of ONE parsed source file to a temporary
// directory and use Go's -overlay flag. The original file is never edited.
func (m *ModuleGraph) contractOverlay(g *Graph,id int,tmp string)(string,error){
 sym:=g.Symbols[id]
 sig,ok:=sym.object.Type().(*types.Signature)
 if !ok||sig.Variadic(){return "",fmt.Errorf("variadic/unknown function signature unsupported")}
 original:=sym.decl.Type.Params.List
 for _,f:=range original{
  for _,name:=range f.Names{
   if name.Name=="__mbvm_cross_package_probe"{
    return "",fmt.Errorf("reserved mutation parameter already exists")
   }
  }
 }
 changed:=append([]*ast.Field(nil),original...)
 changed=append(changed,&ast.Field{
  Names:[]*ast.Ident{ast.NewIdent("__mbvm_cross_package_probe")},
  Type:ast.NewIdent("int"),
 })
 filename:=g.fset.Position(sym.decl.Pos()).Filename
 var formatted bytes.Buffer
 found:=false
 sym.decl.Type.Params.List=changed
 // Restore AST before any subprocess is run or return is taken.
 for _,node:=range g.files{
  if g.fset.Position(node.Pos()).Filename!=filename{continue}
  found=true
  err:=format.Node(&formatted,g.fset,node)
  sym.decl.Type.Params.List=original
  if err!=nil{return "",fmt.Errorf("format virtual source: %w",err)}
  break
 }
 if !found{
  sym.decl.Type.Params.List=original
  return "",fmt.Errorf("source file for %s not found in analyzed AST",sym.Qualified())
 }
 sourcePath:=filepath.Join(tmp,"replaced.go")
 if err:=os.WriteFile(sourcePath,formatted.Bytes(),0600);err!=nil{return "",err}
 overlayPath:=filepath.Join(tmp,"overlay.json")
 raw,err:=json.Marshal(struct{
  Replace map[string]string `json:"Replace"`
 }{Replace:map[string]string{filename:sourcePath}})
 if err!=nil{return "",err}
 if err:=os.WriteFile(overlayPath,raw,0600);err!=nil{return "",err}
 return overlayPath,nil
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
  Consumers:[]ConsumerBuildWitness{},ConfirmedBrokenConsumers:[]string{},
  InconclusiveConsumers:[]string{},
  Warnings:[]string{
   "Only direct statically linked consumers in loaded Go packages are checked.",
   "The baseline must compile; changed builds with only target-package diagnostics are inconclusive.",
   "Go build may update the compiler build cache and writes its binary only to temporary files.",
   "Interface dispatch, reflection, function values and alternate build tags are not covered.",
  },
 }
 g,id,err:=m.FindSymbol(target);if err!=nil{return result,err}
 before,err:=g.SourceSHA256();if err!=nil{return result,err}
 consumers:=m.DirectConsumerPackages(target)
 if len(consumers)==0{
  result.SourceFilesUnmodified=true
  return result,nil
 }
 tmp,err:=os.MkdirTemp("","mbvm-cross-compile-")
 if err!=nil{return result,err}
 defer os.RemoveAll(tmp)
 overlay,err:=m.contractOverlay(g,id,tmp)
 if err!=nil{return result,err}
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
  passed,diagnostics,changedErr:=m.buildConsumer(ctx,pkg,filepath.Join(tmp,fmt.Sprintf("changed-%d",idx)),overlay)
  cancel()
  check.ChangedBuildPassed=passed
  check.Diagnostics=diagnostics
  if changedErr!=nil{
   check.Reason="mutated Go build could not complete: "+changedErr.Error()
   result.InconclusiveConsumers=append(result.InconclusiveConsumers,pkg)
  }else if !passed&&containsConsumerDiagnostic(diagnostics,m.PackageDirs[pkg],m.Root){
   check.ConfirmedConsumerError=true
   result.ConfirmedBrokenConsumers=append(result.ConfirmedBrokenConsumers,pkg)
  }else if !passed{
   check.Reason="mutated build failed without a consumer-package diagnostic"
   result.InconclusiveConsumers=append(result.InconclusiveConsumers,pkg)
  }
  result.Consumers=append(result.Consumers,check)
 }
 after,err:=g.SourceSHA256();if err!=nil{return result,err}
 if before!=after{return result,fmt.Errorf("original Go source changed during overlay compilation")}
 result.SourceFilesUnmodified=true
 sort.Strings(result.ConfirmedBrokenConsumers)
 sort.Strings(result.InconclusiveConsumers)
 return result,nil
}
