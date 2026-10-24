package hdccsr

import (
 "fmt"
 "go/ast"
 "go/parser"
 "go/token"
 "math/big"
 "sort"
 "strconv"
 "strings"
)

// ASTCanonicalIntFunction normalizes a deliberately restricted subset of
// pure, two-argument Go integer functions. Equal forms certify equivalent
// arithmetic expressions/conditional branches within that subset.
// Unequal forms mean "not proved equivalent", NOT "proved different".
// Unsupported input is rejected explicitly.
func ASTCanonicalIntFunction(src string) (string,error) {
 if len(src)>4096{return "",fmt.Errorf("snippet exceeds 4096 bytes")}
 f,err:=parser.ParseFile(token.NewFileSet(),"candidate.go","package semanticlab\n"+src,0)
 if err!=nil{return "",err}
 if len(f.Decls)!=1{return "",fmt.Errorf("expected a single function")}
 fn,ok:=f.Decls[0].(*ast.FuncDecl)
 if !ok||fn.Recv!=nil||fn.Body==nil||fn.Type.TypeParams!=nil{return "",fmt.Errorf("only free functions supported")}
 if fn.Type.Params==nil||fn.Type.Results==nil||len(fn.Type.Results.List)!=1{return "",fmt.Errorf("expected two int parameters and one int return")}
 result:=fn.Type.Results.List[0]
 ty,ok:=result.Type.(*ast.Ident)
 if !ok||ty.Name!="int"||len(result.Names)!=0{return "",fmt.Errorf("expected unnamed int return")}
 names:=map[string]astMonomial{}
 slot:=0
 for _,p:=range fn.Type.Params.List{
  pt,ok:=p.Type.(*ast.Ident)
  if !ok||pt.Name!="int"{return "",fmt.Errorf("only int arguments supported")}
  for _,n:=range p.Names{
   if n.Name=="_"{return "",fmt.Errorf("blank argument unsupported")}
   if _,exists:=names[n.Name];exists{return "",fmt.Errorf("duplicate argument")}
   if slot==0{names[n.Name]=astMonomial{a:1}
   }else if slot==1{names[n.Name]=astMonomial{b:1}
   }else{return "",fmt.Errorf("expected exactly two parameters")}
   slot++
  }
 }
 if slot!=2{return "",fmt.Errorf("expected exactly two named parameters")}
 return astCanonicalBody(fn.Body.List,names)
}

type astMonomial struct{a,b uint8}
type astPolynomial map[astMonomial]*big.Int

