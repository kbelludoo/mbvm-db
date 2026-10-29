package mbvmdb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// IngestLoLIdle ingests reverse-engineered game assets from LoL Idle into MBVM-DB.
func (db *DB) IngestLoLIdle(farmDir string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// 1. Ingest Game Updates & Patch Notes (novidades.js)
	if err := db.ingestNovidades(filepath.Join(farmDir, "novidades.js")); err != nil {
		return fmt.Errorf("ingest novidades: %w", err)
	}

	// 2. Ingest Champions & Kits (campeoes.js & dados-*.js)
	if err := db.ingestCampeoes(filepath.Join(farmDir, "campeoes.js"), farmDir); err != nil {
		return fmt.Errorf("ingest campeoes: %w", err)
	}

	// 3. Ingest Items & Forge (itens.js)
	if err := db.ingestItens(filepath.Join(farmDir, "itens.js")); err != nil {
		return fmt.Errorf("ingest itens: %w", err)
	}

	// 4. Ingest Zones, Maps & Drops (motor.js)
	if err := db.ingestMotor(filepath.Join(farmDir, "motor.js")); err != nil {
		return fmt.Errorf("ingest motor: %w", err)
	}

	// 5. Ingest Network Endpoints & Protocols (rede.js)
	if err := db.ingestRede(filepath.Join(farmDir, "rede.js")); err != nil {
		return fmt.Errorf("ingest rede: %w", err)
	}

	// 6. Ingest Core Mechanics (Dungeon Covil, Vilemaw, Group Hunt)
	db.ingestMechanics()

	db.compileGameBytecodeLocked()
	return nil
}

func (db *DB) addGameSymbol(sym *SymbolRecord, aliases []string) {
	sym.ID = uint16(len(db.symbols))
	db.symbols = append(db.symbols, sym)
	db.indexByName[strings.ToLower(sym.Name)] = sym
	db.indexByName[fmt.Sprintf("%s.%s", strings.ToLower(sym.Package), strings.ToLower(sym.Name))] = sym
	for _, a := range aliases {
		clean := strings.ToLower(strings.TrimSpace(a))
		if clean != "" {
			if _, exists := db.indexByName[clean]; !exists {
				db.indexByName[clean] = sym
			}
		}
	}
}

func (db *DB) ingestNovidades(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read novidades: %w", err)
	}
	content := string(data)

	reBlock := regexp.MustCompile(`(?s)\{\s*id:\s*['"]([^'"]+)['"],\s*(?:img:\s*['"][^'"]*['"],\s*)?data:\s*['"]([^'"]+)['"],\s*titulo:\s*['"]([^'"]+)['"],\s*itens:\s*\[(.*?)\],\s*\}`)
	matches := reBlock.FindAllStringSubmatch(content, -1)

	reItem := regexp.MustCompile(`\[\s*['"]([^'"]*)['"]\s*,\s*['"]([^'"]*)['"]\s*\]`)

	for _, m := range matches {
		id, dataStr, titulo, itemsRaw := m[1], m[2], m[3], m[4]
		itemMatches := reItem.FindAllStringSubmatch(itemsRaw, -1)
		var fields []string
		for _, im := range itemMatches {
			fields = append(fields, fmt.Sprintf("%s %s", im[1], im[2]))
		}

		sym := &SymbolRecord{
			Kind:      "game_update",
			Package:   "novidades",
			Name:      id,
			Receiver:  dataStr,
			Signature: titulo,
			Fields:    fields,
		}
		aliases := []string{
			id,
			dataStr,
			"update-" + id,
			"patch-" + id,
			titulo,
		}
		if strings.Contains(id, "2026-10-24") {
			aliases = append(aliases, "novidades", "hoje", "forja+7", "mix-cartas", "vilemaw-bau")
		}
		if strings.Contains(id, "2026-10-23") {
			aliases = append(aliases, "covil", "covil-tahm-kench", "fizz", "caranguejo")
		}
		if strings.Contains(id, "2026-10-21") {
			aliases = append(aliases, "nivel-100", "miticos", "freljord", "dragao-anciao", "base-inimiga")
		}
		db.addGameSymbol(sym, aliases)
	}
	return nil
}

