package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kbelludoo/mbvm-db/pkg/mbvmdb"
)

func main() {
	fmt.Println("🚀 [MBVM-DB] Gerando bancos de dados de jogo (game.mbvmdb) em todos os bots...")

	// 1. bot leveling go
	compileBotLeveling()

	// 2. baiak-idle-bot-main
	compileBaiakIdle()

	// 3. pokebot go
	compilePokebot()

	// 4. pokeidleworld go
	compilePokeidleworld()

	// 5. pokerlegacybot
	compilePokerlegacy()

	// 6. tibia idle
	compileTibiaIdle()

	// 7. luxidlego
	compileLuxidle()

	// 8. farmbot go
	compileFarmbot()

	fmt.Println("✅ Todos os bots agora possuem seus bancos compilados!")
}

func compileBotLeveling() {
	db := mbvmdb.New()
	protoFiles := []string{
		`c:/Users/kbell/Downloads/bot leveling go/proto_part_0.bin`,
		`c:/Users/kbell/Downloads/bot leveling go/proto_part_4.bin`,
		`c:/Users/kbell/Downloads/bot leveling go/leveling_v1_character.proto.bin`,
	}
	for _, pf := range protoFiles {
		_ = db.IngestProtoDescriptor(pf)
	}
	db.CompileGameBytecode()
	out := `c:/Users/kbell/Downloads/bot leveling go/game.mbvmdb`
	_ = db.SaveToFile(out)
	fmt.Printf("✓ [bot leveling go] %s compilado (%d símbolos)\n", filepath.Base(out), db.TotalSymbols())
}

func compileBaiakIdle() {
	data, err := os.ReadFile(`c:/Users/kbell/Downloads/baiak-idle-bot-main/baiak-bot-go/pkg/game/engine_hunts.json`)
	if err != nil {
		fmt.Printf("Baiak err: %v\n", err)
		return
	}
	type HuntInfo struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		MinLevel int      `json:"minLevel"`
		Monsters []string `json:"monsters"`
		AvgExp   float64  `json:"avgExp"`
		GoldKill float64  `json:"goldKill"`
		AvgDmg   float64  `json:"avgDmg"`
	}
	type EngineData struct {
		Hunts []HuntInfo `json:"hunts"`
	}
	var ed EngineData
	_ = json.Unmarshal(data, &ed)

	var items []mbvmdb.GameCatalogItem
	for _, h := range ed.Hunts {
		mName := strings.Join(h.Monsters, ", ")
		items = append(items, mbvmdb.GameCatalogItem{
			GateID:        h.ID,
			GateName:      h.Name,
			MonsterID:     h.ID,
			MonsterName:   mName,
			Difficulty:    fmt.Sprintf("MinLevel %d | Exp: %.0f | Gold: %.0f", h.MinLevel, h.AvgExp, h.GoldKill),
			RequiredLevel: uint32(h.MinLevel),
		})
	}

	db := mbvmdb.New()
	db.IngestCatalogEntries(items)
	db.CompileGameBytecode()
	out := `c:/Users/kbell/Downloads/baiak-idle-bot-main/game.mbvmdb`
	_ = db.SaveToFile(out)
	out2 := `c:/Users/kbell/Downloads/baiak-idle-bot-main/baiak-bot-go/game.mbvmdb`
	_ = db.SaveToFile(out2)
	fmt.Printf("✓ [baiak-idle-bot-main] %s compilado (%d símbolos)\n", filepath.Base(out), db.TotalSymbols())
}

func compilePokebot() {
	data, err := os.ReadFile(`c:/Users/kbell/Downloads/pokebot go/pkg/game/data/hunts.json`)
	if err != nil {
		return
	}
	type Hunt struct {
		Slug         string   `json:"slug"`
		Nome         string   `json:"nome"`
		Descricao    string   `json:"descricao"`
		Tier         int      `json:"tier"`
		Pokes        []string `json:"pokes"`
		Nivel        []int    `json:"nivel"`
		KillsPorHora int      `json:"killsPorHora"`
	}
	var hunts []Hunt
	_ = json.Unmarshal(data, &hunts)

	var items []mbvmdb.GameCatalogItem
	for _, h := range hunts {
		reqLvl := uint32(1)
		if len(h.Nivel) > 0 {
			reqLvl = uint32(h.Nivel[0])
		}
		items = append(items, mbvmdb.GameCatalogItem{
			GateID:        h.Slug,
			GateName:      h.Nome,
			MonsterID:     h.Slug,
			MonsterName:   strings.Join(h.Pokes, ", "),
			Difficulty:    fmt.Sprintf("Tier %d (%s) | %d Kills/h", h.Tier, h.Descricao, h.KillsPorHora),
			RequiredLevel: reqLvl,
		})
	}
	db := mbvmdb.New()
	db.IngestCatalogEntries(items)
	db.CompileGameBytecode()
	out := `c:/Users/kbell/Downloads/pokebot go/game.mbvmdb`
	_ = db.SaveToFile(out)
	fmt.Printf("✓ [pokebot go] %s compilado (%d símbolos)\n", filepath.Base(out), db.TotalSymbols())
}

