package mbvmdb

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// ExtractSlice performs ultra-fast BFS on the CSR graph starting from target, collecting connected symbols up to depth.
func (db *DB) ExtractSlice(targetSymbol string, maxDepth int) *ContextSlice {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.extractSliceLocked(targetSymbol, maxDepth)
}

func (db *DB) extractSliceLocked(targetSymbol string, maxDepth int) *ContextSlice {
	if targetSymbol == "" || maxDepth < 0 {
		return &ContextSlice{
			TargetSymbol:   targetSymbol,
			Symbols:        nil,
			TokensEstimate: 0,
		}
	}

	visitedBitset := NewBitset(len(db.symbols) + 64)
	var queue []uint16
	var selected []*SymbolRecord

	// Find starting symbols matching target
	// 1. Exact match check first
	for _, sym := range db.symbols {
		fullName := fmt.Sprintf("%s.%s", sym.Package, sym.Name)
		var recvName string
		if sym.Receiver != "" {
			cleanRecv := CleanReceiverName(sym.Receiver)
			recvName = fmt.Sprintf("%s.%s", cleanRecv, sym.Name)
		}

		if sym.Name == targetSymbol || fullName == targetSymbol || (recvName != "" && recvName == targetSymbol) {
			if !visitedBitset.Has(sym.ID) {
				visitedBitset.Set(sym.ID)
				selected = append(selected, sym)
				queue = append(queue, sym.ID)
			}
		}
	}

	// 2. Fallback to case-insensitive or index alias only if no exact match was found
	if len(selected) == 0 {
		targetLower := strings.ToLower(targetSymbol)
		if directSym, exists := db.indexByName[targetLower]; exists {
			if !visitedBitset.Has(directSym.ID) {
				visitedBitset.Set(directSym.ID)
				selected = append(selected, directSym)
				queue = append(queue, directSym.ID)
			}
		}

		if len(selected) == 0 {
			for _, sym := range db.symbols {
				if strings.EqualFold(sym.Name, targetSymbol) {
					if !visitedBitset.Has(sym.ID) {
						visitedBitset.Set(sym.ID)
						selected = append(selected, sym)
						queue = append(queue, sym.ID)
					}
				}
			}
		}
	}

	// BFS traversal over CSR adjacency arrays
	depth := 0
	for len(queue) > 0 && depth < maxDepth {
		levelSize := len(queue)
		for i := 0; i < levelSize; i++ {
			currID := queue[0]
			queue = queue[1:]

			if int(currID) >= len(db.symbols) {
				continue
			}
			currSym := db.symbols[currID]

			// 1. Traverse outgoing edges via CSR
			if int(currID+1) < len(db.csrOffsets) {
				startEdge := db.csrOffsets[currID]
				endEdge := db.csrOffsets[currID+1]
				for e := startEdge; e < endEdge; e++ {
					neighborID := db.csrEdges[e]
					if !visitedBitset.Has(neighborID) {
						visitedBitset.Set(neighborID)
						queue = append(queue, neighborID)
						selected = append(selected, db.symbols[neighborID])
					}
				}
			}

			// 2. Traverse incoming edges via Reverse CSR
			if int(currID+1) < len(db.csrRevOffsets) {
				startRev := db.csrRevOffsets[currID]
				endRev := db.csrRevOffsets[currID+1]
				for e := startRev; e < endRev; e++ {
					neighborID := db.csrRevEdges[e]
					if !visitedBitset.Has(neighborID) {
						visitedBitset.Set(neighborID)
						queue = append(queue, neighborID)
						selected = append(selected, db.symbols[neighborID])
					}
				}
			}

			// 3. Add referenced types in struct fields and function signatures
			for _, f := range currSym.Fields {
				typeTokens := extractTypeTokens(f)
				for _, token := range typeTokens {
					if sym, exists := db.indexByName[token]; exists {
						if !visitedBitset.Has(sym.ID) {
							visitedBitset.Set(sym.ID)
							queue = append(queue, sym.ID)
							selected = append(selected, sym)
						}
					}
				}
			}
		}
		depth++
	}

	projection := db.ProjectSlice(selected)
	tokensEstimate := len(projection) / 4

	return &ContextSlice{
		TargetSymbol:   targetSymbol,
		Symbols:        selected,
		TokensEstimate: tokensEstimate,
	}
}

