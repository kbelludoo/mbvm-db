package impact

import (
 "fmt"
 "go/ast"
 "go/build"
 "go/parser"
 "go/token"
 "go/types"
 "os"
 "path/filepath"
 "runtime"
 "sort"
)

// Symbol uses a typechecked Go declaration, not a lexical name match.
// IDs are meaningful only within one Graph snapshot.
type Symbol struct {
 ID int `json:"id"`
 Name string `json:"name"`
 Receiver string `json:"receiver,omitempty"`
 PackagePath string `json:"package_path"`
 Signature string `json:"signature"`
 Position string `json:"position"`
 decl *ast.FuncDecl
 object *types.Func
}
type CallSite struct{
 Caller int `json:"caller"`
 Callee int `json:"callee"`
 Position string `json:"position"`
}

// ImportedCall records a statically resolved call to a non-local Go
// function or concrete method. Callee uses the declaring package path
// and receiver-qualified name, not a method-name-only heuristic.
type ImportedCall struct{
 Caller int `json:"caller"`
 Callee string `json:"callee"`
 Position string `json:"position"`
}
type Graph struct{
 Dir string
 PackagePath string
 Symbols []Symbol
 Calls []CallSite
 ImportedCalls []ImportedCall
 Offsets []uint32 // incoming CSR, len(Symbols)+1
 Callers []uint32
 ExcludedCalls int // interface, external or otherwise not declared in this package
 fset *token.FileSet
 files []*ast.File
 importer types.Importer
 sourceFiles []string
}

// Load checks a single build-selected package using go/types and a
// module-aware export importer. Dynamic calls and external consumers
// are not guessed to match a local function.
func Load(dir string)(*Graph,error){return loadWithPackagePath(dir,"")}

// loadWithPackagePath resolves no-import packages using the path reported
// by the module loader, rather than an invented typed.local name.
// Imported packages are checked for path consistency; never retag an
// already typechecked imported package with a conflicting identity.
func loadWithPackagePath(dir,expected string)(*Graph,error){
 return loadWithSharedImporter(dir,expected,nil,nil)
}

