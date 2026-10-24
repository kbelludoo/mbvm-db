package main

import (
    "flag"
    "fmt"
    "os"
    hdccsr "github.com/kbelludoo/mbvm-db/experiments/hdc-csr"
)

func main(){
    root:=flag.String("root",".","Go repository root")
    out:=flag.String("out","output/real_ast_manifest.json","manifest destination (keep source texts private)")
    flag.Parse()
    manifest,err:=hdccsr.ExportRealAST(*root)
    if err!=nil{fmt.Fprintln(os.Stderr,"AST export:",err);os.Exit(1)}
    if err=hdccsr.WriteRealASTManifest(*out,manifest);err!=nil{
        fmt.Fprintln(os.Stderr,"write manifest:",err);os.Exit(1)
    }
    fmt.Printf("Exported %d real Go AST symbols / %d edges; digest=%s; file=%s\n",
        len(manifest.Symbols),len(manifest.Edges),manifest.SourceDigest,*out)
}
