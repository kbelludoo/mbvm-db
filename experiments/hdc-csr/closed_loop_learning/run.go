package closedloop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

type SeedReport struct {
	Seed                       int64              `json:"seed"`
	Split                      Split              `json:"split"`
	InitialPositiveLinks       int                `json:"initial_positive_links"`
	Proposals                  int                `json:"acquisition_proposals"`
	Confirmed                  int                `json:"new_confirmed_links"`
	Rejected                   int                `json:"proposals_not_observed_in_source"`
	VersionOneSHA256           string             `json:"version_one_sha256"`
	VersionTwoSHA256           string             `json:"version_two_sha256"`
	HoldoutIsolated            bool               `json:"holdout_isolated"`
	Metrics                    map[string]Metrics `json:"metrics"`
	VersusBefore               Gain               `json:"gain_vs_before"`
	VersusMoreTraining         Gain               `json:"gain_vs_more_training_without_new_facts"`
	PositiveRecallGainObserved bool               `json:"positive_recall_gain_vs_both_controls"`
}

type Report struct {
	Schema         string       `json:"schema"`
	Snapshot       Snapshot     `json:"snapshot"`
	Relation       string       `json:"relation"`
	Representation string       `json:"representation"`
	TopK           int          `json:"top_k"`
	TrainSteps     int          `json:"train_steps"`
	UpdateSteps    int          `json:"update_steps"`
	SeedReports    []SeedReport `json:"seeds"`
	PositiveSeeds  int          `json:"positive_gain_seeds"`
	ElapsedMS      int64        `json:"elapsed_ms"`
	Interpretation string       `json:"interpretation"`
	Limits         []string     `json:"limits"`
}

func Run(ctx context.Context, cfg Config) (Report, error) {
	start := time.Now()
	var report Report
	if cfg.Output == "" || len(cfg.Seeds) == 0 || len(cfg.Seeds) > 20 || cfg.TopK < 1 || cfg.TopK > 50 || cfg.TrainSteps < 1 || cfg.TrainSteps > 1000000 || cfg.UpdateSteps < 1 || cfg.UpdateSteps > 1000000 {
		return report, fmt.Errorf("invalid experiment configuration")
	}
	seenSeeds := map[int64]bool{}
	for _, seed := range cfg.Seeds {
		if seenSeeds[seed] {
			return report, fmt.Errorf("duplicate seed")
		}
		seenSeeds[seed] = true
	}
	if entries, err := os.ReadDir(cfg.Output); err == nil && len(entries) > 0 {
		return report, fmt.Errorf("output directory is not empty; preserve prior runs")
	} else if err != nil && !os.IsNotExist(err) {
		return report, fmt.Errorf("inspect output directory: %w", err)
	}
	c, err := LoadCorpus(ctx, cfg.Directory)
	if err != nil {
		return report, err
	}
	verifier, err := NewTypedVerifier(ctx, c.Snapshot)
	if err != nil {
		return report, err
	}
	if err := os.MkdirAll(cfg.Output, 0700); err != nil {
		return report, fmt.Errorf("create output directory: %w", err)
	}
	report = Report{Schema: "mbvm-closed-loop-report-v1", Snapshot: c.Snapshot, Relation: "direct statically resolved same-package Go call", Representation: "metadata-only deterministic HDC8192 + 12-feature 113-parameter MLP; no MiniLM checkpoint used", TopK: cfg.TopK, TrainSteps: cfg.TrainSteps, UpdateSteps: cfg.UpdateSteps, Limits: []string{
		"Public source development experiment; file holdout within one package, not a blind external repository.",
		"No held-out test symbol is used in training/acquisition, in either role; test metadata is only scored after fitting.",
		"Models see declaration metadata only; no function bodies, hidden edges or test labels are input features.",
		"The source oracle has full-source access and can recover static links exactly; scores do not replace a type checker.",
		"The verifier is a separately reloaded extractor but shares Go/types with the original index; not a compiler-independent oracle.",
		"Balanced SGD scores are not calibrated probabilities. Nonlinks mean no resolved static call in this snapshot, not no runtime call.",
		"Fixed budgets/hyperparameters; no search over holdout scores. File bootstrap intervals are descriptive for small dependent public fixtures.",
		"Versioned binary sidecars preserve this experiment only; no new core DB format, GNN, bytecode VM or production learning service.",
	}}
	for _, seed := range cfg.Seeds {
		r, err := runSeed(ctx, c, verifier, cfg, seed)
		if err != nil {
			return report, fmt.Errorf("seed %d: %w", seed, err)
		}
		report.SeedReports = append(report.SeedReports, r)
		if r.PositiveRecallGainObserved {
			report.PositiveSeeds++
		}
	}
	if err := verifier.EnsureUnchanged(); err != nil {
		return report, err
	}
	report.ElapsedMS = time.Since(start).Milliseconds()
	report.Interpretation = "Closed loop executed; generalization benefit is not established unless held-out gains exceed both before-update and equal-step no-new-facts controls. Negative/tied outcomes are retained."
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return report, fmt.Errorf("encode experiment report: %w", err)
	}
	if err := writeNew(filepath.Join(cfg.Output, "report.json"), raw); err != nil {
		return report, err
	}
	return report, nil
}

