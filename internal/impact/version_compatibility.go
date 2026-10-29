package impact

import (
 "bytes"
 "fmt"
 "go/ast"
 "go/format"
 "go/parser"
 "go/types"
 "runtime"
 "sort"
)

// CompatibilityCheck compiles a controlled HYBRID: the old package's
// declarations and consumers with ONE new signature transplanted into
// the old declaration AST. It does not compile all new code together
// with external old dependents and must not be called an actual diff
// build of the whole repository.
type CompatibilityCheck struct {
 Attempted bool `json:"attempted"`
 Supported bool `json:"supported"`
 Reason string `json:"reason,omitempty"`
 HybridPackageTypechecked bool `json:"old_consumers_with_new_signature_typechecked"`
 TypeDiagnostics int `json:"type_diagnostics"`
 BrokenOldDirectCallers []string `json:"broken_old_direct_callers"`
 UnattributedErrors int `json:"unattributed_errors"`
}

func transplantSignature(oldGraph,newGraph *Graph,newID int)(*ast.FuncType,error){
 source:=newGraph.Symbols[newID]
 if source.decl==nil||source.decl.Type==nil{return nil,fmt.Errorf("new function type absent")}
 if source.decl.Type.TypeParams!=nil{
  return nil,fmt.Errorf("generic function signatures are not supported by this compatibility test")
 }
 var buf bytes.Buffer
 if err:=format.Node(&buf,newGraph.fset,source.decl.Type);err!=nil{return nil,err}
 // Parsing into the OLD token.FileSet makes diagnostic positions valid.
 // The synthetic file is never typechecked separately or saved to disk.
 declaration:="package hybrid\nvar __mbvm_new_signature "+buf.String()+"\n"
 parsed,err:=parser.ParseFile(oldGraph.fset,"<memory-new-signature>.go",declaration,0)
 if err!=nil{return nil,fmt.Errorf("parse new signature: %w",err)}
 if len(parsed.Decls)!=1{return nil,fmt.Errorf("not exactly one signature")}
 gen,ok:=parsed.Decls[0].(*ast.GenDecl)
 if !ok||len(gen.Specs)!=1{return nil,fmt.Errorf("invalid signature syntax")}
 spec,ok:=gen.Specs[0].(*ast.ValueSpec)
 if !ok{return nil,fmt.Errorf("not a value specification")}
 typ,ok:=spec.Type.(*ast.FuncType)
 if !ok{return nil,fmt.Errorf("not a function type")}
 return typ,nil
}

func CheckOldConsumersWithNewSignature(oldGraph *Graph,oldID int,newGraph *Graph,newID int)CompatibilityCheck{
 result:=CompatibilityCheck{BrokenOldDirectCallers:[]string{}}
 if oldGraph==nil||newGraph==nil||oldGraph.PackagePath!=newGraph.PackagePath||
  oldID<0||newID<0||oldID>=len(oldGraph.Symbols)||newID>=len(newGraph.Symbols){
  result.Reason="different packages or invalid symbol IDs"
  return result
 }
 oldSymbol:=oldGraph.Symbols[oldID]
 newSymbol:=newGraph.Symbols[newID]
 if oldSymbol.decl==nil||newSymbol.decl==nil||oldSymbol.decl.Type==nil{
  result.Reason="one declaration has no parsable function type"
  return result
 }
 if oldSymbol.decl.Type.TypeParams!=nil{
  result.Reason="generic function signatures not supported by hybrid checker"
  return result
 }
 newType,err:=transplantSignature(oldGraph,newGraph,newID)
 if err!=nil{result.Reason=err.Error();return result}
 result.Attempted=true
 original:=oldSymbol.decl.Type
 oldSymbol.decl.Type=newType
 defer func(){oldSymbol.decl.Type=original}()
 var diagnostics []types.Error
 cfg:=types.Config{
  Importer:oldGraph.importer,
  Sizes:types.SizesFor("gc",runtime.GOARCH),
  Error:func(err error){if e,ok:=err.(types.Error);ok{diagnostics=append(diagnostics,e)}},
 }
 _,checkErr:=cfg.Check(oldGraph.PackagePath,oldGraph.fset,oldGraph.files,nil)
 result.HybridPackageTypechecked=(checkErr==nil)
 if checkErr!=nil&&len(diagnostics)==0{
  result.Reason=fmt.Sprintf("Go type checker failure without diagnostic: %v",checkErr)
  return result
 }
 // The hybrid is trustworthy for direct consumer evidence only when
 // the transplanted target declaration/body itself still typechecks.
 // Otherwise errors in old callers can be cascading artifacts.
 invalidTarget:=false
 errored:=map[int]bool{}
 for _,e:=range diagnostics{
  result.TypeDiagnostics++
  owner:=-1
  for _,s:=range oldGraph.Symbols{
   if e.Pos>=s.decl.Pos()&&e.Pos<=s.decl.End(){owner=s.ID;break}
  }
  if owner<0{
   result.UnattributedErrors++
   invalidTarget=true
  }else if owner==oldID{
   invalidTarget=true
  }else{
   errored[owner]=true
  }
 }
 if invalidTarget{
  result.Reason="target declaration or unowned code failed under hybrid; consumer errors may cascade"
  return result
 }
 direct:=make(map[int]bool)
 for _,call:=range oldGraph.Calls{if call.Callee==oldID{direct[call.Caller]=true}}
 for caller:=range errored{
  if direct[caller]{
   result.BrokenOldDirectCallers=append(result.BrokenOldDirectCallers,oldGraph.Symbols[caller].Qualified())
  }
 }
 sort.Strings(result.BrokenOldDirectCallers)
 result.Supported=true
 return result
}
