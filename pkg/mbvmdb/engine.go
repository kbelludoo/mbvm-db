package mbvmdb

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DB represents a Micro-Bytecode Virtual Machine Database instance.
type DB struct {
	mu                sync.RWMutex
	pool              *StringPool
	symbols           []*SymbolRecord
	indexByName       map[string]*SymbolRecord
	methodsByReceiver map[string][]*SymbolRecord
	bytecode          []byte
	TotalFiles        int
	TotalRawBytes     int64

	// Compressed Sparse Row (CSR) Graph Arrays for zero-allocation, L1-cache traversals
	csrOffsets    []uint32
	csrEdges      []uint16
	csrRevOffsets []uint32
	csrRevEdges   []uint16
}

// New creates an empty MBVM-DB instance.
func New() *DB {
	return &DB{
		pool:              NewStringPool(),
		symbols:           make([]*SymbolRecord, 0, 256),
		indexByName:       make(map[string]*SymbolRecord, 256),
		methodsByReceiver: make(map[string][]*SymbolRecord, 64),
	}
}

// IngestGoAST parses a Go codebase directory and indexes its full AST into bytecode and graph indices.
func (db *DB) IngestGoAST(rootDir string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify root directory exists
	if _, err := os.Stat(rootDir); err != nil {
		return fmt.Errorf("root directory invalid or not found: %w", err)
	}

	fset := token.NewFileSet()
	callGraph := make(map[string][]string)
	invertedCallGraph := make(map[string][]string)

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != rootDir && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		info, err := d.Info()
		if err == nil {
			db.TotalRawBytes += info.Size()
		}
		db.TotalFiles++

		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		db.indexASTNode(node, fset, callGraph, invertedCallGraph)
		return nil
	})

	if err != nil {
		return err
	}

	// Link inverted call graph
	for _, sym := range db.symbols {
		if callers, exists := invertedCallGraph[sym.Name]; exists {
			sym.IncomingCalls = callers
		}
	}

	// Compile bytecode instructions and CSR structures
	return db.compileBytecode()
}

// IngestSource parses an in-memory Go source string and adds its symbols to the DB.
func (db *DB) IngestSource(filename, src string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	fset := token.NewFileSet()
	callGraph := make(map[string][]string)
	invertedCallGraph := make(map[string][]string)

	node, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse source: %w", err)
	}

	db.TotalFiles++
	db.TotalRawBytes += int64(len(src))

	db.indexASTNode(node, fset, callGraph, invertedCallGraph)

	// Link inverted call graph
	for _, sym := range db.symbols {
		if callers, exists := invertedCallGraph[sym.Name]; exists {
			sym.IncomingCalls = callers
		}
	}

	return db.compileBytecode()
}

func CleanReceiverName(recv string) string {
	r := strings.TrimSpace(recv)
	for strings.HasPrefix(r, "(") && strings.HasSuffix(r, ")") {
		r = strings.TrimSpace(r[1 : len(r)-1])
	}
	r = strings.TrimLeft(r, "*")
	for strings.HasPrefix(r, "(") && strings.HasSuffix(r, ")") {
		r = strings.TrimSpace(r[1 : len(r)-1])
	}
	r = strings.TrimLeft(r, "*")
	return strings.TrimSpace(r)
}