func (db *DB) ingestCampeoes(path, farmDir string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read campeoes: %w", err)
	}
	content := string(data)

	reChamp := regexp.MustCompile(`(?s)id:\s*['"]([^'"]+)['"],\s*nome:\s*['"]([^'"]+)['"],\s*titulo:\s*['"]([^'"]+)['"],\s*funcao:\s*['"]([^'"]+)['"],\s*regiao:\s*['"]([^'"]+)['"],\s*disponivel:\s*(true|false),\s*alcance:\s*['"]([^'"]+)['"],\s*resumo:\s*['"]([^'"]+)['"]`)
	matches := reChamp.FindAllStringSubmatch(content, -1)

	for _, m := range matches {
		id, nome, titulo, funcao, regiao, _, alcance, resumo := m[1], m[2], m[3], m[4], m[5], m[6], m[7], m[8]
		sym := &SymbolRecord{
			Kind:      "game_champion",
			Package:   "campeoes",
			Name:      nome,
			Receiver:  fmt.Sprintf("%s (%s)", funcao, regiao),
			Signature: alcance,
			Fields: []string{
				fmt.Sprintf("Título: %s", titulo),
				fmt.Sprintf("Função: %s | Alcance: %s | Região: %s", funcao, alcance, regiao),
				fmt.Sprintf("Resumo: %s", resumo),
			},
		}
		aliases := []string{id, nome, "campeao-" + id, id + "-kit"}
		db.addGameSymbol(sym, aliases)

		dadosPath := filepath.Join(farmDir, fmt.Sprintf("dados-%s.js", id))
		if dData, dErr := os.ReadFile(dadosPath); dErr == nil {
			db.ingestChampDataJSON(nome, string(dData))
		}
	}
	return nil
}

func (db *DB) ingestChampDataJSON(champName, content string) {
	sIdx := strings.Index(content, "{")
	eIdx := strings.LastIndex(content, "}")
	if sIdx == -1 || eIdx == -1 || sIdx >= eIdx {
		return
	}

	var d struct {
		Campeao struct {
			Stats map[string]float64 `json:"stats"`
		} `json:"campeao"`
		Habilidades map[string]struct {
			Nome      string    `json:"nome"`
			Descricao string    `json:"descricao"`
			Recarga   []float64 `json:"recarga"`
		} `json:"habilidades"`
	}

	if err := json.Unmarshal([]byte(content[sIdx:eIdx+1]), &d); err != nil {
		return
	}

	for k, h := range d.Habilidades {
		var fields []string
		fields = append(fields, h.Descricao)
		if len(h.Recarga) > 0 {
			fields = append(fields, fmt.Sprintf("Recarga: %v s", h.Recarga))
		}

		sym := &SymbolRecord{
			Kind:      "game_skill",
			Package:   "habilidades",
			Name:      fmt.Sprintf("%s.%s", champName, k),
			Receiver:  h.Nome,
			Signature: fmt.Sprintf("Tecla %s", k),
			Fields:    fields,
		}
		db.addGameSymbol(sym, []string{
			fmt.Sprintf("%s-%s", strings.ToLower(champName), strings.ToLower(k)),
			fmt.Sprintf("%s.%s", strings.ToLower(champName), strings.ToLower(k)),
			h.Nome,
		})
	}
}

func (db *DB) ingestItens(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read itens: %w", err)
	}
	content := string(data)
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start == -1 || end == -1 || start >= end {
		return fmt.Errorf("invalid ITENS_DADOS array bounds")
	}

	var rawItems []struct {
		ID       string   `json:"id"`
		Nome     string   `json:"nome"`
		Preco    int      `json:"preco"`
		Raridade string   `json:"raridade"`
		Lendario bool     `json:"lendario"`
		Espaco   string   `json:"espaco"`
		Resumo   string   `json:"resumo"`
		AD       float64  `json:"ad"`
		AP       float64  `json:"ap"`
		Vida     float64  `json:"vida"`
		Armadura float64  `json:"armadura"`
		RM       float64  `json:"rm"`
		VA       float64  `json:"va"`
		Critico  float64  `json:"critico"`
		Mana     float64  `json:"mana"`
		De       []string `json:"de"`
	}

	if err := json.Unmarshal([]byte(content[start:end+1]), &rawItems); err != nil {
		return fmt.Errorf("unmarshal itens: %w", err)
	}

	for _, it := range rawItems {
		var stats []string
		if it.AD > 0 {
			stats = append(stats, fmt.Sprintf("+%.0f Dano de Ataque", it.AD))
		}
		if it.AP > 0 {
			stats = append(stats, fmt.Sprintf("+%.0f Poder de Habilidade", it.AP))
		}
		if it.Vida > 0 {
			stats = append(stats, fmt.Sprintf("+%.0f Vida", it.Vida))
		}
		if it.Armadura > 0 {
			stats = append(stats, fmt.Sprintf("+%.0f Armadura", it.Armadura))
		}
		if it.RM > 0 {
			stats = append(stats, fmt.Sprintf("+%.0f Resistência Mágica", it.RM))
		}
		if it.VA > 0 {
			stats = append(stats, fmt.Sprintf("+%.0f%% Vel. Ataque", it.VA*100))
		}
		if it.Critico > 0 {
			stats = append(stats, fmt.Sprintf("+%.0f%% Acerto Crítico", it.Critico*100))
		}
		if it.Mana > 0 {
			stats = append(stats, fmt.Sprintf("+%.0f Mana", it.Mana))
		}
		if it.Resumo != "" {
			stats = append(stats, fmt.Sprintf("Efeito: %s", it.Resumo))
		}
		if len(it.De) > 0 {
			stats = append(stats, fmt.Sprintf("Componentes: %s", strings.Join(it.De, ", ")))
		}

		tierLabel := it.Raridade
		if it.Lendario {
			tierLabel = "Lendário"
		}
		if it.ID == "3004" || it.ID == "3084" || it.Nome == "Coração de Aço" || it.Nome == "Tempestade de Luden" || it.Nome == "Manamune" {
			tierLabel = "Mítico"
		}

		sym := &SymbolRecord{
			Kind:      "game_item",
			Package:   "itens",
			Name:      it.Nome,
			Receiver:  tierLabel,
			Signature: fmt.Sprintf("%s (Preço: %d Ouro)", it.Espaco, it.Preco),
			Fields:    stats,
		}
		db.addGameSymbol(sym, []string{
			it.ID,
			it.Nome,
			"item-" + it.ID,
			"item-" + strings.ToLower(it.Nome),
		})
	}
	return nil
}

