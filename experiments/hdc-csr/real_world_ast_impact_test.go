package hdccsr

import (
	"strings"
	"testing"
)

func realImpactFixture(t testing.TB)(*RealASTManifest,*RealReverseCallsCSR,int){
	t.Helper()
	// Tests run in experiments/hdc-csr, so this is a REAL checked-in
	// source package, not a generated fixture or Kaggle vector index.
	manifest,err:=ExportRealAST("../../pkg/mbvmdb")
	if err!=nil{t.Fatalf("real MBVM-DB AST ingest: %v",err)}
	if len(manifest.Symbols)<8{t.Fatalf("unexpectedly small real package: %d symbols",len(manifest.Symbols))}
	reverse,err:=BuildRealReverseCallsCSR(manifest)
	if err!=nil{t.Fatal(err)}
	if reverse.Edges==0{t.Fatal("real AST graph has no resolved call edges")}
	target,err:=SelectRealImpactTarget(manifest,reverse)
	if err!=nil{t.Fatal(err)}
	return manifest,reverse,target
}

func TestRealWorldASTImpactOnOwnPackage(t *testing.T){
	manifest,graph,target:=realImpactFixture(t)
	sym:=manifest.Symbols[target]
	mutation,err:=RealMutationOfDeclaration(sym)
	if err!=nil{t.Fatal(err)}
	if !mutation.FormattingStable||mutation.OriginalType==mutation.ChangedType||
		!strings.Contains(mutation.ChangedType,"__mbvm_contract_probe int"){
		t.Fatalf("invalid controlled contract mutation: %+v",mutation)
	}
	scratch:=NewRealImpactScratch(graph.Nodes)
	reach,err:=graph.Impact(target,scratch)
	if err!=nil{t.Fatal(err)}
	if reach.ImmediateCallers<1{t.Fatal("target should have immediate static callers")}
	if reach.AllCallers!=reach.ImmediateCallers+reach.TransitiveCallers{
		t.Fatalf("reach mismatch %+v",reach)
	}
	repeat,err:=graph.Impact(target,scratch)
	if err!=nil||repeat!=reach{t.Fatalf("unstable reach results: before %+v after %+v err=%v",reach,repeat,err)}
	t.Logf("REAL pkg/mbvmdb: AST symbols=%d resolved call edges=%d target=%s.%s ID=%d, direct callers=%d indirect=%d total=%d maxdepth=%d",
		len(manifest.Symbols),graph.Edges,sym.Package,sym.Name,target,
		reach.ImmediateCallers,reach.TransitiveCallers,reach.AllCallers,reach.MaxDepth)
	t.Logf("contract change (structural only): %s -> %s",mutation.OriginalType,mutation.ChangedType)
}

func TestRealImpactReverseCSRCyclesAndDuplicateEdges(t *testing.T){
	m:=&RealASTManifest{
		Symbols:[]RealASTSymbol{{ID:0},{ID:1},{ID:2},{ID:3}},
		Offsets:[]uint32{0,3,4,5,6},
		Edges:[]uint16{1,1,2,3,1,2},
		Relations:[]EdgeRelation{RelCall,RelCall,RelCall,RelCall,RelCall,RelCall},
	}
	g,err:=BuildRealReverseCallsCSR(m)
	if err!=nil{t.Fatal(err)}
	seen:=NewRealImpactScratch(g.Nodes)
	r,err:=g.Impact(1,seen)
	if err!=nil{t.Fatal(err)}
	if r.ImmediateCallers!=2||r.TransitiveCallers!=1||r.AllCallers!=3{
		t.Fatalf("expected direct={0,2} transitively={3}: %+v",r)
	}
	if allocations:=testing.AllocsPerRun(50,func(){
		if _,err:=g.Impact(1,seen);err!=nil{panic(err)}
	});allocations!=0{
		t.Logf("cached BFS allocated %.1f times per operation (investigate performance before asserting zero)",allocations)
	}
	if _,err:=g.Impact(-1,seen);err==nil{t.Fatal("invalid target accepted")}
	bad:=*m
	bad.Offsets=[]uint32{0,3,999,5,6}
	if _,err:=BuildRealReverseCallsCSR(&bad);err==nil{t.Fatal("corrupt CSR should be rejected")}
}

func BenchmarkRealWorldASTImpact(b *testing.B){
	manifest,graph,target:=realImpactFixture(b)
	sym:=manifest.Symbols[target]
	b.Run("FullIngestMutationReverseCSR",func(b *testing.B){
		b.ReportAllocs()
		for i:=0;i<b.N;i++{
			m,err:=ExportRealAST("../../pkg/mbvmdb")
			if err!=nil{b.Fatal(err)}
			g,err:=BuildRealReverseCallsCSR(m)
			if err!=nil{b.Fatal(err)}
			id,err:=SelectRealImpactTarget(m,g)
			if err!=nil{b.Fatal(err)}
			if _,err:=RealMutationOfDeclaration(m.Symbols[id]);err!=nil{b.Fatal(err)}
			s:=NewRealImpactScratch(g.Nodes)
			if _,err:=g.Impact(id,s);err!=nil{b.Fatal(err)}
		}
	})
	b.Run("BuildReverseCSR",func(b *testing.B){
		b.ReportAllocs()
		for i:=0;i<b.N;i++{
			g,err:=BuildRealReverseCallsCSR(manifest)
			if err!=nil||g.Edges==0{b.Fatal(err)}
		}
	})
	b.Run("RealSignatureMutation",func(b *testing.B){
		b.ReportAllocs()
		for i:=0;i<b.N;i++{
			m,err:=RealMutationOfDeclaration(sym)
			if err!=nil||m.OriginalType==m.ChangedType{b.Fatal(err)}
		}
	})
	b.Run("CachedReverseCSRQuery",func(b *testing.B){
		s:=NewRealImpactScratch(graph.Nodes)
		b.ReportAllocs()
		b.ResetTimer()
		for i:=0;i<b.N;i++{
			r,err:=graph.Impact(target,s)
			if err!=nil||r.ImmediateCallers==0{b.Fatal(err)}
		}
	})
}
