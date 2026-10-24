package hdccsr

import (
    "encoding/binary"
    "encoding/json"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func makeRealASTFixture(t *testing.T) (*RealASTManifest,string) {
    t.Helper()
    root:=t.TempDir()
    code:=`package demo
func Handler() { Service() }
func Service() { Store() }
func Store() {}
`
    if err:=os.WriteFile(filepath.Join(root,"demo.go"),[]byte(code),0644);err!=nil{t.Fatal(err)}
    manifest,err:=ExportRealAST(root)
    if err!=nil{t.Fatal(err)}
    return manifest,root
}

func TestExportRealASTAlignedSymbols(t *testing.T) {
    m,_:=makeRealASTFixture(t)
    if len(m.Symbols)!=3 {t.Fatalf("expected 3 real symbols, got %d",len(m.Symbols))}
    for i,s:=range m.Symbols {
        if s.ID!=uint32(i)||s.Text==""||!strings.HasSuffix(s.File,"demo.go"){
            t.Fatalf("misaligned real AST record %d: %+v",i,s)
        }
    }
    if m.Symbols[0].Name!="Handler"||m.Symbols[1].Name!="Service"||m.Symbols[2].Name!="Store" {
        t.Fatalf("wrong ID mapping: %v %v %v",m.Symbols[0].Name,m.Symbols[1].Name,m.Symbols[2].Name)
    }
    if len(m.Edges)!=2||m.Edges[0]!=1||m.Edges[1]!=2||
        m.Relations[0]!=RelCall||m.Relations[1]!=RelCall {
        t.Fatalf("CSR call graph mismatch: edges=%v relations=%v",m.Edges,m.Relations)
    }
    path:=filepath.Join(t.TempDir(),"manifest.json")
    if err:=WriteRealASTManifest(path,m);err!=nil{t.Fatal(err)}
    roundTrip,err:=ReadRealASTManifest(path)
    if err!=nil{t.Fatal(err)}
    if roundTrip.SourceDigest!=m.SourceDigest {t.Fatal("source digest changed")}
    roundTrip.Symbols[0].ID=999
    buf,_:=json.Marshal(roundTrip)
    if err=os.WriteFile(path,buf,0644);err!=nil{t.Fatal(err)}
    if _,err=ReadRealASTManifest(path);err==nil{t.Fatal("corrupt IDs accepted")}
}

func TestLoadRealCorpusRequiresValidReceiptAndIndex(t *testing.T) {
    m,_:=makeRealASTFixture(t)
    dir:=t.TempDir()
    manifestPath:=filepath.Join(dir,"real_manifest.json")
    indexPath:=filepath.Join(dir,"vectors.bin")
    receiptPath:=filepath.Join(dir,"receipt.json")
    if err:=WriteRealASTManifest(manifestPath,m);err!=nil{t.Fatal(err)}
    n:=len(m.Symbols)
    bin:=make([]byte,64+1024*n)
    copy(bin[:8],[]byte("MBVM8K1\x00"))
    binary.LittleEndian.PutUint32(bin[8:12],2)
    binary.LittleEndian.PutUint32(bin[12:16],uint32(n))
    binary.LittleEndian.PutUint16(bin[16:18],8192)
    for i:=0;i<n;i++ {binary.LittleEndian.PutUint64(bin[64+i*1024:],uint64(100+i))}
    if err:=os.WriteFile(indexPath,bin,0644);err!=nil{t.Fatal(err)}
    manSHA,_:=sha256File(manifestPath)
    idxSHA,_:=sha256File(indexPath)
    receipt:=RealExportReceipt{
        Schema:"mbvm-real-vectors-v1",SourceDigest:m.SourceDigest,
        ManifestSHA256:manSHA,IndexSHA256:idxSHA,VectorCount:n,
        VectorBits:8192,IndexVersion:2,BitOrder:"little",
        RealGoAST:true,ProjectionTrained:true,
    }
    receiptBytes,_:=json.Marshal(receipt)
    if err:=os.WriteFile(receiptPath,receiptBytes,0644);err!=nil{t.Fatal(err)}
    corpus,err:=LoadRealCorpus(manifestPath,indexPath,receiptPath)
    if err!=nil{t.Fatal(err)}
    if corpus.Graph.TotalNodes!=n||corpus.Vectors[1][0]!=101 {
        t.Fatalf("real data load mismatch")
    }
    // Failure gates: wrong binary checksum and missing verifiable training receipt.
    bin[64]^=0x01
    if err:=os.WriteFile(indexPath,bin,0644);err!=nil{t.Fatal(err)}
    if _,err=LoadRealCorpus(manifestPath,indexPath,receiptPath);err==nil{
        t.Fatal("tampered vector artifact passed SHA-256 validation")
    }
    if err:=os.Remove(receiptPath);err!=nil{t.Fatal(err)}
    if _,err=LoadRealCorpus(manifestPath,indexPath,receiptPath);err==nil{
        t.Fatal("unverified corpus accepted")
    }
}