func compilePokeidleworld() {
	data, err := os.ReadFile(`c:/Users/kbell/Downloads/pokeidleworld go/pkg/game/catalog.json`)
	if err != nil {
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
	type Catalog struct {
		Hunts []Hunt `json:"hunts"`
	}
	var cat Catalog
	_ = json.Unmarshal(data, &cat)

	var items []mbvmdb.GameCatalogItem
	for _, h := range cat.Hunts {
		items = append(items, mbvmdb.GameCatalogItem{
			GateID:        h.Slug,
			GateName:      h.Name,
			MonsterID:     h.Slug,
			MonsterName:   h.Name,
			Difficulty:    fmt.Sprintf("Area %s | Types: %s/%s", h.Area, h.Type1, h.Type2),
			RequiredLevel: uint32(h.Level),
		})
	}
	db := mbvmdb.New()
	db.IngestCatalogEntries(items)
	db.CompileGameBytecode()
	out := `c:/Users/kbell/Downloads/pokeidleworld go/game.mbvmdb`
	_ = db.SaveToFile(out)
	fmt.Printf("✓ [pokeidleworld go] %s compilado (%d símbolos)\n", filepath.Base(out), db.TotalSymbols())
}

func compilePokerlegacy() {
	data, err := os.ReadFile(`c:/Users/kbell/Downloads/pokerlegacybot/pkg/game/catalog.json`)
	if err != nil {
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
	_ = json.Unmarshal(data, &cat)

	var items []mbvmdb.GameCatalogItem
	for _, sp := range cat.Species {
		items = append(items, mbvmdb.GameCatalogItem{
			GateID:        fmt.Sprintf("pokedex-%d", sp.ID),
			GateName:      fmt.Sprintf("#%d", sp.ID),
			MonsterID:     strings.ToLower(sp.Name),
			MonsterName:   sp.Name,
			Difficulty:    fmt.Sprintf("Types: %s | BaseExp: %d", strings.Join(sp.Types, "/"), sp.BaseExp),
			RequiredLevel: uint32(sp.ID),
		})
	}
	db := mbvmdb.New()
	db.IngestCatalogEntries(items)
	db.CompileGameBytecode()
	out := `c:/Users/kbell/Downloads/pokerlegacybot/game.mbvmdb`
	_ = db.SaveToFile(out)
	fmt.Printf("✓ [pokerlegacybot] %s compilado (%d símbolos)\n", filepath.Base(out), db.TotalSymbols())
}

func compileTibiaIdle() {
	data, err := os.ReadFile(`c:/Users/kbell/Downloads/tibia idle/pkg/protocol/hunts.json`)
	if err != nil {
		return
	}
	type Hunt struct {
		ID       int      `json:"id"`
		Title    string   `json:"title"`
		Monsters []string `json:"monsters"`
		Island   string   `json:"island"`
	}
	type HuntsFile struct {
		Hunts []Hunt `json:"hunts"`
	}
	var hf HuntsFile
	_ = json.Unmarshal(data, &hf)

	var items []mbvmdb.GameCatalogItem
	for _, h := range hf.Hunts {
		items = append(items, mbvmdb.GameCatalogItem{
			GateID:        fmt.Sprintf("hunt-%d", h.ID),
			GateName:      h.Title,
			MonsterID:     fmt.Sprintf("hunt-%d", h.ID),
			MonsterName:   strings.Join(h.Monsters, ", "),
			Difficulty:    fmt.Sprintf("Island: %s", h.Island),
			RequiredLevel: uint32(h.ID),
		})
	}
	db := mbvmdb.New()
	db.IngestCatalogEntries(items)
	db.CompileGameBytecode()
	out := `c:/Users/kbell/Downloads/tibia idle/game.mbvmdb`
	_ = db.SaveToFile(out)
	fmt.Printf("✓ [tibia idle] %s compilado (%d símbolos)\n", filepath.Base(out), db.TotalSymbols())
}

func compileLuxidle() {
	// luxidlego uses catalog from codebase & types
	db := mbvmdb.New()
	_ = db.IngestGoAST(`c:/Users/kbell/Downloads/luxidlego/pkg/game`)
	out := `c:/Users/kbell/Downloads/luxidlego/game.mbvmdb`
	_ = db.SaveToFile(out)
	fmt.Printf("✓ [luxidlego] %s compilado (%d símbolos)\n", filepath.Base(out), db.TotalSymbols())
}

func compileFarmbot() {
	db := mbvmdb.New()
	_ = db.IngestGoAST(`c:/Users/kbell/Downloads/farmbot go/pkg/game`)
	out := `c:/Users/kbell/Downloads/farmbot go/game.mbvmdb`
	_ = db.SaveToFile(out)
	fmt.Printf("✓ [farmbot go] %s compilado (%d símbolos)\n", filepath.Base(out), db.TotalSymbols())
}
