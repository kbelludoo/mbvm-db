package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	comparison "github.com/kbelludoo/mbvm-db/experiments/competitor_validation"
)

func main() {
	mbvm := flag.String("mbvm", "output/competitor-tools/mbvm-db.exe", "MBVM executable")
	gopls := flag.String("gopls", "output/competitor-tools/gopls.exe", "pinned gopls executable")
	out := flag.String("out", "", "new report directory")
	roots := flag.String("roots", ".;../twitch-miner-go;../kick-miner-go", "semicolon-separated module roots; empty means only fixture")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "-out must name a new directory")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	var selected []string
	if *roots != "" {
		selected = strings.Split(*roots, ";")
	}
	_, err := comparison.Run(ctx, comparison.Config{MBVM: *mbvm, Gopls: *gopls, Output: *out, Roots: selected})
	if err != nil {
		fmt.Fprintf(os.Stderr, "comparison: %s\n", err)
		os.Exit(1)
	}
}
