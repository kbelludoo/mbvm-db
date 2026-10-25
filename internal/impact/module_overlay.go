package impact

import (
 "bytes"
 "encoding/json"
 "fmt"
 "go/ast"
 "go/format"
 "go/parser"
 "go/token"
 "go/types"
 "os"
 "path/filepath"
 "sort"
)

// PreparedOverlay is a temporary, hypothetical provider contract:
// the requested signature is changed and only type-resolved LOCAL call
// sites are adapted. External consumer source remains untouched.
type PreparedOverlay struct {
 Path string
 ReplacedFiles []string
 AdaptedInternalCalls int
}

const crossPackageProbe = "__mbvm_cross_package_probe"

// PrepareContractOverlay parses fresh ASTs with comments preserved, so
// compiler directives and build tags survive the virtual replacements.
// The Graph's own AST is NEVER modified and real source is never written.
func (m *ModuleGraph) PrepareContractOverlay(g *Graph,id int,tmp string)(PreparedOverlay,error){
 var result PreparedOverlay
 if g==nil||id<0||id>=len(g.Symbols){return result,fmt.Errorf("invalid provider symbol")}
 sym:=g.Symbols[id]
 sig,ok:=sym.object.Type().(*types.Signature)
 if !ok||sig.Variadic(){return result,fmt.Errorf("variadic or unknown target signature")}
 if sym.decl==nil||sym.decl.Type==nil||sym.decl.Type.Params==nil{
  return result,fmt.Errorf("target declaration has no parameter list")
 }
 collision:=false
 ast.Inspect(sym.decl,func(n ast.Node)bool{
  if x,ok:=n.(*ast.Ident);ok&&x.Name==crossPackageProbe{collision=true}
  return true
 })
 if collision{return result,fmt.Errorf("reserved overlay parameter collides with source")}
 targetPos:=sym.Position
 localCallPositions:=map[string]bool{}
 for _,site:=range g.Calls{
  if site.Callee==id{localCallPositions[site.Position]=true}
 }
 seenCalls:=map[string]bool{}
 replacements:=map[string]string{}
 targetFound:=false
 fset:=token.NewFileSet()
 for fileIndex,filename:=range g.sourceFiles{
  src,err:=os.ReadFile(filename)
  if err!=nil{return result,fmt.Errorf("read source %s: %w",filename,err)}
  node,err:=parser.ParseFile(fset,filename,src,parser.ParseComments)
  if err!=nil{return result,fmt.Errorf("parse provider %s: %w",filename,err)}
  changed:=false
  for _,decl:=range node.Decls{
   fd,ok:=decl.(*ast.FuncDecl)
   if !ok||fset.Position(fd.Pos()).String()!=targetPos{continue}
   if targetFound{return result,fmt.Errorf("duplicate target declaration at %s",targetPos)}
   targetFound=true
   if fd.Type.Params==nil{return result,fmt.Errorf("target declaration missing params")}
   fd.Type.Params.List=append(fd.Type.Params.List,&ast.Field{
    Names:[]*ast.Ident{ast.NewIdent(crossPackageProbe)},
    Type:ast.NewIdent("int"),
   })
   changed=true
  }
  ast.Inspect(node,func(n ast.Node)bool{
   call,ok:=n.(*ast.CallExpr)
   if !ok{return true}
   pos:=fset.Position(call.Pos()).String()
   if !localCallPositions[pos]{return true}
   // The original Graph proved that this exact call expression
   // resolves to the target types.Func in the provider package.
   if seenCalls[pos]{return true}
   seenCalls[pos]=true
   call.Args=append(call.Args,&ast.BasicLit{Kind:token.INT,Value:"0"})
   result.AdaptedInternalCalls++
   changed=true
   return true
  })
  if !changed{continue}
  var formatted bytes.Buffer
  if err:=format.Node(&formatted,fset,node);err!=nil{
   return result,fmt.Errorf("format virtual source %s: %w",filename,err)
  }
  replacement:=filepath.Join(tmp,fmt.Sprintf("overlay-%03d.go",fileIndex))
  if err:=os.WriteFile(replacement,formatted.Bytes(),0600);err!=nil{return result,err}
  replacements[filename]=replacement
  result.ReplacedFiles=append(result.ReplacedFiles,filename)
 }
 if !targetFound{return result,fmt.Errorf("target declaration was not found in selected Go sources: %s",targetPos)}
 if len(seenCalls)!=len(localCallPositions){
  return result,fmt.Errorf("matched %d of %d typed internal calls; refusing incomplete provider overlay",
   len(seenCalls),len(localCallPositions))
 }
 sort.Strings(result.ReplacedFiles)
 if len(replacements)==0{return result,fmt.Errorf("provider overlay made no source replacements")}
 overlayPath:=filepath.Join(tmp,"overlay.json")
 raw,err:=json.Marshal(struct{Replace map[string]string `json:"Replace"`}{Replace:replacements})
 if err!=nil{return result,err}
 if err:=os.WriteFile(overlayPath,raw,0600);err!=nil{return result,err}
 result.Path=overlayPath
 return result,nil
}
