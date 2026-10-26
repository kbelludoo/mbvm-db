package closedloop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"slices"

	hdccsr "github.com/kbelludoo/mbvm-db/experiments/hdc-csr"
)

// TypedVerifier reloads original source through a DIFFERENT existing extractor
// from the initial impact.Graph. It never accepts model scores or the masked
// graph as evidence. Both extractors use Go's type checker: this is not a
// compiler-independent formal oracle or evidence about dynamic runtime calls.
type TypedVerifier struct {
	snapshot  Snapshot
	known     map[string]bool
	positions map[Relation]string
}

func NewTypedVerifier(ctx context.Context, s Snapshot) (*TypedVerifier, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create verifier: %w", err)
	}
	v := &TypedVerifier{snapshot: s, known: map[string]bool{}, positions: map[Relation]string{}}
	if err := v.EnsureUnchanged(); err != nil {
		return nil, err
	}
	g, err := hdccsr.LoadTypedRealGraph(s.Directory, s.Package)
	if err != nil {
		return nil, fmt.Errorf("independent source typecheck: %w", err)
	}
	names := make([]string, len(g.Nodes))
	for i, n := range g.Nodes {
		key := n.PackagePath + "." + n.Name
		if n.Receiver != "" {
			key = n.PackagePath + ".(" + n.Receiver + ")." + n.Name
		}
		names[i] = key
		v.known[key] = true
	}
	for _, site := range g.CallSites {
		v.positions[Relation{names[site.Caller], names[site.Callee]}] = site.Position
	}
	if err := v.EnsureUnchanged(); err != nil {
		return nil, err
	}
	return v, nil
}

func (v *TypedVerifier) EnsureUnchanged() error {
	pkg, err := build.Default.ImportDir(v.snapshot.Directory, 0)
	if err != nil {
		return fmt.Errorf("select verifier sources: %w", err)
	}
	names := slices.Clone(pkg.GoFiles)
	slices.Sort(names)
	h := sha256.New()
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(v.snapshot.Directory, name))
		if err != nil {
			return fmt.Errorf("fingerprint verifier source: %w", err)
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(raw)
		h.Write([]byte{0})
	}
	env, err := contextDigest(v.snapshot.Directory)
	if err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != v.snapshot.SourceSHA256 || env != v.snapshot.ContextSHA256 {
		return fmt.Errorf("source or module/build context changed since snapshot; refusing stale verification")
	}
	return nil
}

func (v *TypedVerifier) Verify(ctx context.Context, r Relation, version int, score float64) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, fmt.Errorf("verify proposal: %w", err)
	}
	if err := v.EnsureUnchanged(); err != nil {
		return Observation{}, fmt.Errorf("verify current source: %w", err)
	}
	if !v.known[r.Caller] || !v.known[r.Callee] {
		return Observation{}, fmt.Errorf("proposal references an unknown typed symbol")
	}
	pos, exists := v.positions[r]
	reason := "no direct statically resolved local call in this source snapshot"
	if exists {
		reason = "go/types resolves an original source call to the exact local function/method identity"
	}
	return Observation{Relation: r, Confirmed: exists, SourceSHA256: v.snapshot.SourceSHA256, ContextSHA256: v.snapshot.ContextSHA256, Verifier: "independent-go-types-source-v1", Position: pos, Reason: reason, ProposedByVersion: version, Score: score}, nil
}
