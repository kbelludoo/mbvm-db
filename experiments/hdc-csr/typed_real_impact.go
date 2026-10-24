package hdccsr

import (
 "fmt"
 "go/ast"
 "go/build"
 "go/importer"
 "go/parser"
 "go/token"
 "go/types"
 "os"
 "path/filepath"
 "runtime"
 "sort"
)

// TypedRealSymbol indexes declarations by *types.Func object identity,
// NOT by a potentially ambiguous function or method name.
type TypedRealSymbol struct {
 ID int `json:"id"`
 Name string `json:"name"`
 Receiver string `json:"receiver,omitempty"`
 PackagePath string `json:"package_path"`
 Position string `json:"position"`
 Signature string `json:"signature"`
 decl *ast.FuncDecl
 obj *types.Func
}

type TypedRealCallSite struct {
 Caller int `json:"caller"`
 Callee int `json:"callee"`
 Position string `json:"position"`
 pos token.Pos
}
type TypedRealGraph struct {
 Fset *token.FileSet
 Files []*ast.File
 PackagePath string
 Nodes []TypedRealSymbol
 CallSites []TypedRealCallSite
 Reverse *RealReverseCallsCSR
}

// One-package, non-CGO, Go-source type resolution; Go build tags select
// the same source files as the current GOOS/GOARCH. Imports must be
// resolvable with the Go standard go/importer. No code is written to disk.
func LoadTypedRealGraph(dir,packagePath string)(*TypedRealGraph,error){
 abs,err:=filepath.Abs(dir);if err!=nil{return nil,err}
 pkg,err:=build.Default.ImportDir(abs,0)
 if err!=nil{return nil,fmt.Errorf("select Go package: %w",err)}
 if len(pkg.CgoFiles)>0{return nil,fmt.Errorf("cgo package requires a different loader")}
 if len(pkg.GoFiles)==0{return nil,fmt.Errorf("no selected Go files")}
 names:=append([]string(nil),pkg.GoFiles...);sort.Strings(names)
 fset:=token.NewFileSet()
 parsed:=make([]*ast.File,0,len(names))
 for _,file:=range names{
  src,err:=os.ReadFile(filepath.Join(abs,file));if err!=nil{return nil,err}
  node,err:=parser.ParseFile(fset,filepath.Join(abs,file),src,0)
  if err!=nil{return nil,fmt.Errorf("parse %s: %w",file,err)}
  parsed=append(parsed,node)
 }
 if packagePath==""{packagePath="typed.local/"+pkg.Name}
 info:=&types.Info{
  Defs:make(map[*ast.Ident]types.Object),
  Uses:make(map[*ast.Ident]types.Object),
  Selections:make(map[*ast.SelectorExpr]*types.Selection),
 }
 cfg:=types.Config{Importer:importer.Default(),Sizes:types.SizesFor("gc",runtime.GOARCH)}
 if _,err=cfg.Check(packagePath,fset,parsed,info);err!=nil{
  return nil,fmt.Errorf("baseline Go typecheck: %w",err)
 }
 result:=&TypedRealGraph{Fset:fset,Files:parsed,PackagePath:packagePath}
 byObject:=make(map[*types.Func]int)
 for _,file:=range parsed{
  for _,decl:=range file.Decls{
   fd,ok:=decl.(*ast.FuncDecl);if !ok{continue}
   fn,ok:=info.Defs[fd.Name].(*types.Func)
   if !ok{return nil,fmt.Errorf("missing type identity for %s",fd.Name.Name)}
   signature,ok:=fn.Type().(*types.Signature)
   if !ok{return nil,fmt.Errorf("missing signature for %s",fn.Name())}
   receiver:=""
   if signature.Recv()!=nil{receiver=types.TypeString(signature.Recv().Type(),func(*types.Package)string{return ""})}
   id:=len(result.Nodes)
   result.Nodes=append(result.Nodes,TypedRealSymbol{
    ID:id,Name:fn.Name(),Receiver:receiver,PackagePath:packagePath,
    Position:fset.Position(fd.Pos()).String(),
    Signature:types.TypeString(fn.Type(),func(*types.Package)string{return ""}),
    decl:fd,obj:fn,
   })
   byObject[fn]=id
  }
 }
 for _,caller:=range result.Nodes{
  ast.Inspect(caller.decl.Body,func(node ast.Node)bool{
   call,ok:=node.(*ast.CallExpr);if !ok{return true}
   target:=typedTargetObject(call.Fun,info)
   if target!=nil{
    if id,ok:=byObject[target];ok{
     result.CallSites=append(result.CallSites,TypedRealCallSite{
      Caller:caller.ID,Callee:id,Position:fset.Position(call.Pos()).String(),pos:call.Pos(),
     })
    }
   }
   return true
  })
 }
 reverse:=&RealReverseCallsCSR{Nodes:len(result.Nodes),Offsets:make([]uint32,len(result.Nodes)+1)}
 for _,site:=range result.CallSites{reverse.Offsets[site.Callee+1]++}
 for i:=1;i<len(reverse.Offsets);i++{reverse.Offsets[i]+=reverse.Offsets[i-1]}
 reverse.Callers=make([]uint32,len(result.CallSites))
 cursor:=append([]uint32(nil),reverse.Offsets[:len(result.Nodes)]...)
 for _,site:=range result.CallSites{
  at:=cursor[site.Callee]
  reverse.Callers[at]=uint32(site.Caller)
  cursor[site.Callee]++
 }
 reverse.Edges=len(reverse.Callers)
 result.Reverse=reverse
 return result,nil
}

