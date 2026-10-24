package hdccsr

import (
    "crypto/sha256"
    "encoding/binary"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io"
    "os"

    "github.com/kbelludoo/mbvm-db/pkg/mbvmdb"
)

type RealExportReceipt struct {
    Schema string `json:"schema"`
    SourceDigest string `json:"source_digest"`
    ManifestSHA256 string `json:"manifest_sha256"`
    IndexSHA256 string `json:"index_sha256"`
    VectorCount int `json:"vector_count"`
    VectorBits int `json:"vector_bits"`
    IndexVersion uint32 `json:"index_version"`
    BitOrder string `json:"bit_order"`
    RealGoAST bool `json:"real_go_ast"`
    ProjectionTrained bool `json:"projection_trained"`
}

type RealCorpus struct {
    Manifest *RealASTManifest
    Receipt RealExportReceipt
    Graph *ExpCSRGraph
    Vectors []Hypervector
}

func sha256File(path string)(string,error){
    f,err:=os.Open(path)
    if err!=nil{return "",err}
    defer f.Close()
    h:=sha256.New()
    if _,err=io.Copy(h,f);err!=nil{return "",err}
    return hex.EncodeToString(h.Sum(nil)),nil
}

// LoadRealCorpus requires a matching AST export, trained-projection receipt,
// and MBVM8K1 version 2 binary. It rejects any missing or synthetic artifact.
// All IDs and CSR adjacency are aligned to the original AST symbol ordering.
func LoadRealCorpus(manifestPath,indexPath,receiptPath string)(*RealCorpus,error){
    manifest,err:=ReadRealASTManifest(manifestPath)
    if err!=nil{return nil,err}
    receiptBytes,err:=os.ReadFile(receiptPath)
    if err!=nil{return nil,err}
    var receipt RealExportReceipt
    if err=json.Unmarshal(receiptBytes,&receipt);err!=nil{return nil,err}
    n:=len(manifest.Symbols)
    if receipt.Schema!="mbvm-real-vectors-v1"||!receipt.RealGoAST||!receipt.ProjectionTrained||
        receipt.BitOrder!="little"||receipt.IndexVersion!=2||
        receipt.VectorBits!=VectorBits||receipt.VectorCount!=n||
        receipt.SourceDigest!=manifest.SourceDigest{
        return nil,fmt.Errorf("receipt does not attest aligned trained Go-AST 8192-bit vectors")
    }
    sourceHash,err:=sha256File(manifestPath)
    if err!=nil{return nil,err}
    if receipt.ManifestSHA256!=sourceHash {return nil,fmt.Errorf("manifest SHA-256 mismatch")}
    indexHash,err:=sha256File(indexPath)
    if err!=nil{return nil,err}
    if receipt.IndexSHA256!=indexHash {return nil,fmt.Errorf("vector index SHA-256 mismatch")}

    f,err:=os.Open(indexPath)
    if err!=nil{return nil,err}
    header:=make([]byte,64)
    _,err=io.ReadFull(f,header)
    stat,statErr:=f.Stat()
    _=f.Close()
    if err!=nil{return nil,err}
    if statErr!=nil{return nil,statErr}
    if string(header[:8])!=mbvmdb.NeuralMagic8K {
        return nil,fmt.Errorf("wrong neural magic; expected MBVM8K1")
    }
    if binary.LittleEndian.Uint32(header[8:12])!=2||
        int(binary.LittleEndian.Uint32(header[12:16]))!=n||
        int(binary.LittleEndian.Uint16(header[16:18]))!=VectorBits {
        return nil,fmt.Errorf("index binary header version/count/dimensions mismatch")
    }
    if stat.Size()!=int64(64+1024*n) {
        return nil,fmt.Errorf("index size mismatch: got %d want %d",stat.Size(),64+1024*n)
    }
    index,err:=mbvmdb.LoadNeuralIndexFromFile(indexPath)
    if err!=nil{return nil,err}
    if int(index.TotalSymbols)!=n{return nil,fmt.Errorf("loaded index count mismatch")}
    original:=make([]Hypervector,n)
    for id:=0;id<n;id++ {
        words:=index.GetVector(uint32(id))
        if len(words)!=WordsPerVector{return nil,fmt.Errorf("bad vector at ID %d",id)}
        copy(original[id][:],words)
    }
    graph:=&ExpCSRGraph{
        TotalNodes:n,
        Offsets:manifest.Offsets,
        Edges:manifest.Edges,
        Relations:manifest.Relations,
    }
    return &RealCorpus{Manifest:manifest,Receipt:receipt,Graph:graph,Vectors:original},nil
}
