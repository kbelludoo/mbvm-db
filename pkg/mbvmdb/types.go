package mbvmdb

import "unique"

// OpCodes for the Micro-Bytecode Virtual Machine Database
const (
	OpPkg       byte = 0x10 // [pkg_name_id:uint16]
	OpInterface byte = 0x11 // [name_id:uint16][methods_count:uint16]
	OpStruct    byte = 0x12 // [name_id:uint16][fields_count:uint16]
	OpField     byte = 0x13 // [field_name_id:uint16][type_id:uint16]
	OpFunc      byte = 0x14 // [func_name_id:uint16][receiver_id:uint16][sig_id:uint16]
	OpCallEdge  byte = 0x15 // [caller_id:uint16][callee_id:uint16]
	OpEnd       byte = 0xFF
)

// SymbolRecord represents an indexed entry inside the MBVM Database.
type SymbolRecord struct {
	ID             uint16   `json:"id"`
	Kind           string   `json:"kind"` // "struct", "interface", "func"
	Package        string   `json:"package"`
	Name           string   `json:"name"`
	Receiver       string   `json:"receiver,omitempty"`
	Signature      string   `json:"signature,omitempty"`
	Fields         []string `json:"fields,omitempty"`
	Methods        []string `json:"methods,omitempty"`
	OutgoingCalls  []string `json:"outgoing_calls,omitempty"`
	IncomingCalls  []string `json:"incoming_calls,omitempty"`
	BytecodeOffset uint32   `json:"bytecode_offset"`
	BytecodeLen    uint16   `json:"bytecode_len"`
}

// StringPool stores deduplicated string tokens.
type StringPool struct {
	pool []string
	dict map[string]uint16
}

func NewStringPool() *StringPool {
	return &StringPool{
		pool: make([]string, 0, 512),
		dict: make(map[string]uint16, 512),
	}
}

func (sp *StringPool) Intern(s string) uint16 {
	if id, exists := sp.dict[s]; exists {
		return id
	}
	id := uint16(len(sp.pool))
	sp.pool = append(sp.pool, s)
	sp.dict[s] = id
	return id
}

func (sp *StringPool) Get(id uint16) string {
	if int(id) < len(sp.pool) {
		return sp.pool[id]
	}
	return ""
}

// UniqueStringPool utilizes Go 1.23+ 'unique' package for canonical value interning,
// reducing memory consumption and enabling O(1) pointer-level token comparisons.
type UniqueStringPool struct {
	pool []unique.Handle[string]
	dict map[unique.Handle[string]]uint16
}

func NewUniqueStringPool() *UniqueStringPool {
	return &UniqueStringPool{
		pool: make([]unique.Handle[string], 0, 512),
		dict: make(map[unique.Handle[string]]uint16, 512),
	}
}

func (usp *UniqueStringPool) Intern(s string) uint16 {
	h := unique.Make(s)
	if id, exists := usp.dict[h]; exists {
		return id
	}
	id := uint16(len(usp.pool))
	usp.pool = append(usp.pool, h)
	usp.dict[h] = id
	return id
}

func (usp *UniqueStringPool) Get(id uint16) string {
	if int(id) < len(usp.pool) {
		return usp.pool[id].Value()
	}
	return ""
}

func (usp *UniqueStringPool) Handle(id uint16) (unique.Handle[string], bool) {
	if int(id) < len(usp.pool) {
		return usp.pool[id], true
	}
	return unique.Handle[string]{}, false
}

// ContextSlice represents a targeted sub-graph extracted by query.
type ContextSlice struct {
	TargetSymbol   string
	Symbols        []*SymbolRecord
	TotalFiles     int
	TokensEstimate int
}

// Bitset represents a high-performance 64-bit word bitmap for zero-allocation visited tracking.
type Bitset struct {
	words []uint64
}

// NewBitset initializes a bitset capable of storing up to size bits.
func NewBitset(size int) *Bitset {
	if size <= 0 {
		size = 64
	}
	return &Bitset{
		words: make([]uint64, (size+63)/64),
	}
}

// Has returns true if the bit for id is set.
func (b *Bitset) Has(id uint16) bool {
	w := id >> 6
	if int(w) >= len(b.words) {
		return false
	}
	return (b.words[w] & (uint64(1) << (id & 63))) != 0
}

// Set marks the bit for id as true, expanding words if necessary.
func (b *Bitset) Set(id uint16) {
	w := id >> 6
	if int(w) >= len(b.words) {
		newWords := make([]uint64, w+1)
		copy(newWords, b.words)
		b.words = newWords
	}
	b.words[w] |= (uint64(1) << (id & 63))
}

// ScoredSymbol associates a symbol record with its centrality & relevance score.
type ScoredSymbol struct {
	Record *SymbolRecord
	Score  float64
}