func astConstant(v *big.Int)astPolynomial{
 if v.Sign()==0{return astPolynomial{}}
 return astPolynomial{{}:new(big.Int).Set(v)}
}
func astAdd(x,y astPolynomial,neg bool)astPolynomial{
 result:=make(astPolynomial,len(x)+len(y))
 for k,v:=range x{result[k]=new(big.Int).Set(v)}
 for k,v:=range y{
  next:=new(big.Int).Set(v)
  if neg{next.Neg(next)}
  if prev,ok:=result[k];ok{prev.Add(prev,next);if prev.Sign()==0{delete(result,k)}
  }else if next.Sign()!=0{result[k]=next}
 }
 return result
}
func astMultiply(x,y astPolynomial)(astPolynomial,error){
 result:=make(astPolynomial)
 for a,va:=range x{
  for b,vb:=range y{
   degreeA,degreeB:=int(a.a)+int(b.a),int(a.b)+int(b.b)
   if degreeA+degreeB>8{return nil,fmt.Errorf("polynomial degree exceeds 8")}
   k:=astMonomial{uint8(degreeA),uint8(degreeB)}
   v:=new(big.Int).Mul(va,vb)
   if old,exists:=result[k];exists{old.Add(old,v);if old.Sign()==0{delete(result,k)}
   }else if v.Sign()!=0{result[k]=v}
   if len(result)>128{return nil,fmt.Errorf("polynomial term budget exceeded")}
  }
 }
 return result,nil
}
func astExpression(expr ast.Expr,vars map[string]astMonomial)(astPolynomial,error){
 switch n:=expr.(type){
 case *ast.Ident:
  m,ok:=vars[n.Name]
  if !ok{return nil,fmt.Errorf("unbound or impure identifier %q",n.Name)}
  return astPolynomial{m:big.NewInt(1)},nil
 case *ast.BasicLit:
  if n.Kind!=token.INT{return nil,fmt.Errorf("only integer literals")}
  v,ok:=new(big.Int).SetString(strings.ReplaceAll(n.Value,"_",""),0)
  if !ok{return nil,fmt.Errorf("invalid integer literal")}
  return astConstant(v),nil
 case *ast.ParenExpr:return astExpression(n.X,vars)
 case *ast.UnaryExpr:
  if n.Op!=token.ADD&&n.Op!=token.SUB{return nil,fmt.Errorf("unsupported unary %s",n.Op)}
  p,err:=astExpression(n.X,vars)
  if err!=nil{return nil,err}
  if n.Op==token.SUB{return astAdd(astPolynomial{},p,true),nil}
  return p,nil
 case *ast.BinaryExpr:
  if n.Op!=token.ADD&&n.Op!=token.SUB&&n.Op!=token.MUL{return nil,fmt.Errorf("unsupported binary %s",n.Op)}
  l,err:=astExpression(n.X,vars)
  if err!=nil{return nil,err}
  r,err:=astExpression(n.Y,vars)
  if err!=nil{return nil,err}
  if n.Op==token.ADD{return astAdd(l,r,false),nil}
  if n.Op==token.SUB{return astAdd(l,r,true),nil}
  return astMultiply(l,r)
 default:return nil,fmt.Errorf("unsupported expression %T",expr)
 }
}
func astPolyString(p astPolynomial)string{
 if len(p)==0{return "0"}
 keys:=make([]astMonomial,0,len(p))
 for k:=range p{keys=append(keys,k)}
 sort.Slice(keys,func(i,j int)bool{if keys[i].a!=keys[j].a{return keys[i].a<keys[j].a};return keys[i].b<keys[j].b})
 var out strings.Builder
 for _,k:=range keys{
  out.WriteString("(");out.WriteString(strconv.Itoa(int(k.a)));out.WriteString(",")
  out.WriteString(strconv.Itoa(int(k.b)));out.WriteString(")=");out.WriteString(p[k].String());out.WriteString(";")
 }
 return out.String()
}
func astCondition(expr ast.Expr,vars map[string]astMonomial)(string,error){
 op,ok:=expr.(*ast.BinaryExpr)
 if !ok{return "",fmt.Errorf("expected binary comparison")}
 l,err:=astExpression(op.X,vars)
 if err!=nil{return "",err}
 r,err:=astExpression(op.Y,vars)
 if err!=nil{return "",err}
 a,b:=astPolyString(l),astPolyString(r)
 switch op.Op{
 case token.GTR:return "GT("+a+"|"+b+")",nil
 case token.LSS:return "GT("+b+"|"+a+")",nil
 case token.GEQ:return "GE("+a+"|"+b+")",nil
 case token.LEQ:return "GE("+b+"|"+a+")",nil
 case token.EQL,token.NEQ:
  if b<a{a,b=b,a}
  return op.Op.String()+"("+a+"|"+b+")",nil
 default:return "",fmt.Errorf("unsupported condition %s",op.Op)
 }
}
func astWithoutEmpty(stmts []ast.Stmt)[]ast.Stmt{
 result:=make([]ast.Stmt,0,len(stmts))
 for _,stmt:=range stmts{if _,ok:=stmt.(*ast.EmptyStmt);!ok{result=append(result,stmt)}}
 return result
}
func astReturn(stmt ast.Stmt,vars map[string]astMonomial)(string,error){
 r,ok:=stmt.(*ast.ReturnStmt)
 if !ok||len(r.Results)!=1{return "",fmt.Errorf("expected one return expression")}
 p,err:=astExpression(r.Results[0],vars)
 if err!=nil{return "",err}
 return "RET{"+astPolyString(p)+"}",nil
}
func astCanonicalBody(stmts []ast.Stmt,vars map[string]astMonomial)(string,error){
 stmts=astWithoutEmpty(stmts)
 if len(stmts)==1{return astReturn(stmts[0],vars)}
 if len(stmts)!=2{return "",fmt.Errorf("only direct returns or if-return followed by return")}
 condition,ok:=stmts[0].(*ast.IfStmt)
 if !ok||condition.Init!=nil||condition.Else!=nil{return "",fmt.Errorf("unsupported control flow")}
 cond,err:=astCondition(condition.Cond,vars)
 if err!=nil{return "",err}
 branch:=astWithoutEmpty(condition.Body.List)
 if len(branch)!=1{return "",fmt.Errorf("if branch must return exactly once")}
 yes,err:=astReturn(branch[0],vars);if err!=nil{return "",err}
 no,err:=astReturn(stmts[1],vars);if err!=nil{return "",err}
 return "IF{"+cond+"?"+yes+":"+no+"}",nil
}

// False means only "not certified equivalent"; never infer a functional
// change from a mismatched normal form without an independent test witness.
func ASTConservativeEquivalent(a,b string)(bool,error){
 ak,err:=ASTCanonicalIntFunction(a)
 if err!=nil{return false,err}
 bk,err:=ASTCanonicalIntFunction(b)
 if err!=nil{return false,err}
 return ak==bk,nil
}
