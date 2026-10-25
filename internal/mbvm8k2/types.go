// Package mbvm8k2 implements the independent MBVM8K2 v2.0 binary codec.
//
// No code in this package changes the legacy MBVM v1 storage implementation.
// Its symbols and edges are snapshots, not serialized Go AST/type objects.
package mbvm8k2

import "errors"

const (
	headerSize = 128
	directoryEntrySize = 48
	maxSections = 64

	sectionStrings   uint32 = 1
	sectionSymbols   uint32 = 2
	sectionOutOffset uint32 = 3
	sectionOutEdges  uint32 = 4
	sectionInOffset  uint32 = 5
	sectionInEdges   uint32 = 6
	sectionBytecode  uint32 = 7
)

var magic = [8]byte{'M', 'B', 'V', 'M', '8', 'K', '2', 0}
var ErrInvalidFormat = errors.New("mbvm8k2: invalid v2 container")
var ErrTooLarge = errors.New("mbvm8k2: resource limit exceeded")

// Limits caps untrusted allocations. Zero means use the safe default.
// The codec additionally enforces the wire format's uint32 count limits.
type Limits struct {
	MaxFileBytes int64
	MaxSymbols   uint64
	MaxEdges     uint64
	MaxStrings   uint64
}

func (l Limits) normalized() Limits {
	if l.MaxFileBytes <= 0 || l.MaxFileBytes > 256<<20 { l.MaxFileBytes = 256 << 20 }
	if l.MaxSymbols == 0 || l.MaxSymbols > 1_000_000 { l.MaxSymbols = 1_000_000 }
	if l.MaxEdges == 0 || l.MaxEdges > 8_000_000 { l.MaxEdges = 8_000_000 }
	if l.MaxStrings == 0 || l.MaxStrings > 4_000_000 { l.MaxStrings = 4_000_000 }
	return l
}

// Symbol uses qualified identity as its only persistent stable identifier.
// Other IDs are local snapshot ordinals and can change between encodings.
type Symbol struct {
	Qualified string
	Name string
	Package string
	Receiver string
	Kind string
	Signature string
	BytecodeOffset uint64
	BytecodeLength uint64
}

// Edge references the order of symbols supplied in Snapshot.Symbols.
// Duplicate call sites are preserved. Position is an opaque UTF-8 locator.
type Edge struct {
	Caller uint32
	Callee uint32
	Position string
	CrossPackage bool
}

type Snapshot struct {
	Symbols []Symbol
	Edges []Edge
	Bytecode []byte
}