// ExtractBudgetSlice extracts a context slice constrained strictly to maxTokenBudget.
// It prioritizes core dependencies using Personalized Relevance & Centrality discounting.
func (db *DB) ExtractBudgetSlice(targetSymbol string, maxDepth int, maxTokenBudget int) *ContextSlice {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if targetSymbol == "" || maxDepth < 0 || maxTokenBudget <= 0 {
		return &ContextSlice{
			TargetSymbol:   targetSymbol,
			Symbols:        nil,
			TokensEstimate: 0,
		}
	}

	visitedBitset := NewBitset(len(db.symbols) + 64)
	type candidate struct {
		sym   *SymbolRecord
		score float64
		depth int
	}
	var scoredCandidates []candidate
	var queue []uint16

	// Find starting symbols matching target
	for _, sym := range db.symbols {
		fullName := fmt.Sprintf("%s.%s", sym.Package, sym.Name)
		var recvName string
		if sym.Receiver != "" {
			cleanRecv := CleanReceiverName(sym.Receiver)
			recvName = fmt.Sprintf("%s.%s", cleanRecv, sym.Name)
		}

		if sym.Name == targetSymbol || fullName == targetSymbol || (recvName != "" && recvName == targetSymbol) {
			if !visitedBitset.Has(sym.ID) {
				visitedBitset.Set(sym.ID)
				queue = append(queue, sym.ID)
				scoredCandidates = append(scoredCandidates, candidate{
					sym:   sym,
					score: 1.0, // Maximum priority for the query target
					depth: 0,
				})
			}
		}
	}

	// BFS with scoring
	depth := 0
	for len(queue) > 0 && depth < maxDepth {
		levelSize := len(queue)
		depth++
		depthFactor := 1.0 / float64(depth)

		for i := 0; i < levelSize; i++ {
			currID := queue[0]
			queue = queue[1:]

			if int(currID) >= len(db.symbols) {
				continue
			}
			currSym := db.symbols[currID]

			addNeighbor := func(neighborID uint16, baseWeight float64) {
				if !visitedBitset.Has(neighborID) {
					visitedBitset.Set(neighborID)
					neighborSym := db.symbols[neighborID]

					// Anti-noise / Centrality discount:
					// Common utilities (logging, formatting) called by > 15 functions receive a discount
					noiseDiscount := 1.0
					if len(neighborSym.IncomingCalls) > 15 {
						noiseDiscount = 15.0 / float64(len(neighborSym.IncomingCalls))
					}

					score := baseWeight * depthFactor * noiseDiscount
					scoredCandidates = append(scoredCandidates, candidate{
						sym:   neighborSym,
						score: score,
						depth: depth,
					})
					queue = append(queue, neighborID)
				}
			}

			// Outgoing CSR edges
			if int(currID+1) < len(db.csrOffsets) {
				for e := db.csrOffsets[currID]; e < db.csrOffsets[currID+1]; e++ {
					addNeighbor(db.csrEdges[e], 0.8)
				}
			}

			// Incoming CSR edges
			if int(currID+1) < len(db.csrRevOffsets) {
				for e := db.csrRevOffsets[currID]; e < db.csrRevOffsets[currID+1]; e++ {
					addNeighbor(db.csrRevEdges[e], 0.6)
				}
			}

			// Fields and Types (high priority context)
			for _, f := range currSym.Fields {
				typeTokens := extractTypeTokens(f)
				for _, token := range typeTokens {
					if sym, exists := db.indexByName[token]; exists {
						addNeighbor(sym.ID, 0.9)
					}
				}
			}
		}
	}

	// Sort candidates by score descending (most relevant first)
	sort.Slice(scoredCandidates, func(i, j int) bool {
		if scoredCandidates[i].score != scoredCandidates[j].score {
			return scoredCandidates[i].score > scoredCandidates[j].score
		}
		return scoredCandidates[i].depth < scoredCandidates[j].depth
	})

	// Greedily fit candidates within maxTokenBudget
	var budgetSelected []*SymbolRecord
	for _, c := range scoredCandidates {
		testSelected := append(budgetSelected, c.sym)
		proj := db.ProjectSlice(testSelected)
		estimatedTokens := len(proj) / 4
		if estimatedTokens <= maxTokenBudget || len(budgetSelected) == 0 {
			budgetSelected = testSelected
		} else {
			// Once budget is reached, stop adding lower-scored peripheral nodes
			break
		}
	}

	finalProj := db.ProjectSlice(budgetSelected)
	return &ContextSlice{
		TargetSymbol:   targetSymbol,
		Symbols:        budgetSelected,
		TokensEstimate: len(finalProj) / 4,
	}
}

