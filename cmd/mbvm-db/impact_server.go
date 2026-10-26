package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kbelludoo/mbvm-db/internal/impact"
)

type impactRequest struct {
	ID     string `json:"id,omitempty"`
	Op     string `json:"op,omitempty"`
	Symbol string `json:"symbol,omitempty"`
}

type impactResponse struct {
	ID          string                        `json:"id,omitempty"`
	Impact      *impact.ModuleImpact          `json:"impact,omitempty"`
	Stats       *impact.ModuleCacheQueryStats `json:"stats,omitempty"`
	Invalidated bool                          `json:"invalidated,omitempty"`
	Error       string                        `json:"error,omitempty"`
}

// runImpactServer keeps one bounded graph cache for one explicitly selected
// module. It is a sequential JSON-lines stdio protocol, not an HTTP service or
// MCP transport. EOF exits cleanly; default CLI interrupt behavior is retained.
func runImpactServer(args []string, input io.Reader, output io.Writer) error {
	flags := flag.NewFlagSet("impact-server", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", ".", "fixed Go module root for this process")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse impact server options: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("impact-server accepts flags only")
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve module root: %w", err)
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		return fmt.Errorf("selected root must contain go.mod: %w", err)
	}
	cache := impact.NewModuleCache(1)
	defer cache.Clear()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var request impactRequest
		response := impactResponse{}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			response.Error = "invalid request: " + err.Error()
		} else {
			var extra impactRequest
			if err := decoder.Decode(&extra); err != io.EOF {
				response.Error = "expected exactly one JSON request per line"
			}
		}
		response.ID = request.ID
		if response.Error == "" {
			switch request.Op {
			case "", "impact":
				if request.Symbol == "" {
					response.Error = "symbol is required"
					break
				}
				result, stats, err := cache.Impact(abs, request.Symbol)
				response.Stats = &stats
				if err != nil {
					response.Error = err.Error()
				} else {
					response.Impact = &result
				}
			case "invalidate":
				if request.Symbol != "" {
					response.Error = "invalidate does not accept a symbol"
					break
				}
				if err := cache.Invalidate(abs); err != nil {
					response.Error = err.Error()
				} else {
					response.Invalidated = true
				}
			default:
				response.Error = "unsupported operation"
			}
		}
		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("write impact response: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read impact requests: %w", err)
	}
	return nil
}