// loadWithSharedImporter keeps the single-package Load behavior unchanged,
// but allows LoadModule to reuse ONE compiler export importer and one
// FileSet across the entire module, without invoking go list per package.
// Shared FileSet/Importer are sequential and must not be used concurrently.
func loadWithSharedImporter(dir,expected string,sharedImporter types.Importer,sharedFileSet *token.FileSet)(*Graph,error){
 if (sharedImporter==nil)!=(sharedFileSet==nil){
  return nil,fmt.Errorf("shared importer and file set must be supplied together")
 }
 if sharedImporter!=nil&&expected==""{
  return nil,fmt.Errorf("shared importer requires an explicit package import path")
 }
 abs,err:=filepath.Abs(dir);if err!=nil{return nil,err}
 selected,err:=build.Default.ImportDir(abs,0)
 if err!=nil{return nil,fmt.Errorf("select Go files: %w",err)}
 if len(selected.CgoFiles)>0{return nil,fmt.Errorf("cgo files unsupported by type-only analyzer")}
 if len(selected.GoFiles)==0{return nil,fmt.Errorf("no build-selected Go files in %s",abs)}
 names:=append([]string(nil),selected.GoFiles...)
 sort.Strings(names)
 fset:=sharedFileSet
 if fset==nil{fset=token.NewFileSet()}
 files:=make([]*ast.File,0,len(names))
 sources:=make([]string,0,len(names))
 for _,name:=range names{
  filename:=filepath.Join(abs,name)
  bytes,err:=os.ReadFile(filename);if err!=nil{return nil,err}
  node,err:=parser.ParseFile(fset,filename,bytes,0);if err!=nil{return nil,fmt.Errorf("parse %s: %w",name,err)}
  files=append(files,node)
  sources=append(sources,filename)
 }
 imp,pkgPath:=sharedImporter,expected
 if imp==nil{
  imp,pkgPath,err=moduleImporter(abs,fset,files,selected.Name)
  if err!=nil{return nil,err}
 }
 if expected!=""{
  if pkgPath!="typed.local/"+selected.Name&&pkgPath!=expected{
   return nil,fmt.Errorf("Go package path mismatch: importer=%s module=%s",pkgPath,expected)
  }
  pkgPath=expected
 }
 info:=&types.Info{Defs:map[*ast.Ident]types.Object{},Uses:map[*ast.Ident]types.Object{},Selections:map[*ast.SelectorExpr]*types.Selection{}}
 cfg:=types.Config{Importer:imp,Sizes:types.SizesFor("gc",runtime.GOARCH)}
 if _,err=cfg.Check(pkgPath,fset,files,info);err!=nil{return nil,fmt.Errorf("baseline Go types: %w",err)}
 g:=&Graph{Dir:abs,PackagePath:pkgPath,fset:fset,files:files,importer:imp,sourceFiles:sources}
 byObject:=make(map[*types.Func]int)
 for _,f:=range files {
  for _,decl:=range f.Decls {
   d,ok:=decl.(*ast.FuncDecl);if !ok{continue}
   obj,ok:=info.Defs[d.Name].(*types.Func);if !ok{return nil,fmt.Errorf("missing typed declaration %s",d.Name.Name)}
   signature,ok:=obj.Type().(*types.Signature);if !ok{return nil,fmt.Errorf("unexpected function type %s",d.Name.Name)}
   recv:=""
   if signature.Recv()!=nil{recv=types.TypeString(signature.Recv().Type(),func(*types.Package)string{return ""})}
   id:=len(g.Symbols)
   g.Symbols=append(g.Symbols,Symbol{
    ID:id,Name:obj.Name(),Receiver:recv,PackagePath:pkgPath,
    Signature:types.TypeString(obj.Type(),func(*types.Package)string{return ""}),
    Position:fset.Position(d.Pos()).String(),decl:d,object:obj,
   })
   byObject[obj]=id
  }
 }
 for _,caller:=range g.Symbols {
  ast.Inspect(caller.decl.Body,func(node ast.Node)bool{
   expr,ok:=node.(*ast.CallExpr);if !ok{return true}
   obj:=calledFunc(expr.Fun,info)
   if obj!=nil{
    if callee,found:=byObject[obj];found{
     g.Calls=append(g.Calls,CallSite{Caller:caller.ID,Callee:callee,Position:fset.Position(expr.Pos()).String()})
     return true
    }
   }
   // A resolved imported function/method may be linked only if the
   // corresponding declaration is included in a later multi-package
   // analysis. Interface dispatch is NEVER assigned to an implementation.
   if obj!=nil&&obj.Pkg()!=nil&&obj.Pkg().Path()!=pkgPath&&!interfaceMethod(obj){
    g.ImportedCalls=append(g.ImportedCalls,ImportedCall{
     Caller:caller.ID,Callee:typedFuncKey(obj),
     Position:fset.Position(expr.Pos()).String(),
    })
   }else{
    // Includes interface dispatch, function values and built-ins.
    g.ExcludedCalls++
   }
   return true
  })
 }
 n:=len(g.Symbols)
 g.Offsets=make([]uint32,n+1)
 for _,c:=range g.Calls {g.Offsets[c.Callee+1]++}
 for i:=1;i<=n;i++{g.Offsets[i]+=g.Offsets[i-1]}
 g.Callers=make([]uint32,len(g.Calls))
 cursor:=append([]uint32(nil),g.Offsets[:n]...)
 for _,c:=range g.Calls{
  at:=cursor[c.Callee];g.Callers[at]=uint32(c.Caller);cursor[c.Callee]++
 }
 return g,nil
}
func calledFunc(expr ast.Expr,info *types.Info)*types.Func{
 switch x:=expr.(type){
 case *ast.Ident: f,_:=info.Uses[x].(*types.Func);return f
 case *ast.SelectorExpr:
  if selection:=info.Selections[x];selection!=nil{
   // Interface dispatch must not be assigned to a particular implementation.
   f,_:=selection.Obj().(*types.Func);return f
  }
  f,_:=info.Uses[x.Sel].(*types.Func);return f
 case *ast.ParenExpr:return calledFunc(x.X,info)
 case *ast.IndexExpr:return calledFunc(x.X,info)
 case *ast.IndexListExpr:return calledFunc(x.X,info)
 default:return nil
 }
}

// typedFuncKey agrees with Symbol.Qualified for the target package, even
// though each package is typechecked in a distinct Go importer instance.
func typedFuncKey(fn *types.Func)string{
 if fn==nil||fn.Pkg()==nil{return ""}
 packagePath:=fn.Pkg().Path()
 sig,ok:=fn.Type().(*types.Signature)
 if ok&&sig.Recv()!=nil{
  qualifier:=func(p *types.Package)string{
   if p==nil||p.Path()==packagePath{return ""}
   return p.Path()
  }
  recv:=types.TypeString(sig.Recv().Type(),qualifier)
  return packagePath+".("+recv+")."+fn.Name()
 }
 return packagePath+"."+fn.Name()
}

func interfaceMethod(fn *types.Func)bool{
 sig,ok:=fn.Type().(*types.Signature)
 if !ok||sig.Recv()==nil{return false}
 recv:=types.Unalias(sig.Recv().Type())
 if ptr,ok:=recv.(*types.Pointer);ok{recv=types.Unalias(ptr.Elem())}
 _,isInterface:=recv.Underlying().(*types.Interface)
 return isInterface
}