// Only direct statically resolved calls to declarations from this package
// are linked. Function values, interface dispatch and reflection remain
// deliberately unresolved; a name match is NEVER substituted.
func typedTargetObject(fun ast.Expr,info *types.Info)*types.Func{
 switch f:=fun.(type){
 case *ast.Ident:
  target,_:=info.Uses[f].(*types.Func);return target
 case *ast.SelectorExpr:
  if sel:=info.Selections[f];sel!=nil{
   target,_:=sel.Obj().(*types.Func);return target
  }
  target,_:=info.Uses[f.Sel].(*types.Func);return target
 case *ast.IndexExpr:return typedTargetObject(f.X,info)
 case *ast.IndexListExpr:return typedTargetObject(f.X,info)
 case *ast.ParenExpr:return typedTargetObject(f.X,info)
 default:return nil
 }
}

type TypedTypeError struct {
 Position string `json:"position"`
 Message string `json:"message"`
 EnclosingSymbol int `json:"enclosing_symbol"`
}
type TypedRealBreakage struct{
 Schema string `json:"schema"`
 Target int `json:"target_id"`
 TargetName string `json:"target_name"`
 SignatureBefore string `json:"signature_before"`
 AddedParameter string `json:"added_parameter"`
 BaselineTypechecked bool `json:"baseline_typechecked"`
 ChangedPackageTypechecked bool `json:"changed_package_typechecked"`
 TypeErrors []TypedTypeError `json:"type_errors"`
 DirectCallers []int `json:"typed_direct_callers"`
 DirectCallersWithErrors []int `json:"direct_callers_with_type_errors"`
 CandidateTransitiveCallers int `json:"potential_indirect_callers"`
 UnattributedErrors int `json:"unattributed_errors"`
 Interpretation string `json:"interpretation"`
}

