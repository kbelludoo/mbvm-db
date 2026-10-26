// Package closedloop runs a bounded, source-verified dependency learning experiment.
// It does not infer runtime calls, mathematical theorems, or unseen program behavior.
package closedloop

import "github.com/kbelludoo/mbvm-db/experiments/hdc-csr"

const featureCount = 12
const hiddenCount = 8

type Relation struct {
	Caller string `json:"caller"`
	Callee string `json:"callee"`
}

type Symbol struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Receiver   string `json:"receiver"`
	Signature  string `json:"signature"`
	File       string `json:"file"`
	nameTokens []string
	typeTokens []string
	vector     hdccsr.Hypervector
}

type Snapshot struct {
	Directory     string `json:"directory"`
	Package       string `json:"package"`
	SourceSHA256  string `json:"source_sha256"`
	ContextSHA256 string `json:"context_sha256"`
	GoVersion     string `json:"go_version"`
	GOOS          string `json:"goos"`
	GOARCH        string `json:"goarch"`
}

type Corpus struct {
	Snapshot Snapshot
	Symbols  []Symbol
	edges    map[Relation]bool
	byID     map[string]int
}

// Split is by source file, not by independently shuffled edges. No Test symbol
// may appear as either caller or callee in training or acquisition observations.
type Split struct {
	Train        []string `json:"train"`
	Acquire      []string `json:"acquire"`
	Test         []string `json:"test"`
	TrainFiles   []string `json:"train_files"`
	AcquireFiles []string `json:"acquire_files"`
	TestFiles    []string `json:"test_files"`
}

type Observation struct {
	Relation          Relation `json:"relation"`
	Confirmed         bool     `json:"confirmed"`
	SourceSHA256      string   `json:"source_sha256"`
	ContextSHA256     string   `json:"context_sha256"`
	Verifier          string   `json:"verifier"`
	Position          string   `json:"position,omitempty"`
	Reason            string   `json:"reason"`
	ProposedByVersion int      `json:"proposed_by_version"`
	Score             float64  `json:"score"`
}

type Fact struct {
	Evidence       Observation `json:"evidence"`
	AddedInVersion int         `json:"added_in_version"`
}

type State struct {
	Schema       string        `json:"schema"`
	Version      int           `json:"version"`
	ModelVersion int           `json:"model_version"`
	ParentSHA256 string        `json:"parent_sha256,omitempty"`
	Snapshot     Snapshot      `json:"snapshot"`
	Split        Split         `json:"split"`
	Model        Model         `json:"model"`
	Facts        []Fact        `json:"facts"`
	Observations []Observation `json:"observations"`
}

type Config struct {
	Directory   string
	Output      string
	Seeds       []int64
	TopK        int
	TrainSteps  int
	UpdateSteps int
}

func DefaultConfig() Config {
	return Config{Directory: "./pkg/mbvmdb", Seeds: []int64{42, 43, 44}, TopK: 5, TrainSteps: 5000, UpdateSteps: 1000}
}

type example struct {
	relation Relation
	x        [featureCount]float64
	y        float64
}
