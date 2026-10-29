package hdccsr

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

// RealReverseCallsCSR contains REVERSED RelCall edges from the real
// Go AST exporter. The exporter resolves names heuristically; this is a
// conservative "possible callers" navigation aid, not a Go compiler
// or a type-resolved complete call graph.
type RealReverseCallsCSR struct {
	Offsets []uint32
	Callers []uint32
	Nodes int
	Edges int
}

func BuildRealReverseCallsCSR(m *RealASTManifest) (*RealReverseCallsCSR,error) {
	if m==nil{return nil,fmt.Errorf("nil AST manifest")}
	n:=len(m.Symbols)
	if n==0||len(m.Offsets)!=n+1||len(m.Edges)!=len(m.Relations) {
		return nil,fmt.Errorf("invalid real AST CSR layout")
	}
	if m.Offsets[0]!=0||int(m.Offsets[n])!=len(m.Edges){
		return nil,fmt.Errorf("invalid real AST CSR offsets")
	}
	degrees:=make([]uint32,n)
	for src:=0;src<n;src++ {
		if m.Symbols[src].ID!=uint32(src) || m.Offsets[src]>m.Offsets[src+1] || int(m.Offsets[src+1])>len(m.Edges){
			return nil,fmt.Errorf("bad symbol id or offset at %d",src)
		}
		for e:=m.Offsets[src];e<m.Offsets[src+1];e++ {
			tgt:=int(m.Edges[e])
			if tgt<0||tgt>=n{return nil,fmt.Errorf("edge %d out of range",e)}
			if m.Relations[e]==RelCall&&tgt!=src {
				degrees[tgt]++
			}
		}
	}
	rev:=&RealReverseCallsCSR{Offsets:make([]uint32,n+1),Nodes:n}
	for id,count:=range degrees {
		rev.Offsets[id+1]=rev.Offsets[id]+count
	}
	rev.Callers=make([]uint32,rev.Offsets[n])
	rev.Edges=len(rev.Callers)
	cursor:=make([]uint32,n)
	copy(cursor,rev.Offsets[:n])
	for src:=0;src<n;src++ {
		for e:=m.Offsets[src];e<m.Offsets[src+1];e++ {
			tgt:=int(m.Edges[e])
			if m.Relations[e]!=RelCall||tgt==src{continue}
			k:=cursor[tgt]
			rev.Callers[k]=uint32(src)
			cursor[tgt]++
		}
	}
	return rev,nil
}

// RealImpactScratch is allocated once per query-worker. A caller must NOT
// share scratch across goroutines. The stamp-based BFS avoids clearing the
// visited array for every query and does not allocate in its hot path.
type RealImpactScratch struct {
	seen []uint32
	queue []uint32
	stamp uint32
}
func NewRealImpactScratch(nodes int)*RealImpactScratch{
	return &RealImpactScratch{seen:make([]uint32,nodes),queue:make([]uint32,nodes)}
}

type RealImpactReach struct{
	ImmediateCallers int
	TransitiveCallers int
	AllCallers int
	MaxDepth int
}

func (g *RealReverseCallsCSR) Impact(target int,s *RealImpactScratch)(RealImpactReach,error){
	var result RealImpactReach
	if g==nil||s==nil||target<0||target>=g.Nodes||
		len(s.seen)<g.Nodes||len(s.queue)<g.Nodes{return result,fmt.Errorf("invalid reverse query or scratch")}
	if s.stamp==^uint32(0) {
		clear(s.seen)
		s.stamp=0
	}
	s.stamp++
	tick:=s.stamp
	head,tail:=0,1
	s.queue[0]=uint32(target)
	s.seen[target]=tick
	depth:=0
	for head<tail {
		levelEnd:=tail
		for head<levelEnd {
			id:=int(s.queue[head]);head++
			for e:=g.Offsets[id];e<g.Offsets[id+1];e++{
				caller:=int(g.Callers[e])
				if s.seen[caller]==tick{continue}
				s.seen[caller]=tick
				s.queue[tail]=uint32(caller);tail++
				if depth==0{result.ImmediateCallers++}else{result.TransitiveCallers++}
			}
		}
		if tail>levelEnd{result.MaxDepth=depth+1}
		depth++
	}
	result.AllCallers=tail-1
	return result,nil
}

