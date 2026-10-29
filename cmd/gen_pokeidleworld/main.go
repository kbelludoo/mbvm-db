package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	catData, err := os.ReadFile(`c:/Users/kbell/Downloads/pokeidleworld go/pkg/game/catalog.json`)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	type Hunt struct {
		Slug  string `json:"slug"`
		Name  string `json:"name"`
		Level int    `json:"level"`
		Area  string `json:"area"`
		Type1 string `json:"type1"`
		Type2 string `json:"type2"`
	}
	type Cat struct {
		Hunts []Hunt `json:"hunts"`
	}
	var cat Cat
	_ = json.Unmarshal(catData, &cat)

	var slugs []string
	for _, h := range cat.Hunts {
		slugs = append(slugs, h.Slug)
	}
	sort.Strings(slugs)

	var sb strings.Builder
	sb.WriteString("package game\n\n// AllPokeidleworldHunts lists all 454 hunts and monster slugs\nvar AllPokeidleworldHunts = []string{\n")
	for _, s := range slugs {
		sb.WriteString(fmt.Sprintf("\t%q,\n", s))
	}
	sb.WriteString("}\n")

	_ = os.WriteFile(`c:/Users/kbell/Downloads/pokeidleworld go/pkg/game/all_world_hunts.go`, []byte(sb.String()), 0644)
	fmt.Printf("Gerado all_world_hunts.go com %d slugs.\n", len(slugs))
}
