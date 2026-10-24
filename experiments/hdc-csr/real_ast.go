package hdccsr

import (
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "go/ast"
    "go/parser"
    "go/printer"
    "go/token"
    "io/fs"
    "os"
    "path/filepath"
    "strings"

    "github.com/kbelludoo/mbvm-db/pkg/mbvmdb"
)

// RealASTManifest makes the AST text/graph/ID correspondence explicit.
// It mirrors MBVM-DB's current outgoing CSR; not a type-resolved Go call graph.
type RealASTManifest struct {
    Schema string `json:"schema"`
    SourceRoot string `json:"source_root"`
    SourceDigest string `json:"source_digest"`
    Symbols []RealASTSymbol `json:"symbols"`
    Offsets []uint32 `json:"offsets"`
    Edges []uint16 `json:"edges"`
    Relations []EdgeRelation `json:"relations"`
}
type RealASTSymbol struct {
    ID uint32 `json:"id"`
    Kind string `json:"kind"`
    Package string `json:"package"`
    Name string `json:"name"`
    Receiver string `json:"receiver,omitempty"`
    Signature string `json:"signature,omitempty"`
    File string `json:"file"`
    Text string `json:"text"`
    TextSHA256 string `json:"text_sha256"`
}
type realSnippet struct { File, Text string }

func realKey(kind, pkg, receiver, name string) string {
    return kind+"\x00"+pkg+"\x00"+receiver+"\x00"+name
}

// ExportRealAST uses the same MBVM-DB AST ingestion as the production database
// and associates each recorded symbol ID with its real Go declaration text.
// It rejects misalignment, rather than emitting a fake embedding mapping.
func ExportRealAST(root string) (*RealASTManifest, error) {
    abs, err := filepath.Abs(root)
    if err != nil { return nil, err }
    db := mbvmdb.New()
    if err := db.IngestGoAST(abs); err != nil { return nil, err }
    symbols := db.Symbols()
    if len(symbols)==0 { return nil, fmt.Errorf("no Go symbols in %s",abs) }
    if len(symbols)>65536 { return nil, fmt.Errorf("MBVM SymbolRecord.ID is uint16: %d symbols exceed capacity",len(symbols)) }

    snippets := make(map[string][]realSnippet)
    fset:=token.NewFileSet()
    err=filepath.WalkDir(abs,func(path string,d fs.DirEntry,walkErr error)error{
        if walkErr!=nil {return walkErr}
        if d.IsDir() {
            name:=d.Name()
            if path!=abs && (strings.HasPrefix(name,".")||name=="vendor"||name=="node_modules") {
                return filepath.SkipDir
            }
            return nil
        }
        if !strings.HasSuffix(d.Name(),".go")||strings.HasSuffix(d.Name(),"_test.go"){return nil}
        parsed,parseErr:=parser.ParseFile(fset,path,nil,parser.ParseComments)
        if parseErr!=nil {
            // Current IngestGoAST skips invalid files; match that behavior.
            return nil
        }
        rel,err:=filepath.Rel(abs,path)
        if err!=nil {return err}
        rel=filepath.ToSlash(rel)
        ast.Inspect(parsed,func(n ast.Node)bool{
            kind,name,receiver:="","",""
            switch x:=n.(type){
            case *ast.TypeSpec:
                switch x.Type.(type){
                case *ast.StructType: kind="struct"
                case *ast.InterfaceType: kind="interface"
                }
                if kind!="" {name=x.Name.Name}
            case *ast.FuncDecl:
                kind="func"
                name=x.Name.Name
                if x.Recv!=nil && len(x.Recv.List)>0 {
                    var b bytes.Buffer
                    _=printer.Fprint(&b,fset,x.Recv.List[0].Type)
                    receiver=b.String()
                }
            }
            if kind!="" {
                var b bytes.Buffer
                _=printer.Fprint(&b,fset,n)
                key:=realKey(kind,parsed.Name.Name,receiver,name)
                snippets[key]=append(snippets[key],realSnippet{rel,b.String()})
            }
            return true
        })
        return nil
    })
    if err!=nil {return nil,err}

    manifest:=&RealASTManifest{
        Schema:"mbvm-real-ast-v1",
        SourceRoot:filepath.Base(abs),
        Symbols:make([]RealASTSymbol,0,len(symbols)),
        Offsets:make([]uint32,len(symbols)+1),
    }
    index:=make(map[string]uint16,len(symbols)*2)
    methods:=make(map[string][]uint16)
    for i,s:=range symbols {
        if int(s.ID)!=i {return nil,fmt.Errorf("symbol ID mismatch at position %d: %d",i,s.ID)}
        key:=realKey(s.Kind,s.Package,s.Receiver,s.Name)
        matches:=snippets[key]
        if len(matches)==0 {return nil,fmt.Errorf("AST source missing for ID %d (%s.%s)",i,s.Package,s.Name)}
        item:=matches[0]
        snippets[key]=matches[1:]
        h:=sha256.Sum256([]byte(item.Text))
        manifest.Symbols=append(manifest.Symbols,RealASTSymbol{
            ID:uint32(i),Kind:s.Kind,Package:s.Package,Name:s.Name,
            Receiver:s.Receiver,Signature:s.Signature,File:item.File,
            Text:item.Text,TextSHA256:hex.EncodeToString(h[:]),
        })
        id:=uint16(i)
        if s.Kind=="struct"||s.Kind=="interface" {
            index[s.Name]=id
            index[s.Package+"."+s.Name]=id
        } else if s.Kind=="func" {
            caller:=s.Name
            if s.Receiver!="" {
                clean:=mbvmdb.CleanReceiverName(s.Receiver)
                caller=clean+"."+s.Name
                methods[clean]=append(methods[clean],id)
            }
            index[caller]=id
            if _,exists:=index[s.Name];!exists {index[s.Name]=id}
            index[s.Package+"."+s.Name]=id
        }
    }
    // Recreate production buildCSR's symbol name resolution and edge ordering.
    for i,s:=range symbols {
        manifest.Offsets[i]=uint32(len(manifest.Edges))
        for _,call:=range s.OutgoingCalls {
            if target,ok:=index[call];ok {
                manifest.Edges=append(manifest.Edges,target)
                manifest.Relations=append(manifest.Relations,RelCall)
            }
        }
        if s.Kind=="struct" {
            for _,id:=range methods[s.Name] {
                manifest.Edges=append(manifest.Edges,id)
                manifest.Relations=append(manifest.Relations,RelMemberOf)
            }
        }
        if s.Receiver!="" {
            if target,ok:=index[mbvmdb.CleanReceiverName(s.Receiver)];ok {
                manifest.Edges=append(manifest.Edges,target)
                manifest.Relations=append(manifest.Relations,RelMemberOf)
            }
        }
    }
    manifest.Offsets[len(symbols)]=uint32(len(manifest.Edges))
    // Digest text+IDs+graph; the exporter checkpoint must specify this digest.
    raw,err:=json.Marshal(struct{
        Symbols []RealASTSymbol `json:"symbols"`
        Offsets []uint32 `json:"offsets"`
        Edges []uint16 `json:"edges"`
        Relations []EdgeRelation `json:"relations"`
    }{manifest.Symbols,manifest.Offsets,manifest.Edges,manifest.Relations})
    if err!=nil {return nil,err}
    digest:=sha256.Sum256(raw)
    manifest.SourceDigest=hex.EncodeToString(digest[:])
    return manifest,nil
}

