package competitorvalidation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/kbelludoo/mbvm-db/internal/impact"
)

func Run(ctx context.Context, cfg Config) (Report, error) {
	var err error
	for _, p := range []*string{&cfg.MBVM, &cfg.Gopls, &cfg.Output} {
		*p, err = filepath.Abs(*p)
		if err != nil {
			return Report{}, fmt.Errorf("resolve configured path: %w", err)
		}
	}
	if err := os.Mkdir(cfg.Output, 0700); err != nil {
		return Report{}, fmt.Errorf("output must be a new directory: %w", err)
	}
	r, err := metadata(cfg)
	if err != nil {
		return r, err
	}
	fixture := filepath.Join(cfg.Output, "fixture")
	if err := makeFixture(fixture); err != nil {
		return r, err
	}
	roots := append([]string{fixture}, cfg.Roots...)
	for i, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			return r, fmt.Errorf("resolve case root: %w", err)
		}
		c := caseResult{Root: abs, Truth: "agreement with current MBVM typed graph"}
		if i == 0 {
			c.Truth = "independent literal fixture labels"
		}
		fmt.Printf("START %d %s\n", i, abs)
		if err := runCase(ctx, cfg, i, &c); err != nil {
			c.Error = err.Error()
		}
		r.Cases = append(r.Cases, c)
		fmt.Printf("DONE %d MBVM %d/%d exact %.3f ms, gopls %d/%d exact %.3f ms, error=%s\n", i, c.MBVMExact, len(c.Pairs), c.MBVM.Median, c.GoplsExact, len(c.Pairs), c.Gopls.Median, c.Error)
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return r, fmt.Errorf("encode report: %w", err)
		}
		if err := os.WriteFile(filepath.Join(cfg.Output, "report.json"), b, 0600); err != nil {
			return r, fmt.Errorf("save report: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return r, fmt.Errorf("comparison stopped: %w", err)
		}
	}
	return r, nil
}

