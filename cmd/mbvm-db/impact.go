package main

import (
 "encoding/json"
 "flag"
 "fmt"
 "io"
 "strings"

 "github.com/kbelludoo/mbvm-db/internal/impact"
)

// runImpact is deliberately a separate testable command handler.
// JSON goes only to stdout; failures are returned to the CLI entrypoint.
func runImpact(args []string,out io.Writer)error{
 flags:=flag.NewFlagSet("impact",flag.ContinueOnError)
 flags.SetOutput(io.Discard)
 dir:=flags.String("dir",".","Go package source directory")
 symbol:=flags.String("symbol","","fully qualified or unambiguous function/method name")
 jsonOutput:=flags.Bool("json",false,"machine-readable result")
 extra:=flags.String("simulate-add-param","","only 'int' is currently supported")
 if err:=flags.Parse(args);err!=nil{return fmt.Errorf("impact flags: %w",err)}
 if flags.NArg()!=0{return fmt.Errorf("unexpected arguments: %s",strings.Join(flags.Args()," "))}
 if strings.TrimSpace(*symbol)==""{return fmt.Errorf("-symbol is required")}
 simulate:=*extra!=""
 if simulate&&*extra!="int"{return fmt.Errorf("-simulate-add-param supports only 'int'")}
 report,err:=impact.Analyze(*dir,*symbol,simulate)
 if err!=nil{return err}
 if *jsonOutput{
  encoder:=json.NewEncoder(out)
  encoder.SetIndent("","  ")
  return encoder.Encode(report)
 }
 fmt.Fprintf(out,"Symbol: %s\nSignature: %s\nSource SHA-256: %s\n",report.Symbol,report.Signature,report.SourceSHA256)
 fmt.Fprintf(out,"Direct callers (%d):\n",len(report.DirectCallers))
 for _,n:=range report.DirectCallers{fmt.Fprintf(out,"  %s\n",n)}
 fmt.Fprintf(out,"Transitive candidates (%d), depth %d:\n",report.TransitiveCandidates,report.MaxDepth)
 for _,n:=range report.TransitiveCallers{fmt.Fprintf(out,"  %s\n",n)}
 if report.Simulation!=nil{
  sim:=report.Simulation
  fmt.Fprintf(out,"Simulated contract change: %s\nType diagnostics: %d\n",sim.Mutation,sim.TypeDiagnostics)
  fmt.Fprintf(out,"Direct callers with type errors (%d):\n",len(sim.BrokenDirectCallers))
  for _,n:=range sim.BrokenDirectCallers{fmt.Fprintf(out,"  %s\n",n)}
 }
 return nil
}