// extractTypeTokens extracts identifier tokens from complex type declarations
func extractTypeTokens(s string) []string {
	var tokens []string
	var current strings.Builder

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// ProjectSlice formats the extracted symbols into ultra-dense, 100% deterministic MB-VM syntax.
func (db *DB) ProjectSlice(symbols []*SymbolRecord) string {
	if len(symbols) == 0 {
		return "@MBVM:slice:empty\n"
	}

	var sb strings.Builder
	sb.WriteString("@MBVM:slice:target%mode=dynamic_slice\n")

	// Group symbols by package
	pkgs := make(map[string][]*SymbolRecord)
	for _, s := range symbols {
		pkgs[s.Package] = append(pkgs[s.Package], s)
	}

	// Sort package names alphabetically for 100% determinism
	pkgList := make([]string, 0, len(pkgs))
	for p := range pkgs {
		pkgList = append(pkgList, p)
	}
	sort.Strings(pkgList)

	for _, pkg := range pkgList {
		syms := pkgs[pkg]
		// Deterministic sort within package: structs -> interfaces -> funcs, then by Name
		sort.Slice(syms, func(i, j int) bool {
			kindOrder := func(k string) int {
				switch k {
				case "struct":
					return 1
				case "interface":
					return 2
				case "func":
					return 3
				default:
					return 4
				}
			}
			ki, kj := kindOrder(syms[i].Kind), kindOrder(syms[j].Kind)
			if ki != kj {
				return ki < kj
			}
			if syms[i].Name != syms[j].Name {
				return syms[i].Name < syms[j].Name
			}
			return syms[i].Receiver < syms[j].Receiver
		})

		sb.WriteString(fmt.Sprintf("\n📦 pkg %s\n", pkg))
		for _, s := range syms {
			switch s.Kind {
			case "struct":
				if len(s.Fields) == 0 {
					sb.WriteString(fmt.Sprintf("  type %s struct {}\n", s.Name))
				} else {
					sb.WriteString(fmt.Sprintf("  type %s struct { %s }\n", s.Name, strings.Join(s.Fields, ", ")))
				}
			case "interface":
				if len(s.Methods) == 0 {
					sb.WriteString(fmt.Sprintf("  type %s interface {}\n", s.Name))
				} else {
					sb.WriteString(fmt.Sprintf("  type %s interface { %s }\n", s.Name, strings.Join(s.Methods, ", ")))
				}
			case "func":
				if s.Receiver != "" {
					sb.WriteString(fmt.Sprintf("  func (%s) %s%s\n", s.Receiver, s.Name, s.Signature))
				} else {
					sb.WriteString(fmt.Sprintf("  func %s%s\n", s.Name, s.Signature))
				}
			case "proto_message":
				sb.WriteString(fmt.Sprintf("  message %s {\n", s.Name))
				for _, f := range s.Fields {
					sb.WriteString(fmt.Sprintf("    %s\n", f))
				}
				sb.WriteString("  }\n")
			case "proto_enum":
				sb.WriteString(fmt.Sprintf("  enum %s {\n", s.Name))
				for _, f := range s.Fields {
					sb.WriteString(fmt.Sprintf("    %s;\n", f))
				}
				sb.WriteString("  }\n")
			case "proto_tag", "proto_enum_val":
				sb.WriteString(fmt.Sprintf("  %s (%s)\n", s.Name, s.Receiver))
			case "game_hunt":
				sb.WriteString(fmt.Sprintf("  tier %s (%s) [%s]\n", s.Name, s.Receiver, s.Signature))
				for _, f := range s.Fields {
					sb.WriteString(fmt.Sprintf("    • %s\n", f))
				}
			default:
				sb.WriteString(fmt.Sprintf("  symbol %s: %s\n", s.Name, strings.Join(s.Fields, ", ")))
			}
		}
	}
	return sb.String()
}