func (db *DB) ingestMotor(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read motor: %w", err)
	}
	content := string(data)

	// Robust block-by-block parsing of locations
	reID := regexp.MustCompile(`id:\s*['"]([^'"]+)['"]`)
	reNome := regexp.MustCompile(`nome:\s*['"]([^'"]+)['"]`)
	reFaixa := regexp.MustCompile(`faixa:\s*\[(\d+),\s*(\d+)\]`)
	reNM := regexp.MustCompile(`nm:\s*\[(\d+),\s*(\d+)\]`)
	reNivel := regexp.MustCompile(`nivel:\s*(\d+)`)
	reTier := regexp.MustCompile(`tier:\s*(\d+)`)
	reDesc := regexp.MustCompile(`descricao:\s*['"]([^'"]+)['"]`)
	reChefe := regexp.MustCompile(`chefe:\s*\[\s*['"]([^'"]+)['"]`)

	// Split LOCAIS and LOCAIS_FRELJORD entries
	blocks := strings.Split(content, "{ id:")
	for idx, b := range blocks {
		if idx == 0 {
			continue
		}
		block := "{ id:" + strings.Split(b, "},")[0]

		mID := reID.FindStringSubmatch(block)
		mNome := reNome.FindStringSubmatch(block)
		if len(mID) < 2 || len(mNome) < 2 {
			continue
		}

		id := mID[1]
		nome := mNome[1]

		var fields []string
		mDesc := reDesc.FindStringSubmatch(block)
		if len(mDesc) >= 2 {
			fields = append(fields, mDesc[1])
		}

		sig := "Zona"
		mFaixa := reFaixa.FindStringSubmatch(block)
		if len(mFaixa) >= 3 {
			fields = append(fields, fmt.Sprintf("Faixa Ideal: Nível %s a %s", mFaixa[1], mFaixa[2]))
			sig = fmt.Sprintf("Lv.%s-%s", mFaixa[1], mFaixa[2])
		}

		mNM := reNM.FindStringSubmatch(block)
		if len(mNM) >= 3 {
			fields = append(fields, fmt.Sprintf("Nível dos Monstros: %s a %s", mNM[1], mNM[2]))
		}

		mNivel := reNivel.FindStringSubmatch(block)
		if len(mNivel) >= 2 {
			fields = append(fields, fmt.Sprintf("Requisito de Entrada: Nível %s", mNivel[1]))
		}

		mTier := reTier.FindStringSubmatch(block)
		if len(mTier) >= 2 {
			fields = append(fields, fmt.Sprintf("Tier da Área: %s", mTier[1]))
		}

		mChefe := reChefe.FindStringSubmatch(block)
		if len(mChefe) >= 2 {
			fields = append(fields, fmt.Sprintf("Chefe de Área: %s", mChefe[1]))
		}

		if id == "anciao" || id == "base" || id == "nexus" {
			fields = append(fields, "Drop de Itens Míticos disponível (Coração de Aço, Tempestade de Luden, Manamune)")
		}

		sym := &SymbolRecord{
			Kind:      "game_zone",
			Package:   "mapas",
			Name:      nome,
			Receiver:  id,
			Signature: sig,
			Fields:    fields,
		}
		aliases := []string{
			id,
			nome,
			"mapa-" + id,
			"zona-" + id,
		}
		if id == "anciao" {
			aliases = append(aliases, "dragao-anciao", "dragao_anciao")
		}
		if id == "base" {
			aliases = append(aliases, "base-inimiga", "base_inimiga")
		}
		if id == "nexus" {
			aliases = append(aliases, "nexus-inimigo", "nexus_inimigo")
		}
		if strings.HasPrefix(id, "fj") {
			aliases = append(aliases, "freljord", "gelo")
		}
		db.addGameSymbol(sym, aliases)
	}
	return nil
}

