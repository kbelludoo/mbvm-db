package impact

import (
 "crypto/sha256"
 "encoding/hex"
 "fmt"
 "go/ast"
 "go/types"
 "io"
 "os"
 "path/filepath"
 "runtime"
 "sort"
)

// SourceSHA256 hashes names and byte contents of the exact Go files
// selected for this build variant. The digest is stable across copies
// on different machines; no bytes are changed by this command.
func (g *Graph) SourceSHA256()(string,error){
 if g==nil{return "",fmt.Errorf("nil graph")}
 names:=append([]string(nil),g.sourceFiles...)
 sort.Strings(names)
 hash:=sha256.New()
 for _,file:=range names{
  raw,err:=os.ReadFile(file);if err!=nil{return "",err}
  name:=filepath.Base(file)
  io.WriteString(hash,name);hash.Write([]byte{0})
  hash.Write(raw);hash.Write([]byte{0})
 }
 return hex.EncodeToString(hash.Sum(nil)),nil
}

type CompileDiagnostic struct {
 Position string `json:"position"`
 Message string `json:"message"`
 EnclosingSymbol string `json:"enclosing_symbol,omitempty"`
}

type Simulation struct {
 Applied bool `json:"applied"`
 Mutation string `json:"mutation"`
 MutatedPackageTypechecked bool `json:"mutated_package_typechecked"`
 BrokenDirectCallers []string `json:"broken_direct_callers"`
 TypeDiagnostics int `json:"total_type_diagnostics"`
 Diagnostics []CompileDiagnostic `json:"diagnostics"`
 UnattributedDiagnostics int `json:"unattributed_diagnostics"`
}

// SimulateAddIntParameter verifies the whole selected package while the
// AST is modified in memory, and reverts it before returning. It does
// not compile downstream modules or prove runtime behavior.
func (g *Graph) SimulateAddIntParameter(id int)(Simulation,error){
 result:=Simulation{BrokenDirectCallers:[]string{},Diagnostics:[]CompileDiagnostic{}}
 if g==nil||id<0||id>=len(g.Symbols){return result,fmt.Errorf("invalid target")}
 symbol:=g.Symbols[id]
 if symbol.decl==nil||symbol.decl.Type==nil||symbol.decl.Type.Params==nil{
  return result,fmt.Errorf("missing function parameter list")
 }
 sig,ok:=symbol.object.Type().(*types.Signature)
 if !ok||sig.Variadic(){return result,fmt.Errorf("cannot add a required trailing parameter to a variadic function")}
 const name="__mbvm_contract_probe"
 collision:=false
 ast.Inspect(symbol.decl,func(node ast.Node)bool{
  if ident,ok:=node.(*ast.Ident);ok&&ident.Name==name{collision=true}
  return true
 })
 if collision{return result,fmt.Errorf("reserved simulation parameter collides with source")}
 before,err:=g.SourceSHA256();if err!=nil{return result,err}
 original:=symbol.decl.Type.Params.List
 changed:=append([]*ast.Field(nil),original...)
 changed=append(changed,&ast.Field{Names:[]*ast.Ident{ast.NewIdent(name)},Type:ast.NewIdent("int")})
 symbol.decl.Type.Params.List=changed
 // Do not retain any mutation, even if go/types fails.
 defer func(){symbol.decl.Type.Params.List=original}()
 result.Applied=true
 result.Mutation="added required parameter '"+name+" int'"
 var typeErrors []types.Error
 check:=types.Config{
  Importer:g.importer,
  Sizes:types.SizesFor("gc",runtime.GOARCH),
  Error:func(err error){
   if e,ok:=err.(types.Error);ok{typeErrors=append(typeErrors,e)}
  },
 }
 _,checkErr:=check.Check(g.PackagePath,g.fset,g.files,nil)
 result.MutatedPackageTypechecked=checkErr==nil
 if checkErr!=nil&&len(typeErrors)==0{return result,fmt.Errorf("type checker failed without typed diagnostic: %w",checkErr)}
 direct:=make(map[int]bool)
 for _,c:=range g.Calls{if c.Callee==id{direct[c.Caller]=true}}
 broken:=make(map[int]bool)
 for _,diagnostic:=range typeErrors {
  owner:=-1
  for index,s:=range g.Symbols{
   if diagnostic.Pos>=s.decl.Pos()&&diagnostic.Pos<=s.decl.End(){owner=index;break}
  }
  entry:=CompileDiagnostic{Position:g.fset.Position(diagnostic.Pos).String(),Message:diagnostic.Msg}
  if owner>=0{
   entry.EnclosingSymbol=g.Symbols[owner].Qualified()
   if direct[owner]{broken[owner]=true}
  }else{result.UnattributedDiagnostics++}
  result.Diagnostics=append(result.Diagnostics,entry)
 }
 result.TypeDiagnostics=len(result.Diagnostics)
 for caller:=range broken{
  result.BrokenDirectCallers=append(result.BrokenDirectCallers,g.Symbols[caller].Qualified())
 }
 sort.Strings(result.BrokenDirectCallers)
 after,err:=g.SourceSHA256()
 if err!=nil{return result,err}
 if before!=after{return result,fmt.Errorf("source files changed while running in-memory simulation")}
 return result,nil
}
