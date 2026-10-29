package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	huntsData, err := os.ReadFile(`c:/Users/kbell/Downloads/pokebot go/pkg/game/data/hunts.json`)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	type Hunt struct {
		Slug         string   `json:"slug"`
		Nome         string   `json:"nome"`
		Pokes        []string `json:"pokes"`
		Tier         int      `json:"tier"`
		KillsPorHora int      `json:"killsPorHora"`
	}
	var hunts []Hunt
	_ = json.Unmarshal(huntsData, &hunts)

	var slugs []string
	pokesMap := make(map[string]bool)
	for _, h := range hunts {
		slugs = append(slugs, h.Slug)
		for _, p := range h.Pokes {
			pokesMap[p] = true
		}
	}
	sort.Strings(slugs)

	var pokes []string
	for p := range pokesMap {
		pokes = append(pokes, p)
	}
	sort.Strings(pokes)

	var sb strings.Builder
	sb.WriteString("package game\n\n// AllPokebotHuntsCatalog lists all hunting areas and slugs in Pokebot\nvar AllPokebotHuntsCatalog = []string{\n")
	for _, s := range slugs {
		sb.WriteString(fmt.Sprintf("\t%q,\n", s))
	}
	sb.WriteString("}\n\n// AllPokebotSpawnMonsters lists all active pokemon spawns\nvar AllPokebotSpawnMonsters = []string{\n")
	for _, p := range pokes {
		sb.WriteString(fmt.Sprintf("\t%q,\n", p))
	}
	sb.WriteString("}\n")

	_ = os.WriteFile(`c:/Users/kbell/Downloads/pokebot go/pkg/game/all_hunts_catalog.go`, []byte(sb.String()), 0644)
	fmt.Printf("Gerado all_hunts_catalog.go com %d slugs e %d pokemons.\n", len(slugs), len(pokes))
}
