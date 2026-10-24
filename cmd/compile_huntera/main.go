package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kbelludoo/mbvm-db/pkg/mbvmdb"
)

func main() {
	fmt.Println("🚀 Compilando game.mbvmdb para Huntera...")

	db := mbvmdb.New()

	// 1. Ingest Opcodes (Incoming & Outgoing network protocol)
	opcodesData, err := os.ReadFile(`c:/Users/kbell/Downloads/huntera/huntera-go/pkg/protocol/opcodes.json`)
	if err == nil {
		type Opcodes struct {
			Incoming map[string]int `json:"incoming"`
			Outgoing map[string]int `json:"outgoing"`
		}
		var ops Opcodes
		if err := json.Unmarshal(opcodesData, &ops); err == nil {
			var incEntries []mbvmdb.GameCatalogItem
			for name, code := range ops.Incoming {
				incEntries = append(incEntries, mbvmdb.GameCatalogItem{
					GateID:        fmt.Sprintf("opcode-in-%d", code),
					GateName:      fmt.Sprintf("OpcodeIn:%s", name),
					MonsterID:     name,
					MonsterName:   fmt.Sprintf("Incoming Opcode %d: %s", code, name),
					Difficulty:    "Network Protocol (Server -> Client)",
					RequiredLevel: uint32(code),
				})
			}
			for name, code := range ops.Outgoing {
				incEntries = append(incEntries, mbvmdb.GameCatalogItem{
					GateID:        fmt.Sprintf("opcode-out-%d", code),
					GateName:      fmt.Sprintf("OpcodeOut:%s", name),
					MonsterID:     name,
					MonsterName:   fmt.Sprintf("Outgoing Opcode %d: %s", code, name),
					Difficulty:    "Network Protocol (Client -> Server)",
					RequiredLevel: uint32(code),
				})
			}
			db.IngestCatalogEntries(incEntries)
			fmt.Printf("✓ %d Opcodes ingeridos\n", len(incEntries))
		}
	}

	// 2. Ingest Catalog Hunts & Monsters
	catData, err := os.ReadFile(`c:/Users/kbell/Downloads/huntera/huntera-go/pkg/bot/catalog.json`)
	if err == nil {
		type CatalogMonster struct {
			Name       string `json:"name"`
			BestiaryID string `json:"bestiaryId"`
		}
		type CatalogHunt struct {
			ID       string           `json:"id"`
			Name     string           `json:"name"`
			Monsters []CatalogMonster `json:"monsters"`
		}
		type HuntCatalog struct {
			Hunts []CatalogHunt `json:"hunts"`
		}
		var cat HuntCatalog
		if err := json.Unmarshal(catData, &cat); err == nil {
			var huntEntries []mbvmdb.GameCatalogItem
			for _, h := range cat.Hunts {
				var mNames []string
				for _, m := range h.Monsters {
					mNames = append(mNames, fmt.Sprintf("%s (%s)", m.Name, m.BestiaryID))
				}
				huntEntries = append(huntEntries, mbvmdb.GameCatalogItem{
					GateID:      h.ID,
					GateName:    h.Name,
					MonsterID:   h.ID,
					MonsterName: strings.Join(mNames, ", "),
					Difficulty:  fmt.Sprintf("Cave ID: %s | Total Mobs: %d", h.ID, len(h.Monsters)),
				})
			}
			db.IngestCatalogEntries(huntEntries)
			fmt.Printf("✓ %d Hunts de Huntera ingeridas\n", len(huntEntries))
		}
	}

	db.CompileGameBytecode()

	targets := []string{
		`c:/Users/kbell/Downloads/huntera/game.mbvmdb`,
		`c:/Users/kbell/Downloads/huntera/huntera-go/game.mbvmdb`,
		`c:/Users/kbell/Downloads/huntera/huntera-bot/game.mbvmdb`,
	}
	for _, t := range targets {
		_ = db.SaveToFile(t)
		fmt.Printf("✓ Gravado: %s (%d símbolos)\n", t, db.TotalSymbols())
	}
}
