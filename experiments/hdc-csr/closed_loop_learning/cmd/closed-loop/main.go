package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

	closedloop "github.com/kbelludoo/mbvm-db/experiments/hdc-csr/closed_loop_learning"
)

func main() {
	cfg := closedloop.DefaultConfig()
	seeds := "42,43,44"
	flag.StringVar(&cfg.Directory, "dir", cfg.Directory, "Go package source directory (at least five files with functions)")
	flag.StringVar(&cfg.Output, "out", "", "new or empty output directory; required")
	flag.StringVar(&seeds, "seeds", seeds, "comma-separated fixed split/training seeds")
	flag.IntVar(&cfg.TopK, "top-k", cfg.TopK, "proposals per acquisition caller and retrieval cutoff")
	flag.IntVar(&cfg.TrainSteps, "train-steps", cfg.TrainSteps, "initial balanced SGD steps")
	flag.IntVar(&cfg.UpdateSteps, "update-steps", cfg.UpdateSteps, "same incremental/control SGD budget")
	flag.Parse()
	cfg.Seeds = nil
	for _, part := range strings.Split(seeds, ",") {
		seed, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid seed: %s\n", err)
			os.Exit(1)
		}
		cfg.Seeds = append(cfg.Seeds, seed)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	report, err := closedloop.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "closed-loop experiment: %s\n", err)
		os.Exit(1)
	}
	for _, seed := range report.SeedReports {
		before := seed.Metrics["trained_before_update"]
		after := seed.Metrics["trained_after_verified_update"]
		control := seed.Metrics["continued_training_without_new_facts"]
		fmt.Printf("seed=%d verified=%d/%d heldout=%d Recall@%d before=%.4f after=%.4f no-new-facts=%.4f\n", seed.Seed, seed.Confirmed, seed.Proposals, after.EligibleQueries, report.TopK, before.Recall, after.Recall, control.Recall)
	}
	fmt.Printf("Positive recall gain over BOTH controls: %d/%d seeds. Report: %s/report.json\n", report.PositiveSeeds, len(report.SeedReports), cfg.Output)
}
