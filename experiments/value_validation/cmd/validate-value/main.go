package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	valuevalidation "github.com/kbelludoo/mbvm-db/experiments/value_validation"
)

func main() {
	roots := flag.String("roots", ".;../twitch-miner-go;../kick-miner-go", "semicolon-separated module roots; preserve failures")
	output := flag.String("out", "", "new or empty report directory")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	r, err := valuevalidation.Run(ctx, valuevalidation.Config{Roots: strings.Split(*roots, ";"), Output: *output})
	if err != nil {
		fmt.Fprintf(os.Stderr, "value validation: %s\n", err)
		os.Exit(1)
	}
	for _, c := range r.Cases {
		if c.Error != "" {
			fmt.Printf("INCONCLUSIVE %s: %s\n", c.Root, c.Error)
			continue
		}
		fmt.Printf("%s: %d symbols, %d/%d adjacency parity, cache %.2fx (%.2f -> %.2f ms), indexed adjacency/CSR %.2fx\n", c.Root, c.Symbols, c.AllSymbolsChecked, c.Symbols, c.FreshOverCache, c.Fresh.Median, c.Cached.Median, c.Micro.AdjacencyOverCSR)
	}
	fmt.Printf("Report: %s/report.json\n", *output)
}
