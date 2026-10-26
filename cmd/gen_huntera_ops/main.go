package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	opData, _ := os.ReadFile(`c:/Users/kbell/Downloads/huntera/huntera-go/pkg/protocol/opcodes.json`)
	type Ops struct {
		Incoming map[string]int `json:"incoming"`
		Outgoing map[string]int `json:"outgoing"`
	}
	var ops Ops
	_ = json.Unmarshal(opData, &ops)

	var allNames []string
	for k := range ops.Incoming {
		allNames = append(allNames, k)
	}
	for k := range ops.Outgoing {
		allNames = append(allNames, k)
	}
	sort.Strings(allNames)

	var sb strings.Builder
	sb.WriteString("package protocol\n\n// AllKnownOpcodesCatalog references all 374 opcode identifiers.\nvar AllKnownOpcodesCatalog = []string{\n")
	for _, n := range allNames {
		sb.WriteString(fmt.Sprintf("\t%q,\n", n))
	}
	sb.WriteString("}\n")

	_ = os.WriteFile(`c:/Users/kbell/Downloads/huntera/huntera-go/pkg/protocol/opcodes_catalog_table.go`, []byte(sb.String()), 0644)
	fmt.Printf("Gerado opcodes_catalog_table.go com %d opcodes.\n", len(allNames))
}
