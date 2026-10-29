package mbvmdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
)

// OpCodes for Chess Domain
const (
	OpChessOpening  byte = 0x40
	OpChessMove     byte = 0x41
	OpChessPosition byte = 0x42
	OpChessGame     byte = 0x43
)

// ChessOpeningItem represents an opening variation with moves, statistics, and tactical themes.
type ChessOpeningItem struct {
	ECO           string   `json:"eco"`
	Name          string   `json:"name"`
	Moves         string   `json:"moves"`
	FEN           string   `json:"fen,omitempty"`
	WhiteWin      float64  `json:"white_win"`
	DrawRate      float64  `json:"draw_rate"`
	BlackWin      float64  `json:"black_win"`
	Continuations []string `json:"continuations,omitempty"`
	KeyThemes     []string `json:"key_themes,omitempty"`
}

// ChessGameItem represents a real game played (e.g. from Chess.com).
type ChessGameItem struct {
	ID          string   `json:"id"`
	URL         string   `json:"url,omitempty"`
	White       string   `json:"white"`
	WhiteRating int      `json:"white_rating"`
	Black       string   `json:"black"`
	BlackRating int      `json:"black_rating"`
	Result      string   `json:"result"`
	ECO         string   `json:"eco,omitempty"`
	OpeningName string   `json:"opening_name,omitempty"`
	TimeClass   string   `json:"time_class,omitempty"`
	Date        string   `json:"date,omitempty"`
	Moves       []string `json:"moves"`
	PGN         string   `json:"pgn,omitempty"`
}

// IngestChessOpenings indexes chess openings and their move continuations into the CSR graph.
func (db *DB) IngestChessOpenings(openings []ChessOpeningItem) {
	db.mu.Lock()
	defer db.mu.Unlock()

	for _, o := range openings {
		var details []string
		details = append(details, fmt.Sprintf("ECO: %s", o.ECO))
		details = append(details, fmt.Sprintf("Move Sequence: %s", o.Moves))
		if o.FEN != "" {
			details = append(details, fmt.Sprintf("FEN: %s", o.FEN))
		}
		details = append(details, fmt.Sprintf("Stats: White %.1f%% | Draw %.1f%% | Black %.1f%%", o.WhiteWin, o.DrawRate, o.BlackWin))

		if len(o.Continuations) > 0 {
			details = append(details, fmt.Sprintf("Continuations: %s", strings.Join(o.Continuations, ", ")))
		}
		if len(o.KeyThemes) > 0 {
			details = append(details, fmt.Sprintf("Tactical Themes: %s", strings.Join(o.KeyThemes, "; ")))
		}

		// Normalize name and aliases
		cleanMoves := normalizeMoveKey(o.Moves)
		var outgoing []string
		for _, c := range o.Continuations {
			cand := strings.TrimSpace(c)
			outgoing = append(outgoing, cand)
			outgoing = append(outgoing, fmt.Sprintf("%s -> %s", cleanMoves, cand))
		}

		rec := &SymbolRecord{
			ID:            uint16(len(db.symbols)),
			Kind:          "chess_opening",
			Package:       o.ECO,
			Name:          o.Name,
			Receiver:      o.ECO,
			Signature:     fmt.Sprintf("W:%.0f%% D:%.0f%% B:%.0f%% [%s]", o.WhiteWin, o.DrawRate, o.BlackWin, o.ECO),
			Fields:        details,
			OutgoingCalls: outgoing,
		}
		db.symbols = append(db.symbols, rec)

		// Index aliases for instant lookups
		aliases := []string{
			o.ECO,
			strings.ToLower(o.ECO),
			o.Name,
			strings.ToLower(o.Name),
			cleanMoves,
			fmt.Sprintf("%s: %s", o.ECO, o.Name),
		}

		// Add variation sub-name aliases (e.g. "Najdorf", "Dragon", "Berlin", "Italian")
		if parts := strings.Split(o.Name, ":"); len(parts) > 1 {
			aliases = append(aliases, strings.TrimSpace(parts[1]))
			aliases = append(aliases, strings.ToLower(strings.TrimSpace(parts[1])))
		}

		for _, alias := range aliases {
			clean := strings.ToLower(strings.TrimSpace(alias))
			if clean != "" {
				if _, exists := db.indexByName[clean]; !exists {
					db.indexByName[clean] = rec
				}
			}
		}
	}
}

