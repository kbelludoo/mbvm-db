// run_multilemma_dev executes the PUBLIC CRT-only B/C paired retrieval control.
// It does not require a Gemini API key.
package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "os"

    realeval "github.com/kbelludoo/mbvm-db/experiments/real_eval"
)

func main() {
    report := flag.String("report", "", "optional output path for the JSON development report")
    flag.Parse()
    lab := realeval.NewCRTDevelopmentLab()
    results, err := lab.RunCRTDevelopmentPaired()
    if err != nil {
        fmt.Fprintf(os.Stderr, "CRT development comparison failed: %v\n", err)
        os.Exit(1)
    }
    matching := 0
    for _, trial := range results {
        if trial.SameContext && trial.SameCertificate && trial.Conventional.Certified && trial.CSR.Certified {
            matching++
        }
    }
    fmt.Printf("PUBLIC CRT development controls: %d/%d B/C pairs match with certified arithmetic. No LLM inference.\n", matching, len(results))
    if *report != "" {
        payload := struct {
            Schema string `json:"schema"`
            Interpretation string `json:"interpretation"`
            Results []realeval.CRTPairedTrial `json:"results"`
        }{
            Schema: "mbvm-crt-paired-development-v1",
            Interpretation: "Public CRT intersection fixture parity. Neither CSR advantage nor novel Erdos-Straus discovery is demonstrated.",
            Results: results,
        }
        raw, err := json.MarshalIndent(payload, "", "  ")
        if err != nil {fmt.Fprintf(os.Stderr, "encode report: %v\n", err);os.Exit(1)}
        if err := os.WriteFile(*report, raw, 0600);err != nil {fmt.Fprintf(os.Stderr, "write report: %v\n",err);os.Exit(1)}
        fmt.Printf("JSON development report saved to %s\n",*report)
    }
}