// RealContractMutation is a *structural* signature change to an actual
// declaration from the real repository. It is not a behavioral witness:
// the original package must be rebuilt/tested to establish observable breakage.
type RealContractMutation struct {
	SymbolID uint32
	SymbolName string
	OriginalType string
	ChangedType string
	FormattingStable bool
	Kind string
}

func realFuncFromText(text string)(*token.FileSet,*ast.FuncDecl,error) {
	fset:=token.NewFileSet()
	f,err:=parser.ParseFile(fset,"real_declaration.go","package realimpact\n"+text,0)
	if err!=nil{return nil,nil,err}
	if len(f.Decls)!=1{return nil,nil,fmt.Errorf("expected one declaration")}
	fn,ok:=f.Decls[0].(*ast.FuncDecl)
	if !ok||fn.Type==nil||fn.Type.Params==nil{return nil,nil,fmt.Errorf("not a function declaration")}
	return fset,fn,nil
}
func realTypeString(fset *token.FileSet,fn *ast.FuncDecl)(string,error){
	var buf bytes.Buffer
	if err:=format.Node(&buf,fset,fn.Type);err!=nil{return "",err}
	return buf.String(),nil
}

func RealMutationOfDeclaration(symbol RealASTSymbol)(RealContractMutation,error){
	var info RealContractMutation
	if symbol.Kind!="func"{return info,fmt.Errorf("symbol %q is not a function",symbol.Name)}
	fset,fn,err:=realFuncFromText(symbol.Text)
	if err!=nil{return info,err}
	before,err:=realTypeString(fset,fn)
	if err!=nil{return info,err}

	// Round-trip through gofmt: a formatting-only transformation that
	// preserves the declaration syntax after canonical printing.
	formatted,err:=format.Source([]byte("package realimpact\n"+symbol.Text))
	if err!=nil{return info,err}
	formattedFset:=token.NewFileSet()
	parsed,err:=parser.ParseFile(formattedFset,"formatted.go",formatted,0)
	if err!=nil{return info,err}
	if len(parsed.Decls)!=1{return info,fmt.Errorf("unexpected formatted declarations")}
	ff,ok:=parsed.Decls[0].(*ast.FuncDecl)
	if !ok{return info,fmt.Errorf("formatted declaration is not a function")}
	afterFmt,err:=realTypeString(formattedFset,ff)
	if err!=nil{return info,err}
	// No lexical guess about alpha-renaming. For arbitrary repository
	// functions with dependencies, certifying alpha-equivalence requires
	// name resolution and type checking of the *whole* package.
	if afterFmt!=before{return info,fmt.Errorf("format-only signature changed")}

	// Introduce one *required* int parameter, a syntactic contract change.
	// Never call it a behavioral witness or a validated compile failure.
	for _,p:=range fn.Type.Params.List {
		if _,variadic:=p.Type.(*ast.Ellipsis);variadic{
			return info,fmt.Errorf("cannot append a required parameter after variadic")
		}
		for _,n:=range p.Names {
			if n.Name=="__mbvm_contract_probe"{
				return info,fmt.Errorf("contract probe name collides")
			}
		}
	}
	fn.Type.Params.List=append(fn.Type.Params.List,&ast.Field{
		Names:[]*ast.Ident{ast.NewIdent("__mbvm_contract_probe")},
		Type:ast.NewIdent("int"),
	})
	after,err:=realTypeString(fset,fn)
	if err!=nil{return info,err}
	if after==before{return info,fmt.Errorf("contract mutation did not change signature")}
	return RealContractMutation{
		SymbolID:symbol.ID,SymbolName:symbol.Name,
		OriginalType:before,ChangedType:after,
		FormattingStable:true,Kind:"required_parameter_added",
	},nil
}

// Select a real, parseable function with the most immediate inbound
// resolved call edges. Selection uses only observed AST graph data.
func SelectRealImpactTarget(m *RealASTManifest,g *RealReverseCallsCSR)(int,error){
	if m==nil||g==nil||len(m.Symbols)!=g.Nodes{return -1,fmt.Errorf("incompatible manifest and graph")}
	best,bestDegree:=-1,0
	for id,s:=range m.Symbols{
		if s.Kind!="func"{continue}
		d:=int(g.Offsets[id+1]-g.Offsets[id])
		if d<=bestDegree{continue}
		if _,err:=RealMutationOfDeclaration(s);err!=nil{continue}
		best,bestDegree=id,d
	}
	if best<0{return -1,fmt.Errorf("no parseable real function has a resolved inbound call")}
	return best,nil
}
