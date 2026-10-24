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
	"path/filepath"
	"strings"
	"sync"
)

// DB represents a Micro-Bytecode Virtual Machine Database instance.
type DB struct {
	mu            sync.RWMutex
	pool          *StringPool
	symbols       []*SymbolRecord
	indexByName   map[string]*SymbolRecord
	bytecode      []byte
	TotalFiles    int
	TotalRawBytes int64
}

// New creates an empty MBVM-DB instance.
func New() *DB {
	return &DB{
		pool:        NewStringPool(),
		symbols:     make([]*SymbolRecord, 0, 256),
		indexByName: make(map[string]*SymbolRecord, 256),
	}
}

// IngestGoAST parses a Go codebase directory and indexes its full AST into bytecode and graph indices.
func (db *DB) IngestGoAST(rootDir string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	fset := token.NewFileSet()
	callGraph := make(map[string][]string)
	invertedCallGraph := make(map[string][]string)

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
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
					callerKey = fmt.Sprintf("%s.%s", strings.Trim(recvStr, "*"), funcName)
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
				db.indexByName[funcName] = rec
				db.indexByName[fmt.Sprintf("%s.%s", pkgName, funcName)] = rec
			}
			return true
		})

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

	// Compile bytecode instructions
	return db.compileBytecode()
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
	return nil
}

// ProjectPureMBVM outputs the full, static MB-VM projection of the entire codebase.
func (db *DB) ProjectPureMBVM() string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.ProjectSlice(db.symbols)
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

// BytecodeSize returns compressed bytecode size.
func (db *DB) BytecodeSize() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.bytecode)
}