// Mutate only the parsed, in-memory signature, recheck the whole package,
// attribute diagnostics to enclosing source functions, then RESTORE AST.
// Not a substitute for 'go build' or testing all downstream packages.
func VerifyTypedSignatureBreakage(g *TypedRealGraph,target int)(TypedRealBreakage,error){
 var result TypedRealBreakage
 if g==nil||g.Reverse==nil||target<0||target>=len(g.Nodes){return result,fmt.Errorf("invalid graph or target")}
 symbol:=g.Nodes[target]
 if symbol.decl.Type==nil||symbol.decl.Type.Params==nil{return result,fmt.Errorf("invalid target declaration")}
 for _,field:=range symbol.decl.Type.Params.List{
  if _,variadic:=field.Type.(*ast.Ellipsis);variadic{return result,fmt.Errorf("cannot mutate variadic function by adding required trailing parameter")}
 }
 const extra="__mbvm_typed_contract_probe"
 collision:=false
 ast.Inspect(symbol.decl,func(node ast.Node)bool{
  if id,ok:=node.(*ast.Ident);ok&&id.Name==extra{collision=true}
  return true
 })
 if collision{return result,fmt.Errorf("reserved parameter name collides")}
 result=TypedRealBreakage{
  Schema:"mbvm-typed-go-breakage-v1",Target:target,TargetName:typedName(symbol),
  SignatureBefore:symbol.Signature,AddedParameter:extra+" int",
  BaselineTypechecked:true,
  Interpretation:"Go/types diagnostics after in-memory contract mutation of one complete selected package; neither runtime failures nor cross-package completeness are claimed.",
 }
 direct:=make(map[int]bool)
 for _,site:=range g.CallSites{if site.Callee==target{direct[site.Caller]=true}}
 for id:=range direct{result.DirectCallers=append(result.DirectCallers,id)}
 sort.Ints(result.DirectCallers)
 reach,err:=g.Reverse.Impact(target,NewRealImpactScratch(len(g.Nodes)))
 if err!=nil{return result,err}
 result.CandidateTransitiveCallers=reach.TransitiveCallers

 previous:=symbol.decl.Type.Params.List
 updated:=append([]*ast.Field(nil),previous...)
 updated=append(updated,&ast.Field{Names:[]*ast.Ident{ast.NewIdent(extra)},Type:ast.NewIdent("int")})
 symbol.decl.Type.Params.List=updated
 defer func(){symbol.decl.Type.Params.List=previous}()

 var diagnostics []types.Error
 cfg:=types.Config{
  Importer:importer.Default(),
  Sizes:types.SizesFor("gc",runtime.GOARCH),
  Error:func(err error){if e,ok:=err.(types.Error);ok{diagnostics=append(diagnostics,e)}},
 }
 _,err=cfg.Check(g.PackagePath,g.Fset,g.Files,nil)
 result.ChangedPackageTypechecked=(err==nil)
 if err!=nil&&len(diagnostics)==0{return result,fmt.Errorf("typechecker returned no diagnostic: %w",err)}
 errored:=make(map[int]bool)
 for _,diagnostic:=range diagnostics{
  owner:=-1
  for _,node:=range g.Nodes{
   if diagnostic.Pos>=node.decl.Pos()&&diagnostic.Pos<=node.decl.End(){owner=node.ID;break}
  }
  if owner<0{result.UnattributedErrors++}else{errored[owner]=true}
  result.TypeErrors=append(result.TypeErrors,TypedTypeError{
   Position:g.Fset.Position(diagnostic.Pos).String(),
   Message:diagnostic.Msg,EnclosingSymbol:owner,
  })
 }
 for _,id:=range result.DirectCallers{
  if errored[id]{result.DirectCallersWithErrors=append(result.DirectCallersWithErrors,id)}
 }
 return result,nil
}
func typedName(s TypedRealSymbol)string{
 if s.Receiver!=""{return s.PackagePath+".("+s.Receiver+")."+s.Name}
 return s.PackagePath+"."+s.Name
}
func SelectTypedMutationTarget(g *TypedRealGraph)(int,error){
 if g==nil||g.Reverse==nil{return -1,fmt.Errorf("nil graph")}
 best,degree:=-1,0
 for id,node:=range g.Nodes{
  if node.decl==nil||node.decl.Type==nil||node.decl.Type.Params==nil{continue}
  signature,_:=node.obj.Type().(*types.Signature)
  if signature==nil||signature.Variadic(){continue}
  n:=int(g.Reverse.Offsets[id+1]-g.Reverse.Offsets[id])
  if n>degree{degree=n;best=id}
 }
 if best<0{return -1,fmt.Errorf("no nonvariadic typed declaration with callers")}
 return best,nil
}
