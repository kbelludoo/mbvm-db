package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	huntsData, err := os.ReadFile(`c:/Users/kbell/Downloads/tibia idle/pkg/protocol/hunts.json`)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	type Hunt struct {
		ID       int      `json:"id"`
		Title    string   `json:"title"`
		Monsters []string `json:"monsters"`
	}
	type HF struct {
		Hunts []Hunt `json:"hunts"`
	}
	var hf HF
	_ = json.Unmarshal(huntsData, &hf)

	monstersMap := make(map[string]bool)
	for _, h := range hf.Hunts {
		for _, m := range h.Monsters {
			monstersMap[m] = true
		}
	}

	var mList []string
	for m := range monstersMap {
		mList = append(mList, m)
	}
	sort.Strings(mList)

	var sb strings.Builder
	sb.WriteString("package protocol\n\n// AllKnownTibiaMonsters maps all active monsters across all caves.\nvar AllKnownTibiaMonsters = []string{\n")
	for _, m := range mList {
		sb.WriteString(fmt.Sprintf("\t%q,\n", m))
	}
	sb.WriteString("}\n")

	_ = os.WriteFile(`c:/Users/kbell/Downloads/tibia idle/pkg/protocol/all_monsters_table.go`, []byte(sb.String()), 0644)
	fmt.Printf("Gerado all_monsters_table.go com %d monstros.\n", len(mList))
}
