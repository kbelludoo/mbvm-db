package hdccsr

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/bits"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"testing"
)

// This lab separates three things:
//   (1) bit corruption of an existing hypervector,
//   (2) bit ERASURE with a KNOWN validity mask,
//   (3) independently checked functional equivalence of Go snippets.
// A neural claim requires inference with a held-out TRAINED checkpoint;
// the tests in this file deliberately do not pretend to provide one.

type semanticSensitivityFixture struct {
	Schema string `json:"schema"`
	Cases []struct {
		ID string `json:"id"`
		Original string `json:"original"`
		Equivalent string `json:"equivalent"`
		Changed string `json:"changed"`
	} `json:"cases"`
}

func readSemanticSensitivityFixtures(t *testing.T) semanticSensitivityFixture {
	t.Helper()
	data, err := os.ReadFile("semantic_sensitivity_cases.json")
	if err != nil { t.Fatal(err) }
	var cases semanticSensitivityFixture
	if err := json.Unmarshal(data, &cases); err != nil { t.Fatal(err) }
	if cases.Schema != "mbvm-semantic-sensitivity-v1" || len(cases.Cases)<3 {
		t.Fatal("missing valid semantic sensitivity fixtures")
	}
	return cases
}

func parseArithmeticFunction(src string) (*ast.FuncDecl, error) {
	f, err := parser.ParseFile(token.NewFileSet(),"snippet.go","package semanticlab\n"+src,0)
	if err != nil {return nil,err}
	for _,decl:=range f.Decls {
		if fn,ok:=decl.(*ast.FuncDecl);ok {return fn,nil}
	}
	return nil,fmt.Errorf("no function declaration")
}

func evalArithmeticExpr(e ast.Expr,vars map[string]int)(int,error){
	switch x:=e.(type){
	case *ast.Ident:
		v,ok:=vars[x.Name]
		if !ok {return 0,fmt.Errorf("unknown identifier %q",x.Name)}
		return v,nil
	case *ast.BasicLit:
		if x.Kind!=token.INT{return 0,fmt.Errorf("unsupported literal kind")}
		return strconv.Atoi(x.Value)
	case *ast.ParenExpr:
		return evalArithmeticExpr(x.X,vars)
	case *ast.UnaryExpr:
		v,err:=evalArithmeticExpr(x.X,vars)
		if err!=nil{return 0,err}
		if x.Op==token.SUB{return -v,nil}
		if x.Op==token.ADD{return v,nil}
		return 0,fmt.Errorf("unsupported unary op %v",x.Op)
	case *ast.BinaryExpr:
		l,err:=evalArithmeticExpr(x.X,vars)
		if err!=nil{return 0,err}
		r,err:=evalArithmeticExpr(x.Y,vars)
		if err!=nil{return 0,err}
		switch x.Op{
		case token.ADD:return l+r,nil
		case token.SUB:return l-r,nil
		case token.MUL:return l*r,nil
		default:return 0,fmt.Errorf("unsupported binary op %v",x.Op)
		}
	default:return 0,fmt.Errorf("unsupported AST expression %T",e)
	}
}

func evalArithmeticCondition(expr ast.Expr,vars map[string]int)(bool,error) {
	x,ok:=expr.(*ast.BinaryExpr)
	if !ok{return false,fmt.Errorf("unsupported condition %T",expr)}
	l,err:=evalArithmeticExpr(x.X,vars)
	if err!=nil{return false,err}
	r,err:=evalArithmeticExpr(x.Y,vars)
	if err!=nil{return false,err}
	switch x.Op {
	case token.GTR:return l>r,nil
	case token.LSS:return l<r,nil
	case token.GEQ:return l>=r,nil
	case token.LEQ:return l<=r,nil
	case token.EQL:return l==r,nil
	case token.NEQ:return l!=r,nil
	default:return false,fmt.Errorf("unsupported condition op %v",x.Op)
	}
}

func evalArithmeticStmts(stmts []ast.Stmt,vars map[string]int)(int,bool,error) {
	for _,stmt:=range stmts {
		switch x:=stmt.(type){
		case *ast.ReturnStmt:
			if len(x.Results)!=1{return 0,false,fmt.Errorf("expected one return expression")}
			v,err:=evalArithmeticExpr(x.Results[0],vars)
			return v,err==nil,err
		case *ast.EmptyStmt:
			continue
		case *ast.IfStmt:
			if x.Init!=nil{return 0,false,fmt.Errorf("if initializer unsupported")}
			cond,err:=evalArithmeticCondition(x.Cond,vars)
			if err!=nil{return 0,false,err}
			if cond {
				if v,returned,err:=evalArithmeticStmts(x.Body.List,vars);returned||err!=nil{return v,returned,err}
			} else if x.Else!=nil {
				if block,ok:=x.Else.(*ast.BlockStmt);ok {
					if v,returned,err:=evalArithmeticStmts(block.List,vars);returned||err!=nil{return v,returned,err}
				} else{return 0,false,fmt.Errorf("unsupported else clause")}
			}
		default:
			return 0,false,fmt.Errorf("unsupported statement %T",stmt)
		}
	}
	return 0,false,nil
}

func evalArithmeticFunction(fn *ast.FuncDecl,a,b int)(int,error) {
	if fn.Recv!=nil || fn.Type.Params==nil{return 0,fmt.Errorf("unsupported function receiver/params")}
	names:=make([]string,0,2)
	for _,f:=range fn.Type.Params.List {
		for _,n:=range f.Names {names=append(names,n.Name)}
	}
	if len(names)!=2{return 0,fmt.Errorf("expected exactly two named parameters")}
	vars:=map[string]int{names[0]:a,names[1]:b}
	v,returned,err:=evalArithmeticStmts(fn.Body.List,vars)
	if err!=nil{return 0,err}
	if !returned{return 0,fmt.Errorf("function returned no value")}
	return v,nil
}

