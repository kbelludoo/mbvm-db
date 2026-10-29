package competitorvalidation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kbelludoo/mbvm-db/internal/impact"
)

// SCIPCatalogue supplies independently parsed call positions, not an edge
// whitelist, to the external SCIP index checker. Real expected answers still
// measure agreement; fixture labels are literal independent answers.
type SCIPCatalogue struct {
	Root         string            `json:"root"`
	SourceSHA256 string            `json:"source_sha256"`
	Targets      []target          `json:"targets"`
	Calls        map[string]string `json:"call_positions"`
}

func ExportSCIPCatalogue(root, output string, fixture bool) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("catalogue root: %w", err)
	}
	if fixture {
		if err := os.Mkdir(abs, 0700); err != nil {
			return fmt.Errorf("fixture requires a new directory: %w", err)
		}
		if err := makeFixture(abs); err != nil {
			return err
		}
	}
	before, err := snapshot(abs)
	if err != nil {
		return err
	}
	g, err := impact.LoadModule(abs)
	if err != nil {
		return fmt.Errorf("catalogue typed inventory: %w", err)
	}
	c, err := makeCatalogue(g)
	if err != nil {
		return err
	}
	out := SCIPCatalogue{Root: abs, SourceSHA256: before, Targets: selectTargets(c, fixture), Calls: c.Calls}
	if fixture {
		truth := fixtureTruth()
		for i := range out.Targets {
			want, ok := truth[out.Targets[i].Name]
			if !ok {
				return fmt.Errorf("fixture target lacks independent label")
			}
			out.Targets[i].Want = want
		}
	}
	after, err := snapshot(abs)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("source changed during catalogue export")
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encode SCIP catalogue: %w", err)
	}
	if err := os.WriteFile(output, b, 0600); err != nil {
		return fmt.Errorf("write SCIP catalogue: %w", err)
	}
	return nil
}
