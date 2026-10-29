package competitorvalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type Config struct {
	MBVM, Gopls, Output string
	Roots               []string
}
type distribution struct {
	Samples int     `json:"samples"`
	Median  float64 `json:"median"`
	P95     float64 `json:"p95"`
}
type pair struct {
	Round        int      `json:"round"`
	Symbol       string   `json:"symbol"`
	First        string   `json:"first"`
	Want         []string `json:"expected"`
	MBVM         answer   `json:"mbvm"`
	Gopls        answer   `json:"gopls"`
	MBVMMissing  []string `json:"mbvm_missing"`
	MBVMExtra    []string `json:"mbvm_extra"`
	GoplsMissing []string `json:"gopls_missing"`
	GoplsExtra   []string `json:"gopls_extra"`
}
type caseResult struct {
	Root                   string       `json:"root"`
	Error                  string       `json:"error,omitempty"`
	Truth                  string       `json:"truth"`
	SourceBefore           string       `json:"source_before"`
	SourceAfter            string       `json:"source_after"`
	SourcesUnchanged       bool         `json:"sources_unchanged_before_fixture_edit"`
	Symbols                int          `json:"module_symbols"`
	Targets                []target     `json:"targets"`
	FixtureGraphMismatches []string     `json:"fixture_graph_mismatches,omitempty"`
	MBVMFirstMS            float64      `json:"mbvm_process_to_first_answer_ms"`
	GoplsFirstMS           float64      `json:"gopls_process_to_first_answer_ms"`
	PreparationMS          float64      `json:"remaining_gopls_target_preparation_ms"`
	Pairs                  []pair       `json:"pairs"`
	MBVM                   distribution `json:"mbvm_warm_ms"`
	Gopls                  distribution `json:"gopls_warm_ms"`
	MBVMExact              int          `json:"mbvm_exact_samples"`
	GoplsExact             int          `json:"gopls_exact_samples"`
	MBVMCacheHits          int          `json:"mbvm_cache_hits"`
	Freshness              []pair       `json:"fixture_saved_edit,omitempty"`
	EditAndNotifyMS        float64      `json:"fixture_save_and_notify_ms,omitempty"`
}
type Report struct {
	Schema       string          `json:"schema"`
	Go           string          `json:"go"`
	Platform     string          `json:"platform"`
	GOMAXPROCS   int             `json:"gomaxprocs"`
	GoEnv        json.RawMessage `json:"go_env"`
	GoplsVersion string          `json:"gopls_version"`
	GoplsSHA     string          `json:"gopls_binary_sha256"`
	MBVMSHA      string          `json:"mbvm_binary_sha256"`
	ProtocolSHA  string          `json:"protocol_sha256"`
	Cases        []caseResult    `json:"cases"`
	Limits       []string        `json:"limits"`
}

func hashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

func snapshot(root string) (string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (d.Name() == ".git" || d.Name() == "output" || d.Name() == "vendor" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") || d.Name() == "go.mod" || d.Name() == "go.sum" || d.Name() == "go.work" || d.Name() == "go.work.sum" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("source snapshot: %w", err)
	}
	slices.Sort(paths)
	h := sha256.New()
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return "", fmt.Errorf("snapshot path: %w", err)
		}
		digest, err := hashFile(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%s\n", filepath.ToSlash(rel), digest)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func dist(xs []float64) distribution {
	if len(xs) == 0 {
		return distribution{}
	}
	x := slices.Clone(xs)
	slices.Sort(x)
	m := x[len(x)/2]
	if len(x)%2 == 0 {
		m = (x[len(x)/2-1] + m) / 2
	}
	return distribution{len(x), m, x[int(math.Ceil(.95*float64(len(x))))-1]}
}

func difference(a, b []string) []string {
	out := []string{}
	for _, s := range a {
		if !slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}

func compare(p *pair) {
	p.MBVMMissing = difference(p.Want, p.MBVM.Callers)
	p.MBVMExtra = difference(p.MBVM.Callers, p.Want)
	p.GoplsMissing = difference(p.Want, p.Gopls.Callers)
	p.GoplsExtra = difference(p.Gopls.Callers, p.Want)
}

func summarize(c *caseResult) {
	var a, b []float64
	for _, p := range c.Pairs {
		a = append(a, p.MBVM.Milliseconds)
		b = append(b, p.Gopls.Milliseconds)
		if p.MBVM.Error == "" && len(p.MBVMMissing)+len(p.MBVMExtra) == 0 {
			c.MBVMExact++
		}
		if p.Gopls.Error == "" && len(p.GoplsMissing)+len(p.GoplsExtra) == 0 {
			c.GoplsExact++
		}
		if p.MBVM.CacheStats != nil && p.MBVM.CacheStats.CacheHit {
			c.MBVMCacheHits++
		}
	}
	c.MBVM = dist(a)
	c.Gopls = dist(b)
}

func metadata(cfg Config) (Report, error) {
	r := Report{Schema: "mbvm-gopls-persistent-v1", Go: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH, GOMAXPROCS: runtime.GOMAXPROCS(0), Limits: []string{
		"Only package-level function targets and named production callers in three local Go modules plus one labelled fixture; no concrete/interface method target comparison.",
		"Real-project truth is MBVM graph agreement, not independent correctness. Literal fixture truth is independent of both implementations.",
		"Both servers persist. Preparation is separate. Go build and OS disk caches may be warm; process-to-first-answer is not cold disk time.",
		"Actual API cost: MBVM also returns transitive impact. gopls returns call-hierarchy references normalized by independent source syntax.",
		"No measurement of total process-tree memory, persistent index size, neural learning, Glean or full Sourcegraph.",
		"One machine, seven rounds. Failed and mismatched results are retained; freshness has only one saved fixture edit.",
	}}
	b, err := exec.Command(cfg.Gopls, "version").CombinedOutput()
	if err != nil {
		return r, fmt.Errorf("gopls version: %w: %s", err, b)
	}
	r.GoplsVersion = strings.TrimSpace(string(b))
	r.GoplsSHA, err = hashFile(cfg.Gopls)
	if err != nil {
		return r, err
	}
	r.MBVMSHA, err = hashFile(cfg.MBVM)
	if err != nil {
		return r, err
	}
	r.ProtocolSHA, err = hashFile("experiments/competitor_validation/PROTOCOL.md")
	if err != nil {
		return r, err
	}
	r.GoEnv, err = exec.Command("go", "env", "-json", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS", "GOWORK", "GOPROXY", "GOSUMDB").Output()
	if err != nil {
		return r, fmt.Errorf("record Go environment: %w", err)
	}
	return r, nil
}
