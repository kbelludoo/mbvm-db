// run_multilemma_depth_dev performs PUBLIC three-premise CRT development tests.
// It does not invoke Gemini or claim novel Erdos-Straus proof synthesis.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	realeval "github.com/kbelludoo/mbvm-db/experiments/real_eval"
)

func main() {
	report := flag.String("report", "", "optional path for the JSON development report")
	flag.Parse()
	lab := realeval.NewCRTDepthLab()
	results, err := lab.RunDepthPaired()
	if err != nil {
		fmt.Fprintf(os.Stderr, "three-premise CRT comparison failed: %v\n", err)
		os.Exit(1)
	}
	matches := 0
	for _, item := range results {
		if item.Conventional.Certified && item.CSR.Certified &&
			item.SameContext && item.SameCertificate {
			matches++
		}
	}
	fmt.Printf("PUBLIC three-premise CRT development: %d/%d matching B/C certificates; no LLM calls or CSR superiority claim.\n", matches, len(results))
	if *report == "" { return }
	payload := struct {
		Schema string `json:"schema"`
		Interpretation string `json:"interpretation"`
		Results []realeval.CRTDepthPairedResult `json:"results"`
	}{
		Schema: "mbvm-crt-three-premise-dev-v1",
		Interpretation: "Public 3-lemma CRT controls. Same knowledge, verifier and executable across B/C. No blind holdout, new theorem, or demonstrated CSR advantage.",
		Results: results,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode JSON: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*report, raw, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "write JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Development report: %s\n", *report)
}