// IngestChessGames indexes player games from Chess.com and constructs move-tree transitions.
func (db *DB) IngestChessGames(games []ChessGameItem) {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Track move transitions across games to build frequency and win rates
	type moveTransition struct {
		fromMove string
		move     string
		whiteWin int
		blackWin int
		draw     int
		total    int
	}
	transitions := make(map[string]*moveTransition)

	for _, g := range games {
		var details []string
		details = append(details, fmt.Sprintf("White: %s (%d)", g.White, g.WhiteRating))
		details = append(details, fmt.Sprintf("Black: %s (%d)", g.Black, g.BlackRating))
		details = append(details, fmt.Sprintf("Result: %s", g.Result))
		if g.ECO != "" {
			details = append(details, fmt.Sprintf("ECO: %s", g.ECO))
		}
		if g.OpeningName != "" {
			details = append(details, fmt.Sprintf("Opening: %s", g.OpeningName))
		}
		if g.TimeClass != "" {
			details = append(details, fmt.Sprintf("Time Class: %s", g.TimeClass))
		}
		if g.URL != "" {
			details = append(details, fmt.Sprintf("Chess.com URL: %s", g.URL))
		}
		if len(g.Moves) > 0 {
			maxMoves := len(g.Moves)
			if maxMoves > 20 {
				maxMoves = 20
			}
			details = append(details, fmt.Sprintf("First Moves: %s", strings.Join(g.Moves[:maxMoves], " ")))
		}

		gameTitle := fmt.Sprintf("%s vs %s [%s]", g.White, g.Black, g.Result)
		rec := &SymbolRecord{
			ID:        uint16(len(db.symbols)),
			Kind:      "chess_game",
			Package:   "chess.com",
			Name:      gameTitle,
			Receiver:  g.TimeClass,
			Signature: fmt.Sprintf("%d vs %d (%s)", g.WhiteRating, g.BlackRating, g.Result),
			Fields:    details,
		}
		db.symbols = append(db.symbols, rec)

		// Aliases
		gameAliases := []string{
			g.ID,
			strings.ToLower(g.White),
			strings.ToLower(g.Black),
			fmt.Sprintf("%s_vs_%s", strings.ToLower(g.White), strings.ToLower(g.Black)),
		}
		for _, a := range gameAliases {
			if a != "" {
				if _, exists := db.indexByName[a]; !exists {
					db.indexByName[a] = rec
				}
			}
		}

		// Process move transitions up to ply 24 (12 full moves)
		prevStep := "start"
		limit := len(g.Moves)
		if limit > 24 {
			limit = 24
		}
		for i := 0; i < limit; i++ {
			mv := g.Moves[i]
			tKey := fmt.Sprintf("%s->%s", prevStep, mv)
			trans, exists := transitions[tKey]
			if !exists {
				trans = &moveTransition{
					fromMove: prevStep,
					move:     mv,
				}
				transitions[tKey] = trans
			}
			trans.total++
			if g.Result == "1-0" {
				trans.whiteWin++
			} else if g.Result == "0-1" {
				trans.blackWin++
			} else {
				trans.draw++
			}
			prevStep = mv
		}
	}

	// Register move transition nodes
	for _, tr := range transitions {
		wPct := float64(tr.whiteWin) / float64(tr.total) * 100
		bPct := float64(tr.blackWin) / float64(tr.total) * 100
		dPct := float64(tr.draw) / float64(tr.total) * 100

		nodeName := fmt.Sprintf("Move:%s", tr.move)
		sym := &SymbolRecord{
			ID:        uint16(len(db.symbols)),
			Kind:      "chess_move",
			Package:   "transitions",
			Name:      nodeName,
			Receiver:  tr.fromMove,
			Signature: fmt.Sprintf("%d games (W:%.0f%% D:%.0f%% B:%.0f%%)", tr.total, wPct, dPct, bPct),
			Fields: []string{
				fmt.Sprintf("From Move: %s", tr.fromMove),
				fmt.Sprintf("Move: %s", tr.move),
				fmt.Sprintf("Total Games: %d", tr.total),
				fmt.Sprintf("Score: White %d | Black %d | Draw %d", tr.whiteWin, tr.blackWin, tr.draw),
			},
			IncomingCalls: []string{fmt.Sprintf("Move:%s", tr.fromMove)},
		}
		db.symbols = append(db.symbols, sym)

		alias := strings.ToLower(tr.move)
		if _, exists := db.indexByName[alias]; !exists {
			db.indexByName[alias] = sym
		}
		db.indexByName[strings.ToLower(nodeName)] = sym
	}
}