func runCase(ctx context.Context, cfg Config, index int, c *caseResult) (retErr error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	dir := filepath.Join(cfg.Output, fmt.Sprintf("case-%d", index))
	if err := os.Mkdir(dir, 0700); err != nil {
		return fmt.Errorf("create case output: %w", err)
	}
	var err error
	c.SourceBefore, err = snapshot(c.Root)
	if err != nil {
		return err
	}
	g, err := impact.LoadModule(c.Root)
	if err != nil {
		return fmt.Errorf("load selection graph: %w", err)
	}
	c.Symbols = len(g.Symbols)
	cat, err := makeCatalogue(g)
	if err != nil {
		return err
	}
	c.Targets = selectTargets(cat, index == 0)
	if index == 0 {
		truth := fixtureTruth()
		if len(truth) != len(c.Targets) {
			return fmt.Errorf("fixture target catalogue incomplete: %d vs %d", len(c.Targets), len(truth))
		}
		for i := range c.Targets {
			t := &c.Targets[i]
			want, ok := truth[t.Name]
			if !ok {
				return fmt.Errorf("fixture has unlabelled target %s", t.Name)
			}
			if !slices.Equal(t.Want, want) {
				c.FixtureGraphMismatches = append(c.FixtureGraphMismatches, t.Name)
			}
			t.Want = want
		}
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("no eligible targets")
	}
	// Alternate initialization order too; selection has warmed compiler exports.
	var mb *process
	var gp *lspClient
	startMB := func() error {
		start := time.Now()
		mb, err = startProcess(ctx, cfg.MBVM, c.Root, filepath.Join(dir, "mbvm.stderr.txt"), "impact-server", "-root", c.Root)
		if err != nil {
			return err
		}
		a := mb.mbvmQuery(c.Targets[0].Name)
		c.MBVMFirstMS = ms(start)
		if a.Error != "" {
			return fmt.Errorf("MBVM first query: %s", a.Error)
		}
		return nil
	}
	startGP := func() error {
		start := time.Now()
		p, e := startProcess(ctx, cfg.Gopls, c.Root, filepath.Join(dir, "gopls.stderr.txt"), "serve")
		if e != nil {
			return e
		}
		gp = &lspClient{process: p}
		if err := gp.initialize(c.Root); err != nil {
			return err
		}
		if index == 0 {
			if err := gp.fixtureOpen(c.Root); err != nil {
				return err
			}
		}
		if err := gp.prepare(&c.Targets[0]); err != nil {
			return err
		}
		a := gp.query(c.Targets[0], cat)
		c.GoplsFirstMS = ms(start)
		if a.Error != "" {
			return fmt.Errorf("gopls first query: %s", a.Error)
		}
		return nil
	}
	defer func() {
		for _, entry := range []struct {
			p    *process
			name string
		}{{mb, "mbvm"}, {nil, "gopls"}} {
			p := entry.p
			if entry.name == "gopls" && gp != nil {
				p = gp.process
			}
			if p != nil {
				if err := p.close(filepath.Join(dir, entry.name+".wire.json")); err != nil && retErr == nil {
					retErr = err
				}
			}
		}
	}()
	if index%2 == 0 {
		if err := startMB(); err != nil {
			return err
		}
		if err := startGP(); err != nil {
			return err
		}
	} else {
		if err := startGP(); err != nil {
			return err
		}
		if err := startMB(); err != nil {
			return err
		}
	}
	prep := time.Now()
	for i := 1; i < len(c.Targets); i++ {
		if err := gp.prepare(&c.Targets[i]); err != nil {
			return err
		}
	}
	c.PreparationMS = ms(prep)
	for _, t := range c.Targets { // Equal untimed warm-up on every selected target.
		if a := mb.mbvmQuery(t.Name); a.Error != "" {
			return fmt.Errorf("MBVM warmup: %s", a.Error)
		}
		if a := gp.query(t, cat); a.Error != "" {
			return fmt.Errorf("gopls warmup: %s", a.Error)
		}
	}
	for round := range 7 {
		for q, t := range c.Targets {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("paired sample: %w", err)
			}
			p := pair{Round: round, Symbol: t.Name, Want: t.Want, First: "mbvm"}
			if (round+q)%2 == 0 {
				p.MBVM = mb.mbvmQuery(t.Name)
				p.Gopls = gp.query(t, cat)
			} else {
				p.First = "gopls"
				p.Gopls = gp.query(t, cat)
				p.MBVM = mb.mbvmQuery(t.Name)
			}
			compare(&p)
			c.Pairs = append(c.Pairs, p)
		}
	}
	summarize(c)
	c.SourceAfter, err = snapshot(c.Root)
	if err != nil {
		return err
	}
	c.SourcesUnchanged = c.SourceBefore == c.SourceAfter
	if !c.SourcesUnchanged {
		return fmt.Errorf("source snapshot changed during comparison")
	}
	if index == 0 {
		editStart := time.Now()
		if err := editFixture(c.Root, gp); err != nil {
			return err
		}
		c.EditAndNotifyMS = ms(editStart)
		for _, name := range []string{"example.com/callfixture.Target", "example.com/callfixture.Unused"} {
			i := slices.IndexFunc(c.Targets, func(t target) bool { return t.Name == name })
			t := c.Targets[i]
			want := slices.Clone(t.Want)
			if name == "example.com/callfixture.Target" {
				want = slices.DeleteFunc(want, func(s string) bool { return s == "example.com/callfixture.Switch" })
			} else {
				want = []string{"example.com/callfixture.Switch"}
			}
			p := pair{Symbol: name, Want: want, First: "gopls"}
			p.Gopls = gp.query(t, cat)
			p.MBVM = mb.mbvmQuery(name)
			compare(&p)
			c.Freshness = append(c.Freshness, p)
		}
	}
	return nil
}