func ReadRealASTManifest(path string)(*RealASTManifest,error){
    data,err:=os.ReadFile(path)
    if err!=nil {return nil,err}
    var m RealASTManifest
    if err=json.Unmarshal(data,&m);err!=nil{return nil,err}
    if m.Schema!="mbvm-real-ast-v1" {return nil,fmt.Errorf("unsupported AST manifest schema %q",m.Schema)}
    n:=len(m.Symbols)
    if n==0||n>65536 {return nil,fmt.Errorf("invalid symbol count %d",n)}
    if len(m.Offsets)!=n+1||len(m.Relations)!=len(m.Edges){return nil,fmt.Errorf("invalid CSR array lengths")}
    if m.Offsets[0]!=0||int(m.Offsets[n])!=len(m.Edges){return nil,fmt.Errorf("invalid CSR terminal offset")}
    for i,s:=range m.Symbols {
        if s.ID!=uint32(i) {return nil,fmt.Errorf("symbol ID mismatch: %d at %d",s.ID,i)}
        digest:=sha256.Sum256([]byte(s.Text))
        if hex.EncodeToString(digest[:])!=s.TextSHA256 {return nil,fmt.Errorf("text SHA-256 mismatch for symbol %d",i)}
        if m.Offsets[i]>m.Offsets[i+1] {return nil,fmt.Errorf("non-monotonic CSR offset at %d",i)}
    }
    for _,e:=range m.Edges {if int(e)>=n {return nil,fmt.Errorf("CSR edge out of range: %d",e)}}
    if len(m.SourceDigest)!=64 {return nil,fmt.Errorf("missing source digest")}
    return &m,nil
}

func WriteRealASTManifest(path string,m *RealASTManifest)error{
    if err:=os.MkdirAll(filepath.Dir(path),0755);err!=nil{return err}
    payload,err:=json.MarshalIndent(m,"","  ")
    if err!=nil{return err}
    return os.WriteFile(path,payload,0644)
}
