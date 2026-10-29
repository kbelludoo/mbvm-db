package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/kbelludoo/mbvm-db/pkg/mbvmdb"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolCallResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError"`
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			slog.Error("read error", "err", err)
			break
		}

		line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("\xef\xbb\xbf")))
		if len(line) == 0 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			slog.Error("json unmarshal request", "err", err)
			continue
		}

		resp := handleRPC(&req)
		if resp == nil {
			continue // Notification without response
		}

		respBytes, err := json.Marshal(resp)
		if err != nil {
			slog.Error("json marshal response", "err", err)
			continue
		}

		writer.Write(respBytes)
		writer.WriteByte('\n')
		writer.Flush()
	}
}

func handleRPC(req *rpcRequest) *rpcResponse {
	switch req.Method {
	case "initialize":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "mbvm-db-mcp",
					"version": "1.0.0",
				},
			},
		}

	case "notifications/initialized":
		return nil

	case "ping":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{},
		}

	case "tools/list":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": getTools()},
		}

	case "tools/call":
		res := handleToolCall(req.Params)
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  res,
		}

	default:
		if len(req.ID) > 0 {
			return &rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &rpcError{
					Code:    -32601,
					Message: fmt.Sprintf("Method not found: %s", req.Method),
				},
			}
		}
		return nil
	}
}

func getTools() []map[string]any {
	return []map[string]any{
		{
			"name":        "mbvm_slice",
			"description": "Extract a micro-bytecode subgraph slice for a Go symbol/function, reducing token overhead by 90%+ compared to reading raw files.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"symbol": map[string]any{
						"type":        "string",
						"description": "The function, type, or symbol name to slice (e.g. 'ExtractSlice' or 'DB')",
					},
					"depth": map[string]any{
						"type":        "integer",
						"description": "Graph traversal depth (default 2)",
					},
					"db_path": map[string]any{
						"type":        "string",
						"description": "Path to .mbvmdb binary file or directory (default 'codebase.mbvmdb')",
					},
				},
				"required": []string{"symbol"},
			},
		},
		{
			"name":        "mbvm_slice_budget",
			"description": "Extract a subgraph slice with a strict token budget ceiling and anti-noise pruning.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"symbol": map[string]any{
						"type":        "string",
						"description": "Symbol to slice",
					},
					"max_tokens": map[string]any{
						"type":        "integer",
						"description": "Max token budget (e.g. 500)",
					},
					"depth": map[string]any{
						"type":        "integer",
						"description": "Depth (default 3)",
					},
					"db_path": map[string]any{
						"type":        "string",
						"description": "Path to .mbvmdb binary file or directory (default 'codebase.mbvmdb')",
					},
				},
				"required": []string{"symbol", "max_tokens"},
			},
		},
		{
			"name":        "mbvm_inspect",
			"description": "Inspect total files, indexed symbols, and bytecode volume of a .mbvmdb database.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target": map[string]any{
						"type":        "string",
						"description": "Path to .mbvmdb file or directory (default 'codebase.mbvmdb')",
					},
				},
			},
		},
	}
}

type toolCallArgs struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func handleToolCall(params json.RawMessage) toolCallResult {
	var call toolCallArgs
	if err := json.Unmarshal(params, &call); err != nil {
		return toolCallResult{
			Content: []textContent{{Type: "text", Text: fmt.Sprintf("Error parsing tool call: %v", err)}},
			IsError: true,
		}
	}

	switch call.Name {
	case "mbvm_slice":
		var args struct {
			Symbol string `json:"symbol"`
			Depth  int    `json:"depth"`
			DBPath string `json:"db_path"`
		}
		args.Depth = 2
		args.DBPath = "codebase.mbvmdb"
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return toolCallResult{
				Content: []textContent{{Type: "text", Text: fmt.Sprintf("Invalid arguments: %v", err)}},
				IsError: true,
			}
		}
		if args.Depth <= 0 {
			args.Depth = 2
		}

		db, err := loadDB(args.DBPath)
		if err != nil {
			return toolCallResult{
				Content: []textContent{{Type: "text", Text: fmt.Sprintf("Error loading database: %v", err)}},
				IsError: true,
			}
		}

		slice := db.ExtractSlice(args.Symbol, args.Depth)
		text := fmt.Sprintf("⚡ [MBVM-DB] Símbolo '%s' fatiado (profundidade %d | %d símbolos | ~%d tokens)\n\n%s",
			args.Symbol, args.Depth, len(slice.Symbols), slice.TokensEstimate, db.ProjectSlice(slice.Symbols))

		return toolCallResult{
			Content: []textContent{{Type: "text", Text: text}},
			IsError: false,
		}

	case "mbvm_slice_budget":
		var args struct {
			Symbol    string `json:"symbol"`
			MaxTokens int    `json:"max_tokens"`
			Depth     int    `json:"depth"`
			DBPath    string `json:"db_path"`
		}
		args.Depth = 3
		args.DBPath = "codebase.mbvmdb"
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return toolCallResult{
				Content: []textContent{{Type: "text", Text: fmt.Sprintf("Invalid arguments: %v", err)}},
				IsError: true,
			}
		}
		if args.MaxTokens <= 0 {
			args.MaxTokens = 500
		}
		if args.Depth <= 0 {
			args.Depth = 3
		}

		db, err := loadDB(args.DBPath)
		if err != nil {
			return toolCallResult{
				Content: []textContent{{Type: "text", Text: fmt.Sprintf("Error loading database: %v", err)}},
				IsError: true,
			}
		}

		slice := db.ExtractBudgetSlice(args.Symbol, args.Depth, args.MaxTokens)
		text := fmt.Sprintf("⚡ [MBVM-DB] Símbolo '%s' fatiado com teto de %d tokens (%d símbolos | ~%d tokens)\n\n%s",
			args.Symbol, args.MaxTokens, len(slice.Symbols), slice.TokensEstimate, db.ProjectSlice(slice.Symbols))

		return toolCallResult{
			Content: []textContent{{Type: "text", Text: text}},
			IsError: false,
		}

	case "mbvm_inspect":
		var args struct {
			Target string `json:"target"`
		}
		args.Target = "codebase.mbvmdb"
		if len(call.Arguments) > 0 {
			_ = json.Unmarshal(call.Arguments, &args)
		}
		if args.Target == "" {
			args.Target = "codebase.mbvmdb"
		}

		db, err := loadDB(args.Target)
		if err != nil {
			return toolCallResult{
				Content: []textContent{{Type: "text", Text: fmt.Sprintf("Error loading target: %v", err)}},
				IsError: true,
			}
		}

		text := fmt.Sprintf("📦 MBVM-DB Status: %s\n  • Total de Arquivos: %d\n  • Total de Símbolos Indexados: %d\n  • Tamanho do Bytecode Compilado: %d bytes\n",
			args.Target, db.TotalFiles, db.TotalSymbols(), db.BytecodeSize())

		return toolCallResult{
			Content: []textContent{{Type: "text", Text: text}},
			IsError: false,
		}

	default:
		return toolCallResult{
			Content: []textContent{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", call.Name)}},
			IsError: true,
		}
	}
}

func loadDB(target string) (*mbvmdb.DB, error) {
	if target == "" {
		target = "codebase.mbvmdb"
	}
	if strings.HasSuffix(target, ".mbvmdb") {
		if _, err := os.Stat(target); err == nil {
			db, err := mbvmdb.LoadFromFile(target)
			if err != nil {
				return nil, fmt.Errorf("load from file %s: %w", target, err)
			}
			return db, nil
		}
		// If codebase.mbvmdb exists in current directory fallback
		if _, err := os.Stat("codebase.mbvmdb"); err == nil {
			db, err := mbvmdb.LoadFromFile("codebase.mbvmdb")
			if err != nil {
				return nil, fmt.Errorf("load codebase.mbvmdb: %w", err)
			}
			return db, nil
		}
		target = "."
	}

	db := mbvmdb.New()
	if err := db.IngestGoAST(target); err != nil {
		return nil, fmt.Errorf("ingest go ast: %w", err)
	}
	return db, nil
}