func (db *DB) ingestRede(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read rede: %w", err)
	}
	content := string(data)

	reAPI := regexp.MustCompile(`(?m)pedir\(['"](GET|POST|PUT)['"],\s*['"](/api/[^'"]+)['"]`)
	matches := reAPI.FindAllStringSubmatch(content, -1)

	seen := make(map[string]bool)
	for _, m := range matches {
		method, endpoint := m[1], m[2]
		key := fmt.Sprintf("%s %s", method, endpoint)
		if seen[key] {
			continue
		}
		seen[key] = true

		sym := &SymbolRecord{
			Kind:      "game_api",
			Package:   "rede",
			Name:      endpoint,
			Receiver:  method,
			Signature: "HTTP/2 (JSON)",
			Fields: []string{
				"Headers obrigatórios: Authorization: Bearer <token>, X-Idioma: pt, X-Instancia: <id>",
				"Cloudflare Turnstile WAF ativo em /api/*",
			},
		}
		db.addGameSymbol(sym, []string{
			endpoint,
			strings.ReplaceAll(endpoint, "/api/", "api-"),
		})
	}
	return nil
}

func (db *DB) ingestMechanics() {
	mechanics := []struct {
		Name      string
		Category  string
		Signature string
		Details   []string
		Aliases   []string
	}{
		{
			Name:      "Forja do Ornn até +7",
			Category:  "Ornn",
			Signature: "Atualização 24/10/2026",
			Details: []string{
				"Ornn agora forja itens até o nível +7 (+6 e +7 têm taxa de sucesso reduzida).",
				"Sucesso em +7 é transmitido no chat Global de todos os servidores.",
				"Mix de Cartas: junte 2 cartas da mesma raridade + ouro para tentar raridade superior.",
				"Cartas do mesmo tipo garantem carta desse tipo; falhas acumulam taxa de sucesso (pity).",
			},
			Aliases: []string{"forja", "forja+7", "mix-cartas", "cartas-misteriosas", "ornn"},
		},
		{
			Name:      "Covil do Rei do Rio",
			Category:  "Dungeon",
			Signature: "Nível 30+ (1-4 Jogadores)",
			Details: []string{
				"Dungeon manual em Runeterra, ativada por 4 pedras e manivela no Pântano do Rio.",
				"Chefes: Caranguejo Colossal -> Fizz -> Tahm Kench (O Rei do Rio).",
				"Recompensas: XP de 25 min de caça, Ouro, Dentes de Tahm, Escamas, 12% Lendário, 2% Chapéu do Rei do Rio.",
				"Cooldown: 1 entrada grátis a cada 30 min ou uso de Convite da Sucateira.",
			},
			Aliases: []string{"covil", "tahm-kench", "fizz", "caranguejo-colossal", "dungeon"},
		},
		{
			Name:      "Runeterra Open World",
			Category:  "Mundo Aberto",
			Signature: "Bônus Contínuo",
			Details: []string{
				"Experiência dos monstros: +43% sobre o Summoner's Rift (aumentada em 24/10/2026).",
				"Drop rate: +30% de itens sobre o Summoner's Rift.",
				"Caça em Grupo: cada membro a até 15 tiles de distância dá +15% de XP.",
				"Grupo completo de 4 jogadores concede +45% de XP cumulativo.",
			},
			Aliases: []string{"runeterra", "mundo-aberto", "grupo", "party", "bonus-grupo"},
		},
		{
			Name:      "Circuit Breaker Anti-Cheat Protection",
			Category:  "Segurança",
			Signature: "Go Engine (Invariant #7)",
			Details: []string{
				"Supervisiona rejeições de save e anti-cheat (código 403, 429, recusado: true).",
				"MaxFailures: 2, Cooldown base: 15 min, Cooldown máximo: 60 min, Backoff: 2.0x.",
				"Em trip: move automaticamente para a Fonte (área segura) e suspende requisições de save.",
				"Modo Half-Open: valida sessão via /api/eu antes de rearmar o bot.",
			},
			Aliases: []string{"circuit-breaker", "anti-cheat", "seguranca", "breaker"},
		},
	}

	for _, m := range mechanics {
		sym := &SymbolRecord{
			Kind:      "game_mechanic",
			Package:   "mecanicas",
			Name:      m.Name,
			Receiver:  m.Category,
			Signature: m.Signature,
			Fields:    m.Details,
		}
		db.addGameSymbol(sym, m.Aliases)
	}
}

func (db *DB) compileGameBytecodeLocked() {
	db.CompileGameBytecode()
}
