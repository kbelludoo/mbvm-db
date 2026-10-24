package mbvmdb

import (
	"fmt"
	"strings"
)

// ExtractSlice performs BFS on the call graph starting from target, collecting connected symbols up to depth.
func (db *DB) ExtractSlice(targetSymbol string, maxDepth int) *ContextSlice {
	db.mu.RLock()
	defer db.mu.RUnlock()

	visited := make(map[string]bool)
	var queue []string
	var selected []*SymbolRecord

	// Find starting symbols matching target
	for _, sym := range db.symbols {
		if sym.Name == targetSymbol || fmt.Sprintf("%s.%s", sym.Package, sym.Name) == targetSymbol {
			visited[sym.Name] = true
			queue = append(queue, sym.Name)
			selected = append(selected, sym)
		}
	}

	depth := 0
	for len(queue) > 0 && depth < maxDepth {
		levelSize := len(queue)
		for i := 0; i < levelSize; i++ {
			currName := queue[0]
			queue = queue[1:]

			currSym, ok := db.indexByName[currName]
			if !ok {
				continue
			}

			// Add outgoing calls (callees)
			for _, callee := range currSym.OutgoingCalls {
				if !visited[callee] {
					visited[callee] = true
					queue = append(queue, callee)
					if sym, exists := db.indexByName[callee]; exists {
						selected = append(selected, sym)
					}
				}
			}

			// Add incoming calls (callers)
			for _, caller := range currSym.IncomingCalls {
				if !visited[caller] {
					visited[caller] = true
					queue = append(queue, caller)
					if sym, exists := db.indexByName[caller]; exists {
						selected = append(selected, sym)
					}
				}
			}

			// Also add referenced types in fields/parameters
			for _, f := range currSym.Fields {
				parts := strings.Fields(f)
				for _, p := range parts {
					cleanType := strings.Trim(p, "*[]&")
					if sym, exists := db.indexByName[cleanType]; exists && !visited[cleanType] {
						visited[cleanType] = true
						selected = append(selected, sym)
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

// ProjectSlice formats the extracted symbols into ultra-dense MB-VM syntax.
func (db *DB) ProjectSlice(symbols []*SymbolRecord) string {
	var sb strings.Builder
	sb.WriteString("@MBVM:slice:target%mode=dynamic_slice\n")

	pkgs := make(map[string][]*SymbolRecord)
	for _, s := range symbols {
		pkgs[s.Package] = append(pkgs[s.Package], s)
	}

	for pkg, syms := range pkgs {
		sb.WriteString(fmt.Sprintf("\n📦 pkg %s\n", pkg))
		for _, s := range syms {
			switch s.Kind {
			case "struct":
				sb.WriteString(fmt.Sprintf("  type %s struct { %s }\n", s.Name, strings.Join(s.Fields, ", ")))
			case "interface":
				sb.WriteString(fmt.Sprintf("  type %s interface { %s }\n", s.Name, strings.Join(s.Methods, ", ")))
			case "func":
				if s.Receiver != "" {
					sb.WriteString(fmt.Sprintf("  func (%s) %s%s\n", s.Receiver, s.Name, s.Signature))
				} else {
					sb.WriteString(fmt.Sprintf("  func %s%s\n", s.Name, s.Signature))
				}
			}
		}
	}
	return sb.String()
}
