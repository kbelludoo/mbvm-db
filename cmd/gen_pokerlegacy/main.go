package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	catData, err := os.ReadFile(`c:/Users/kbell/Downloads/pokerlegacybot/pkg/game/catalog.json`)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	type Specie struct {
		ID      int      `json:"id"`
		Name    string   `json:"name"`
		Types   []string `json:"types"`
		BaseExp int      `json:"baseExp"`
	}
	type Cat struct {
		Species map[string]Specie `json:"species"`
	}
	var cat Cat
	_ = json.Unmarshal(catData, &cat)

	var names []string
	for _, sp := range cat.Species {
		names = append(names, sp.Name)
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString("package game\n\n// AllPokerlegacySpeciesCatalog lists all 481 pokemon species\nvar AllPokerlegacySpeciesCatalog = []string{\n")
	for _, n := range names {
		sb.WriteString(fmt.Sprintf("\t%q,\n", n))
	}
	sb.WriteString("}\n")

	_ = os.WriteFile(`c:/Users/kbell/Downloads/pokerlegacybot/pkg/game/all_species_catalog.go`, []byte(sb.String()), 0644)
	fmt.Printf("Gerado all_species_catalog.go com %d espécies.\n", len(names))
}
