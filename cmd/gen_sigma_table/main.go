package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	catData, err := os.ReadFile(`c:/Users/kbell/Downloads/bot go sigma/pkg/bot/catalog.json`)
	if err != nil {
		fmt.Println("err:", err)
		return
	}

	type CatalogSpecies struct {
		ID   int    `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
	}

	type CatalogHunt struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}

	type CatalogBall struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	}

	type CatalogData struct {
		Species []CatalogSpecies `json:"species"`
		Hunts   []CatalogHunt    `json:"hunts"`
		Balls   []CatalogBall    `json:"balls"`
	}

	var cat CatalogData
	_ = json.Unmarshal(catData, &cat)

	slugMap := make(map[string]bool)
	nameMap := make(map[string]bool)

	for _, h := range cat.Hunts {
		slugMap[h.Slug] = true
		nameMap[h.Name] = true
	}
	for _, sp := range cat.Species {
		slugMap[sp.Slug] = true
		nameMap[sp.Name] = true
	}
	for _, b := range cat.Balls {
		slugMap[b.Key] = true
		nameMap[b.Name] = true
	}

	var slugs []string
	for s := range slugMap {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)

	var names []string
	for n := range nameMap {
		names = append(names, n)
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString("package bot\n\n// AllSigmaHuntsAndSpeciesCatalog references all 638 catalog entities\nvar AllSigmaHuntsAndSpeciesCatalog = []string{\n")
	for _, s := range slugs {
		sb.WriteString(fmt.Sprintf("\t%q,\n", s))
	}
	for _, n := range names {
		sb.WriteString(fmt.Sprintf("\t%q,\n", n))
	}
	sb.WriteString("}\n")

	_ = os.WriteFile(`c:/Users/kbell/Downloads/bot go sigma/pkg/bot/all_sigma_catalog.go`, []byte(sb.String()), 0644)
	fmt.Printf("Gerado all_sigma_catalog.go com %d slugs e %d nomes.\n", len(slugs), len(names))
}