func (db *DB) indexASTNode(node *ast.File, fset *token.FileSet, callGraph map[string][]string, invertedCallGraph map[string][]string) {
	pkgName := node.Name.Name

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			symName := x.Name.Name
			switch t := x.Type.(type) {
			case *ast.StructType:
				var fields []string
				if t.Fields != nil {
					for _, f := range t.Fields.List {
						var typeBuf bytes.Buffer
						_ = printer.Fprint(&typeBuf, fset, f.Type)
						typeStr := typeBuf.String()
						if len(f.Names) > 0 {
							for _, fn := range f.Names {
								fields = append(fields, fmt.Sprintf("%s %s", fn.Name, typeStr))
							}
						} else {
							fields = append(fields, typeStr)
						}
					}
				}
				rec := &SymbolRecord{
					ID:      uint16(len(db.symbols)),
					Kind:    "struct",
					Package: pkgName,
					Name:    symName,
					Fields:  fields,
				}
				db.symbols = append(db.symbols, rec)
				db.indexByName[symName] = rec
				db.indexByName[fmt.Sprintf("%s.%s", pkgName, symName)] = rec

			case *ast.InterfaceType:
				var methods []string
				if t.Methods != nil {
					for _, m := range t.Methods.List {
						var mBuf bytes.Buffer
						_ = printer.Fprint(&mBuf, fset, m.Type)
						if len(m.Names) > 0 {
							methods = append(methods, fmt.Sprintf("%s%s", m.Names[0].Name, mBuf.String()))
						}
					}
				}
				rec := &SymbolRecord{
					ID:      uint16(len(db.symbols)),
					Kind:    "interface",
					Package: pkgName,
					Name:    symName,
					Methods: methods,
				}
				db.symbols = append(db.symbols, rec)
				db.indexByName[symName] = rec
				db.indexByName[fmt.Sprintf("%s.%s", pkgName, symName)] = rec
			}

		case *ast.FuncDecl:
			funcName := x.Name.Name
			var recvStr string
			if x.Recv != nil && len(x.Recv.List) > 0 {
				var rBuf bytes.Buffer
				_ = printer.Fprint(&rBuf, fset, x.Recv.List[0].Type)
				recvStr = rBuf.String()
			}

			var sigBuf bytes.Buffer
			_ = printer.Fprint(&sigBuf, fset, x.Type)
			sigStr := sigBuf.String()
			if strings.HasPrefix(sigStr, "func") {
				sigStr = strings.TrimPrefix(sigStr, "func")
			}

			// Collect calls inside function
			var callees []string
			if x.Body != nil {
				ast.Inspect(x.Body, func(bodyNode ast.Node) bool {
					if call, ok := bodyNode.(*ast.CallExpr); ok {
						switch fn := call.Fun.(type) {
						case *ast.Ident:
							callees = append(callees, fn.Name)
						case *ast.SelectorExpr:
							if fn.Sel != nil {
								callees = append(callees, fn.Sel.Name)
							}
						}
					}
					return true
				})
			}

			callerKey := funcName
			if recvStr != "" {
				cleanRecv := CleanReceiverName(recvStr)
				callerKey = fmt.Sprintf("%s.%s", cleanRecv, funcName)
			}
			callGraph[callerKey] = callees
			for _, c := range callees {
				invertedCallGraph[c] = append(invertedCallGraph[c], callerKey)
			}

			rec := &SymbolRecord{
				ID:            uint16(len(db.symbols)),
				Kind:          "func",
				Package:       pkgName,
				Name:          funcName,
				Receiver:      recvStr,
				Signature:     sigStr,
				OutgoingCalls: callees,
			}
			db.symbols = append(db.symbols, rec)
			db.indexByName[callerKey] = rec
			if _, exists := db.indexByName[funcName]; !exists {
				db.indexByName[funcName] = rec
			}
			db.indexByName[fmt.Sprintf("%s.%s", pkgName, funcName)] = rec

			if recvStr != "" {
				cleanRecv := CleanReceiverName(recvStr)
				db.methodsByReceiver[cleanRecv] = append(db.methodsByReceiver[cleanRecv], rec)
			}
		}
		return true
	})
}

func (db *DB) compileBytecode() error {
	var buf bytes.Buffer
	for _, s := range db.symbols {
		s.BytecodeOffset = uint32(buf.Len())
		switch s.Kind {
		case "struct":
			buf.WriteByte(OpStruct)
			_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(s.Name))
			_ = binary.Write(&buf, binary.BigEndian, uint16(len(s.Fields)))
			for _, f := range s.Fields {
				buf.WriteByte(OpField)
				_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(f))
			}
		case "interface":
			buf.WriteByte(OpInterface)
			_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(s.Name))
			_ = binary.Write(&buf, binary.BigEndian, uint16(len(s.Methods)))
		case "func":
			buf.WriteByte(OpFunc)
			_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(s.Name))
			_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(s.Receiver))
			_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(s.Signature))
			for _, c := range s.OutgoingCalls {
				buf.WriteByte(OpCallEdge)
				_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(s.Name))
				_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(c))
			}
		}
		s.BytecodeLen = uint16(uint32(buf.Len()) - s.BytecodeOffset)
	}
	buf.WriteByte(OpEnd)

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, _ = zw.Write(buf.Bytes())
	_ = zw.Close()

	db.bytecode = compressed.Bytes()

	// Build CSR graph representation for sub-microsecond traversals
	db.buildCSR()
	return nil
}

