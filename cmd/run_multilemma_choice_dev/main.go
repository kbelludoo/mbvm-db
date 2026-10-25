// run_multilemma_choice_dev compares two retrieval representations on public
// competing-path CRT fixtures. It does not require a Gemini API key.
package main

import (
 "encoding/json"
 "flag"
 "fmt"
 "os"

 realeval "github.com/kbelludoo/mbvm-db/experiments/real_eval"
)

func main(){
 output:=flag.String("report","","optional JSON report path")
 flag.Parse()
 lab,err:=realeval.NewCRTChoiceLab()
 if err!=nil{fmt.Fprintf(os.Stderr,"initialize proof-path lab: %v\n",err);os.Exit(1)}
 rows,err:=lab.RunChoicePaired()
 if err!=nil{fmt.Fprintf(os.Stderr,"compare proof-path retrievals: %v\n",err);os.Exit(1)}
 fmt.Printf("PUBLIC competing CRT proofs: %d/%d matching B/C certificates; no Gemini calls or CSR advantage claimed.\n",len(rows),len(lab.Tasks))
 if *output==""{return}
 payload:=struct{
  Schema string `json:"schema"`
  Interpretation string `json:"interpretation"`
  Results []realeval.CRTChoicePair `json:"results"`
 }{
  Schema:"mbvm-crt-competing-paths-dev-v1",
  Interpretation:"Public competing proof-path CRT test: same semantics, selector, catalog, and verifier. Tests equivalence and pruning of inapplicable branches, not discovery or CSR superiority.",
  Results:rows,
 }
 raw,err:=json.MarshalIndent(payload,"","  ")
 if err!=nil{fmt.Fprintf(os.Stderr,"encode results: %v\n",err);os.Exit(1)}
 if err:=os.WriteFile(*output,raw,0600);err!=nil{fmt.Fprintf(os.Stderr,"save results: %v\n",err);os.Exit(1)}
 fmt.Printf("Development report: %s\n",*output)
}
