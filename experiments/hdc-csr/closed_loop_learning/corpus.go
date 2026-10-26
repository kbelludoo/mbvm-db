package closedloop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/kbelludoo/mbvm-db/internal/impact"
)

func LoadCorpus(ctx context.Context, dir string) (*Corpus, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load corpus: %w", err)
	}
	g, err := impact.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("load initial typed index: %w", err)
	}
	if len(g.Symbols) < 6 || len(g.Symbols) > 512 {
		return nil, fmt.Errorf("experiment requires 6..512 declared functions, got %d", len(g.Symbols))
	}
	digest, err := g.SourceSHA256()
	if err != nil {
		return nil, fmt.Errorf("fingerprint indexed source: %w", err)
	}
	env, err := contextDigest(g.Dir)
	if err != nil {
		return nil, err
	}
	c := &Corpus{Snapshot: Snapshot{Directory: g.Dir, Package: g.PackagePath, SourceSHA256: digest, ContextSHA256: env, GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}, edges: map[Relation]bool{}, byID: map[string]int{}}
	for _, s := range g.Symbols {
		filename, err := positionFile(s.Position)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(g.Dir, filename)
		if err != nil || !filepath.IsLocal(rel) {
			return nil, fmt.Errorf("source position outside selected package: %s", s.Position)
		}
		n := Symbol{ID: s.Qualified(), Name: s.Name, Receiver: s.Receiver, Signature: s.Signature, File: filepath.ToSlash(rel)}
		n.nameTokens = tokens(n.Name)
		n.typeTokens = tokens(n.Receiver + " " + n.Signature)
		n.vector = encodeSymbol(n)
		c.Symbols = append(c.Symbols, n)
	}
	slices.SortFunc(c.Symbols, func(a, b Symbol) int { return strings.Compare(a.ID, b.ID) })
	for i, s := range c.Symbols {
		c.byID[s.ID] = i
	}
	for _, e := range g.Calls {
		c.edges[Relation{g.Symbols[e.Caller].Qualified(), g.Symbols[e.Callee].Qualified()}] = true
	}
	return c, nil
}

func positionFile(position string) (string, error) {
	// Remove numeric line/column suffixes; preserve Windows drive colons.
	for range 2 {
		at := strings.LastIndexByte(position, ':')
		if at < 0 {
			return "", fmt.Errorf("invalid source position %q", position)
		}
		if _, err := strconv.Atoi(position[at+1:]); err != nil {
			return "", fmt.Errorf("parse source position: %w", err)
		}
		position = position[:at]
	}
	return position, nil
}

func contextDigest(dir string) (string, error) {
	h := sha256.New()
	for _, s := range []string{runtime.Version(), runtime.GOOS, runtime.GOARCH, os.Getenv("GOFLAGS"), os.Getenv("GOWORK"), os.Getenv("CGO_ENABLED"), os.Getenv("GOEXPERIMENT"), os.Getenv("GOTOOLCHAIN")} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	for at := dir; ; at = filepath.Dir(at) {
		mod := filepath.Join(at, "go.mod")
		if _, err := os.Stat(mod); err == nil {
			for _, name := range []string{"go.mod", "go.sum"} {
				data, err := os.ReadFile(filepath.Join(at, name))
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					return "", fmt.Errorf("fingerprint module context: %w", err)
				}
				h.Write([]byte(name))
				h.Write(data)
			}
			break
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("locate module context: %w", err)
		}
		if filepath.Dir(at) == at {
			break
		}
	}
	workspace := os.Getenv("GOWORK")
	if workspace == "" {
		for at := dir; ; at = filepath.Dir(at) {
			candidate := filepath.Join(at, "go.work")
			if _, err := os.Stat(candidate); err == nil {
				workspace = candidate
				break
			} else if !os.IsNotExist(err) {
				return "", fmt.Errorf("locate workspace: %w", err)
			}
			if filepath.Dir(at) == at {
				break
			}
		}
	}
	if workspace != "" && workspace != "off" {
		for _, name := range []string{workspace, workspace + ".sum"} {
			data, err := os.ReadFile(name)
			if os.IsNotExist(err) && name != workspace {
				continue
			}
			if err != nil {
				return "", fmt.Errorf("fingerprint workspace: %w", err)
			}
			h.Write([]byte(filepath.Base(name)))
			h.Write([]byte{0})
			h.Write(data)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (c *Corpus) Partition(seed int64) (Split, error) {
	files := []string{}
	for _, s := range c.Symbols {
		if !slices.Contains(files, s.File) {
			files = append(files, s.File)
		}
	}
	if len(files) < 5 {
		return Split{}, fmt.Errorf("need at least five source files with functions, got %d", len(files))
	}
	key := func(file string) [32]byte { return sha256.Sum256([]byte(fmt.Sprintf("%d:%s", seed, file))) }
	slices.SortFunc(files, func(a, b string) int {
		x, y := key(a), key(b)
		return strings.Compare(hex.EncodeToString(x[:]), hex.EncodeToString(y[:]))
	})
	nTrain := len(files) * 6 / 10
	nAcquire := max(1, len(files)*2/10)
	p := Split{TrainFiles: slices.Clone(files[:nTrain]), AcquireFiles: slices.Clone(files[nTrain : nTrain+nAcquire]), TestFiles: slices.Clone(files[nTrain+nAcquire:])}
	for _, s := range c.Symbols {
		switch {
		case slices.Contains(p.TrainFiles, s.File):
			p.Train = append(p.Train, s.ID)
		case slices.Contains(p.AcquireFiles, s.File):
			p.Acquire = append(p.Acquire, s.ID)
		default:
			p.Test = append(p.Test, s.ID)
		}
	}
	return p, nil
}

func (c *Corpus) examples(callers, candidates []string) []example {
	rows := make([]example, 0, len(callers)*len(candidates))
	for _, a := range callers {
		for _, b := range candidates {
			y := 0.0
			if c.edges[Relation{a, b}] {
				y = 1
			}
			rows = append(rows, example{Relation{a, b}, features(c.Symbols[c.byID[a]], c.Symbols[c.byID[b]]), y})
		}
	}
	return rows
}
