package main

import (
	"flag"
	"fmt"
	"os"

	comparison "github.com/kbelludoo/mbvm-db/experiments/competitor_validation"
)

func main() {
	root := flag.String("root", "", "fixed module root")
	out := flag.String("out", "", "catalogue output path")
	fixture := flag.Bool("fixture", false, "create independent fixture at a new root directory")
	flag.Parse()
	if *root == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "-root and -out are required")
		os.Exit(2)
	}
	if err := comparison.ExportSCIPCatalogue(*root, *out, *fixture); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