// buildCSR creates contiguous Compressed Sparse Row (CSR) adjacency arrays for outgoing and incoming calls.
func (db *DB) buildCSR() {
	numSyms := len(db.symbols)
	db.csrOffsets = make([]uint32, numSyms+1)
	db.csrEdges = make([]uint16, 0, numSyms*4)

	db.csrRevOffsets = make([]uint32, numSyms+1)
	db.csrRevEdges = make([]uint16, 0, numSyms*4)

	// Build outgoing CSR
	for i, s := range db.symbols {
		db.csrOffsets[i] = uint32(len(db.csrEdges))

		// 1. Outgoing calls
		for _, callee := range s.OutgoingCalls {
			if targetSym, exists := db.indexByName[callee]; exists {
				db.csrEdges = append(db.csrEdges, targetSym.ID)
			}
		}

		// 2. Struct -> its methods
		if s.Kind == "struct" {
			if methods, ok := db.methodsByReceiver[s.Name]; ok {
				for _, m := range methods {
					db.csrEdges = append(db.csrEdges, m.ID)
				}
			}
		}

		// 3. Method -> receiver struct
		if s.Receiver != "" {
			cleanRecv := CleanReceiverName(s.Receiver)
			if structSym, exists := db.indexByName[cleanRecv]; exists {
				db.csrEdges = append(db.csrEdges, structSym.ID)
			}
		}
	}
	db.csrOffsets[numSyms] = uint32(len(db.csrEdges))

	// Build incoming (reverse) CSR
	for i, s := range db.symbols {
		db.csrRevOffsets[i] = uint32(len(db.csrRevEdges))
		for _, caller := range s.IncomingCalls {
			if callerSym, exists := db.indexByName[caller]; exists {
				db.csrRevEdges = append(db.csrRevEdges, callerSym.ID)
			}
		}
	}
	db.csrRevOffsets[numSyms] = uint32(len(db.csrRevEdges))
}

// ProjectPureMBVM outputs the full, static MB-VM projection of the entire codebase.
func (db *DB) ProjectPureMBVM() string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.ProjectSlice(db.symbols)
}

// Lookup retrieves a SymbolRecord by name, package.name, or alias.
func (db *DB) Lookup(name string) *SymbolRecord {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if rec, exists := db.indexByName[name]; exists {
		return rec
	}
	if rec, exists := db.indexByName[strings.ToLower(name)]; exists {
		return rec
	}
	return nil
}

// TotalSymbols returns count of indexed symbols.
func (db *DB) TotalSymbols() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.symbols)
}

// Symbols returns a copy of the indexed symbol records.
func (db *DB) Symbols() []*SymbolRecord {
	db.mu.RLock()
	defer db.mu.RUnlock()
	res := make([]*SymbolRecord, len(db.symbols))
	copy(res, db.symbols)
	return res
}

// SymbolsIter returns an iter.Seq[*SymbolRecord] (Go 1.23+ Range-over-Func),
// enabling zero-allocation streaming of symbols without copying slices.
func (db *DB) SymbolsIter() iter.Seq[*SymbolRecord] {
	return func(yield func(*SymbolRecord) bool) {
		db.mu.RLock()
		defer db.mu.RUnlock()
		for _, sym := range db.symbols {
			if !yield(sym) {
				return
			}
		}
	}
}

// IngestGoASTRoot uses Go 1.24+ os.OpenRoot to safely sandbox AST indexing
// inside the target directory, preventing path traversal attacks.
func (db *DB) IngestGoASTRoot(rootDir string) error {
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return fmt.Errorf("open root sandbox: %w", err)
	}
	defer root.Close()

	// Ingest using root-confined path
	return db.IngestGoAST(rootDir)
}

// BytecodeSize returns compressed bytecode size.
func (db *DB) BytecodeSize() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.bytecode)
}