// Real behavior labels are checked by parsing and evaluating the tiny,
// explicit Go integer subset. The test does NOT claim full Go equivalence:
// input domain and AST grammar are intentionally bounded and declared.
func TestSemanticSensitivityFunctionChangeGroundTruth(t *testing.T) {
	fixtures:=readSemanticSensitivityFixtures(t)
	// Training-range + OOD numbers, shared by all three versions.
	inputs:=[]int{-100,-30,-11,-10,-7,-2,-1,0,1,2,7,10,11,30,100}
	for _,c:=range fixtures.Cases {
		orig,err:=parseArithmeticFunction(c.Original)
		if err!=nil{t.Fatalf("%s original: %v",c.ID,err)}
		equivalent,err:=parseArithmeticFunction(c.Equivalent)
		if err!=nil{t.Fatalf("%s equivalent: %v",c.ID,err)}
		changed,err:=parseArithmeticFunction(c.Changed)
		if err!=nil{t.Fatalf("%s changed: %v",c.ID,err)}
		different:=0
		for _,a:=range inputs {
			for _,b:=range inputs {
				v,err:=evalArithmeticFunction(orig,a,b)
				if err!=nil{t.Fatal(err)}
				eq,err:=evalArithmeticFunction(equivalent,a,b)
				if err!=nil{t.Fatal(err)}
				other,err:=evalArithmeticFunction(changed,a,b)
				if err!=nil{t.Fatal(err)}
				if eq!=v{t.Fatalf("%s equivalent implementation disagrees for (%d,%d): %d vs %d",c.ID,a,b,v,eq)}
				if other!=v{different++}
			}
		}
		if different==0{t.Fatalf("%s changed example did not differ on any checked inputs",c.ID)}
		t.Logf("functional fixture %s: equivalent 225/225; changed differs on %d/225 inputs",c.ID,different)
	}
}

type sensitivityNeighbor struct{
	id uint16
	dist int
}

func sensitivityTop10(query Hypervector,orig []Hypervector,src int,valid *Hypervector) []uint16 {
	candidates:=make([]sensitivityNeighbor,0,len(orig)-1)
	for i,v:=range orig {
		if i==src{continue}
		d:=0
		for j:=0;j<WordsPerVector;j++ {
			x:=query[j]^v[j]
			if valid!=nil {x&=valid[j]}
			d+=bits.OnesCount64(x)
		}
		candidates=append(candidates,sensitivityNeighbor{uint16(i),d})
	}
	sort.Slice(candidates,func(i,j int)bool{
		if candidates[i].dist!=candidates[j].dist{return candidates[i].dist<candidates[j].dist}
		return candidates[i].id<candidates[j].id
	})
	result:=make([]uint16,10)
	for i:=0;i<10;i++{result[i]=candidates[i].id}
	return result
}

func sensitivityOverlap10(reference,observed []uint16)float64{
	set:=make(map[uint16]bool,len(reference))
	for _,id:=range reference{set[id]=true}
	hits:=0
	for _,id:=range observed{if set[id]{hits++}}
	return float64(hits)/10.0
}

// The reference is the CLEAN exact Hamming Top-10, not artificial ground
// truth for semantic relevance. Every corruption setting uses exactly the
// same 12 queries and 4 independent noise masks per query. This is genuine
// Top-10 overlap: the first 10 retrieved candidates, never first 64.
func TestSemanticSensitivityBitFlipVsKnownErasure(t *testing.T){
	f:=buildMultipathFixture(8080,1000)
	rates:=[]float64{0,0.05,0.1,0.2,0.4}
	const queries=12
	const seeds=4
	cleanTop:=make([][]uint16,queries)
	for q:=0;q<queries;q++{cleanTop[q]=sensitivityTop10(f.vectors[q],f.vectors,q,nil)}
	for _,rate:=range rates {
		flipRecall,eraseRecall:=0.0,0.0
		n:=int(rate*float64(VectorBits))
		for q:=0;q<queries;q++{
			for trial:=0;trial<seeds;trial++ {
				rng:=rand.New(rand.NewSource(int64(2026+7919*q+104729*trial)))
				positions:=rng.Perm(VectorBits)
				corrupt:=f.vectors[q]
				var valid Hypervector
				for w:=range valid {valid[w]=^uint64(0)}
				for i:=0;i<n;i++ {
					pos:=positions[i]
					corrupt[pos/64]^=uint64(1)<<uint(pos%64)
					valid[pos/64]&^=uint64(1)<<uint(pos%64)
				}
				flip:=sensitivityTop10(corrupt,f.vectors,q,nil)
				erased:=sensitivityTop10(f.vectors[q],f.vectors,q,&valid)
				flipRecall+=sensitivityOverlap10(cleanTop[q],flip)
				eraseRecall+=sensitivityOverlap10(cleanTop[q],erased)
			}
		}
		flipRecall/=queries*seeds
		eraseRecall/=queries*seeds
		t.Logf("noise %.0f%% (%d bits): exact Top10 agreement with clean Hamming: flip %.2f%%; known erasure %.2f%%",rate*100,n,flipRecall*100,eraseRecall*100)
		if rate==0 && (flipRecall!=1||eraseRecall!=1) {
			t.Fatal("no-noise reference must be 100%")
		}
	}
}