// IngestChessPGN parses raw PGN text and ingests all games into the database.
func (db *DB) IngestChessPGN(pgnData string) error {
	games := ParsePGN(pgnData)
	if len(games) == 0 {
		return fmt.Errorf("no valid games found in PGN")
	}
	db.IngestChessGames(games)
	return nil
}

// CompileChessBytecode encodes all chess symbols into the MBVM bytecode and builds CSR graph arrays.
func (db *DB) CompileChessBytecode() {
	db.mu.Lock()
	defer db.mu.Unlock()

	var buf bytes.Buffer
	for _, s := range db.symbols {
		if strings.HasPrefix(s.Kind, "chess_") {
			var op byte
			switch s.Kind {
			case "chess_opening":
				op = OpChessOpening
			case "chess_move":
				op = OpChessMove
			case "chess_game":
				op = OpChessGame
			default:
				op = OpChessPosition
			}

			buf.WriteByte(op)
			_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(s.Name))
			_ = binary.Write(&buf, binary.BigEndian, uint16(len(s.Fields)))
			for _, f := range s.Fields {
				buf.WriteByte(OpChessPosition)
				_ = binary.Write(&buf, binary.BigEndian, db.pool.Intern(f))
			}
		}
	}
	db.bytecode = append(db.bytecode, buf.Bytes()...)
	db.buildCSR()
}

func normalizeMoveKey(m string) string {
	cleaned := regexp.MustCompile(`\d+\.+`).ReplaceAllString(m, "")
	fields := strings.Fields(cleaned)
	return strings.Join(fields, "_")
}

// ParsePGN parses a multi-game PGN string into structured ChessGameItems.
func ParsePGN(pgnText string) []ChessGameItem {
	var games []ChessGameItem
	blocks := strings.Split(pgnText, "\n\n[Event ")

	for i, b := range blocks {
		text := b
		if i > 0 {
			text = "[Event " + text
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		item := ChessGameItem{}
		lines := strings.Split(text, "\n")
		var moveLines []string
		inMoves := false

		reTag := regexp.MustCompile(`^\[([A-Za-z0-9_]+)\s+"(.*)"\]`)
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l == "" {
				inMoves = true
				continue
			}
			if strings.HasPrefix(l, "[") && !inMoves {
				m := reTag.FindStringSubmatch(l)
				if len(m) == 3 {
					k, v := m[1], m[2]
					switch strings.ToLower(k) {
					case "white":
						item.White = v
					case "black":
						item.Black = v
					case "result":
						item.Result = v
					case "whiteelo":
						var rating int
						_, _ = fmt.Sscanf(v, "%d", &rating)
						item.WhiteRating = rating
					case "blackelo":
						var rating int
						_, _ = fmt.Sscanf(v, "%d", &rating)
						item.BlackRating = rating
					case "eco":
						item.ECO = v
					case "opening":
						item.OpeningName = v
					case "timecontrol", "time_class":
						item.TimeClass = v
					case "date", "utcdate":
						item.Date = v
					case "link", "url":
						item.URL = v
					}
				}
			} else {
				inMoves = true
				moveLines = append(moveLines, l)
			}
		}

		// Parse move line tokens
		allMovesStr := strings.Join(moveLines, " ")
		// Remove {comments} and (variations)
		allMovesStr = regexp.MustCompile(`\{[^}]*\}`).ReplaceAllString(allMovesStr, "")
		allMovesStr = regexp.MustCompile(`\([^)]*\)`).ReplaceAllString(allMovesStr, "")
		// Remove move numbers like 1. or 1...
		allMovesStr = regexp.MustCompile(`\d+\.+`).ReplaceAllString(allMovesStr, "")

		rawTokens := strings.Fields(allMovesStr)
		var moves []string
		for _, tok := range rawTokens {
			tok = strings.TrimSpace(tok)
			if tok == "1-0" || tok == "0-1" || tok == "1/2-1/2" || tok == "*" {
				if item.Result == "" {
					item.Result = tok
				}
				continue
			}
			if tok != "" {
				moves = append(moves, tok)
			}
		}

		if len(moves) > 0 || item.White != "" {
			if item.ID == "" {
				item.ID = fmt.Sprintf("game_%d_%s_%s", len(games)+1, item.White, item.Black)
			}
			item.Moves = moves
			games = append(games, item)
		}
	}

	return games
}
