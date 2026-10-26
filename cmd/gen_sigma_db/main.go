package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kbelludoo/mbvm-db/pkg/mbvmdb"
)

func main() {
	fmt.Println("🚀 Compilando game.mbvmdb para Bot Go Sigma...")

	catData, err := os.ReadFile(`c:/Users/kbell/Downloads/bot go sigma/pkg/bot/catalog.json`)
	if err != nil {
		fmt.Printf("Erro ao ler catalog.json: %v\n", err)
		return
	}

	type CatalogSpecies struct {
		ID        int      `json:"id"`
		Slug      string   `json:"slug"`
		Name      string   `json:"name"`
		Types     []string `json:"types"`
		HuntLevel int      `json:"huntLevel"`
		Rarity    string   `json:"rarity"`
	}

	type CatalogHunt struct {
		Slug            string `json:"slug"`
		Name            string `json:"name"`
		MinTrainerLevel int    `json:"minTrainerLevel"`
		Region          string `json:"region"`
	}

	type CatalogBall struct {
		ID       int    `json:"id"`
		Key      string `json:"key"`
		Name     string `json:"name"`
		Strength int    `json:"strength"`
	}

	type CatalogData struct {
		Species []CatalogSpecies `json:"species"`
		Hunts   []CatalogHunt    `json:"hunts"`
		Balls   []CatalogBall    `json:"balls"`
	}

	var cat CatalogData
	if err := json.Unmarshal(catData, &cat); err != nil {
		fmt.Printf("Erro unmarshal: %v\n", err)
		return
	}

	db := mbvmdb.New()
	var entries []mbvmdb.GameCatalogItem

	// 1. Hunts (Áreas de Caça)
	for _, h := range cat.Hunts {
		entries = append(entries, mbvmdb.GameCatalogItem{
			GateID:        h.Slug,
			GateName:      h.Name,
			MonsterID:     h.Slug,
			MonsterName:   h.Name,
			Difficulty:    fmt.Sprintf("Region: %s | MinTrainerLevel: %d", h.Region, h.MinTrainerLevel),
			RequiredLevel: uint32(h.MinTrainerLevel),
		})
	}

	// 2. Species (Pokémons e Bestiário)
	for _, sp := range cat.Species {
		entries = append(entries, mbvmdb.GameCatalogItem{
			GateID:        fmt.Sprintf("species-%d", sp.ID),
			GateName:      sp.Slug,
			MonsterID:     sp.Slug,
			MonsterName:   sp.Name,
			Difficulty:    fmt.Sprintf("Types: %s | Rarity: %s", strings.Join(sp.Types, "/"), sp.Rarity),
			RequiredLevel: uint32(sp.HuntLevel),
		})
	}

	// 3. Balls (Pokébolas e Itens)
	for _, b := range cat.Balls {
		entries = append(entries, mbvmdb.GameCatalogItem{
			GateID:        b.Key,
			GateName:      b.Name,
			MonsterID:     b.Key,
			MonsterName:   b.Name,
			Difficulty:    fmt.Sprintf("Ball Strength: %d", b.Strength),
			RequiredLevel: uint32(b.ID),
		})
	}

	db.IngestCatalogEntries(entries)
	db.CompileGameBytecode()

	out := `c:/Users/kbell/Downloads/bot go sigma/game.mbvmdb`
	if err := db.SaveToFile(out); err != nil {
		fmt.Printf("Erro ao salvar game.mbvmdb: %v\n", err)
		return
	}

	fmt.Printf("✓ Gravado %s com sucesso (%d símbolos)\n", out, db.TotalSymbols())
}