func runSeed(ctx context.Context, c *Corpus, v *TypedVerifier, cfg Config, seed int64) (SeedReport, error) {
	var r SeedReport
	p, err := c.Partition(seed)
	if err != nil {
		return r, err
	}
	r = SeedReport{Seed: seed, Split: p, Metrics: map[string]Metrics{}}
	initial := c.examples(p.Train, p.Train)
	frozen := newModel(seed)
	before := frozen
	if err := before.train(ctx, initial, cfg.TrainSteps, seed); err != nil {
		return r, err
	}
	s := State{Schema: "mbvm-closed-loop-state-v1", Version: 1, ModelVersion: 1, Snapshot: c.Snapshot, Split: p, Model: before}
	for _, row := range initial {
		if row.y != 1 {
			continue
		}
		o, err := v.Verify(ctx, row.relation, 0, 0)
		if err != nil {
			return r, err
		}
		if !o.Confirmed {
			return r, fmt.Errorf("initial index and independent source verifier disagree")
		}
		s.Observations = append(s.Observations, o)
		s.Facts = append(s.Facts, Fact{o, 1})
	}
	r.InitialPositiveLinks = len(s.Facts)
	dir := filepath.Join(cfg.Output, fmt.Sprintf("seed-%d", seed))
	if err := os.Mkdir(dir, 0700); err != nil {
		return r, fmt.Errorf("create seed directory: %w", err)
	}
	if err := v.EnsureUnchanged(); err != nil {
		return r, err
	}
	path := filepath.Join(dir, "state-0001.mbvmcl")
	r.VersionOneSHA256, err = writeState(path, s)
	if err != nil {
		return r, err
	}
	loaded, hash, err := readState(path)
	if err != nil {
		return r, err
	}
	if hash != r.VersionOneSHA256 {
		return r, fmt.Errorf("initial state changed after persistence")
	}
	s = loaded
	acquisition := append(slices.Clone(p.Train), p.Acquire...)
	for _, caller := range p.Acquire {
		ranking := c.rank(caller, acquisition, modelScorer(s.Model))
		for _, candidate := range ranking[:min(cfg.TopK, len(ranking))] {
			o, err := v.Verify(ctx, Relation{caller, candidate.id}, 1, candidate.score)
			if err != nil {
				return r, err
			}
			s.Observations = append(s.Observations, o)
			r.Proposals++
			if o.Confirmed {
				s.Facts = append(s.Facts, Fact{o, 2})
				r.Confirmed++
			} else {
				r.Rejected++
			}
		}
	}
	if err := v.EnsureUnchanged(); err != nil {
		return r, err
	}
	// Persist verified observations BEFORE using them as incremental labels.
	s.Version = 2
	s.ParentSHA256 = hash
	pending := filepath.Join(dir, "verified-before-training.mbvmcl")
	if _, err := writeState(pending, s); err != nil {
		return r, err
	}
	s, hash, err = readState(pending)
	if err != nil {
		return r, err
	}
	update := slices.Clone(initial)
	for _, o := range s.Observations {
		if o.ProposedByVersion != 1 {
			continue
		}
		y := 0.0
		if o.Confirmed {
			y = 1
		}
		update = append(update, example{o.Relation, features(c.Symbols[c.byID[o.Relation.Caller]], c.Symbols[c.byID[o.Relation.Callee]]), y})
	}
	control := s.Model
	if err := control.train(ctx, initial, cfg.UpdateSteps, seed+1); err != nil {
		return r, err
	}
	if err := s.Model.train(ctx, update, cfg.UpdateSteps, seed+1); err != nil {
		return r, err
	}
	s.Version = 3
	s.ModelVersion = 2
	s.ParentSHA256 = hash
	r.VersionTwoSHA256, err = writeState(filepath.Join(dir, "state-0003.mbvmcl"), s)
	if err != nil {
		return r, err
	}
	final, finalHash, err := readState(filepath.Join(dir, "state-0003.mbvmcl"))
	if err != nil {
		return r, err
	}
	if finalHash != r.VersionTwoSHA256 {
		return r, fmt.Errorf("updated state changed after persistence")
	}
	candidates := make([]string, 0, len(c.Symbols))
	for _, n := range c.Symbols {
		candidates = append(candidates, n.ID)
	}
	arms := map[string]scorer{
		"frozen_untrained_mlp": modelScorer(frozen), "trained_before_update": modelScorer(before),
		"continued_training_without_new_facts": modelScorer(control), "trained_after_verified_update": modelScorer(final.Model),
		"fixed_hdc_no_training":        func(_ Relation, x [featureCount]float64) float64 { return x[2] },
		"deterministic_metadata_rules": func(_ Relation, x [featureCount]float64) float64 { return lexicalScore(x) },
		"full_source_typed_oracle_reference": func(rel Relation, _ [featureCount]float64) float64 {
			if c.edges[rel] {
				return 1
			}
			return 0
		},
	}
	for name, score := range arms {
		r.Metrics[name] = c.evaluate(p.Test, candidates, cfg.TopK, score)
	}
	after := r.Metrics["trained_after_verified_update"]
	r.VersusBefore = pairedGain(r.Metrics["trained_before_update"], after, seed)
	r.VersusMoreTraining = pairedGain(r.Metrics["continued_training_without_new_facts"], after, seed)
	r.PositiveRecallGainObserved = r.VersusBefore.Eligible && r.VersusMoreTraining.Eligible && r.VersusBefore.RecallDelta > 0 && r.VersusMoreTraining.RecallDelta > 0
	r.HoldoutIsolated = true // Also enforced by persisted State.validate on every load.
	if err := saveQueries(filepath.Join(dir, "evaluation.jsonl"), r.Metrics); err != nil {
		return r, err
	}
	return r, nil
}

func modelScorer(m Model) scorer {
	return func(_ Relation, x [featureCount]float64) float64 { return m.score(x) }
}

func saveQueries(path string, metrics map[string]Metrics) error {
	names := make([]string, 0, len(metrics))
	for name := range metrics {
		names = append(names, name)
	}
	slices.Sort(names)
	var raw []byte
	for _, name := range names {
		for _, row := range metrics[name].Rows {
			entry := struct {
				Arm   string      `json:"arm"`
				Query QueryMetric `json:"query"`
			}{name, row}
			line, err := json.Marshal(entry)
			if err != nil {
				return fmt.Errorf("encode query metrics: %w", err)
			}
			raw = append(raw, line...)
			raw = append(raw, '\n')
		}
	}
	return writeNew(path, raw)
}
