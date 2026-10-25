package main

import (
 "encoding/json"
 "flag"
 "fmt"
 "io"
 "strings"

 "github.com/kbelludoo/mbvm-db/internal/impact"
)

// impact-module performs module-wide static linkage and optionally runs
// real compiler checks of directly linked cross-package Go consumers.
func runImpactModule(args []string,out io.Writer)error{
 flags:=flag.NewFlagSet("impact-module",flag.ContinueOnError)
 flags.SetOutput(io.Discard)
 root:=flags.String("root",".","Go module root directory")
 symbol:=flags.String("symbol","","fully qualified package/receiver/function identity")
 simulate:=flags.String("simulate-add-param","","optional required trailing parameter (int)")
 asJSON:=flags.Bool("json",false,"emit stable JSON")
 if err:=flags.Parse(args);err!=nil{return err}
 if flags.NArg()!=0{return fmt.Errorf("unexpected arguments: %s",strings.Join(flags.Args()," "))}
 if *symbol==""{return fmt.Errorf("-symbol is required")}
 if *simulate!=""&&*simulate!="int"{return fmt.Errorf("-simulate-add-param supports only int")}
 module,err:=impact.LoadModule(*root);if err!=nil{return err}
 result,err:=module.Impact(*symbol);if err!=nil{return err}
 type moduleReport struct{
  Impact impact.ModuleImpact `json:"impact"`
  CrossPackageBreakage *impact.CrossPackageBreakage `json:"cross_package_breakage,omitempty"`
 }
 report:=moduleReport{Impact:result}
 if *simulate=="int"{
  witness,err:=module.CheckCrossPackageBuild(*symbol);if err!=nil{return err}
  report.CrossPackageBreakage=&witness
 }
 if *asJSON{
  enc:=json.NewEncoder(out);enc.SetIndent("","  ")
  return enc.Encode(report)
 }
 fmt.Fprintf(out,"Symbol: %s\nLoaded packages: %d\nTyped edges: %d\n",
  result.Symbol,result.Packages,result.ResolvedEdges)
 fmt.Fprintf(out,"Direct callers (%d):\n",len(result.DirectCallers))
 for _,caller:=range result.DirectCallers{fmt.Fprintln(out," ",caller)}
 fmt.Fprintf(out,"Transitive candidates (%d):\n",len(result.TransitiveCandidates))
 for _,caller:=range result.TransitiveCandidates{fmt.Fprintln(out," ",caller)}
 if report.CrossPackageBreakage!=nil{
  check:=report.CrossPackageBreakage
  fmt.Fprintf(out,"Confirmed cross-package compile breaks (%d):\n",len(check.ConfirmedBrokenConsumers))
  for _,p:=range check.ConfirmedBrokenConsumers{fmt.Fprintln(out," ",p)}
  fmt.Fprintf(out,"Inconclusive consumer builds (%d)\n",len(check.InconclusiveConsumers))
 }
 return nil
}
