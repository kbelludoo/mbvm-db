package competitorvalidation

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/kbelludoo/mbvm-db/internal/impact"
)

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type span struct {
	Start position `json:"start"`
	End   position `json:"end"`
}
type document struct {
	URI string `json:"uri"`
}
type locationParams struct {
	TextDocument document `json:"textDocument"`
	Position     position `json:"position"`
}
type hierarchyItem struct {
	Name           string `json:"name"`
	Kind           int    `json:"kind"`
	URI            string `json:"uri"`
	Range          span   `json:"range"`
	SelectionRange span   `json:"selectionRange"`
	Detail         string `json:"detail,omitempty"`
}
type incomingCall struct {
	From       hierarchyItem `json:"from"`
	FromRanges []span        `json:"fromRanges"`
}
type target struct {
	Name     string        `json:"symbol"`
	URI      string        `json:"uri"`
	Position position      `json:"position"`
	Want     []string      `json:"expected_callers"`
	Item     hierarchyItem `json:"prepared_item"`
}
type catalogue struct {
	Targets map[string]target
	Calls   map[string]string
}

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func uriPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parse file URI: %w", err)
	}
	if u.Scheme != "file" || u.Host != "" {
		return "", fmt.Errorf("unsupported file URI %q", uri)
	}
	p := u.Path
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.Clean(filepath.FromSlash(p)), nil
}

func point(src []byte, offset int) position {
	before := src[:offset]
	line := strings.Count(string(before), "\n")
	start := strings.LastIndexByte(string(before), '\n') + 1
	return position{line, len(utf16.Encode([]rune(string(src[start:offset]))))}
}

func locKey(path string, p position) string {
	return strings.ToLower(filepath.Clean(path)) + ":" + strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Character)
}

func splitPosition(p string) (string, int, int, error) {
	last := strings.LastIndexByte(p, ':')
	if last < 0 {
		return "", 0, 0, fmt.Errorf("invalid source position %q", p)
	}
	col, e := strconv.Atoi(p[last+1:])
	if e != nil {
		return "", 0, 0, fmt.Errorf("source column: %w", e)
	}
	prev := strings.LastIndexByte(p[:last], ':')
	if prev < 0 {
		return "", 0, 0, fmt.Errorf("missing source line %q", p)
	}
	line, e := strconv.Atoi(p[prev+1 : last])
	if e != nil {
		return "", 0, 0, fmt.Errorf("source line: %w", e)
	}
	return p[:prev], line, col, nil
}

func calleeIdent(e ast.Expr) *ast.Ident {
	switch x := e.(type) {
	case *ast.Ident:
		return x
	case *ast.SelectorExpr:
		return x.Sel
	case *ast.IndexExpr:
		return calleeIdent(x.X)
	case *ast.IndexListExpr:
		return calleeIdent(x.X)
	case *ast.ParenExpr:
		return calleeIdent(x.X)
	}
	return nil
}

func makeCatalogue(g *impact.ModuleGraph) (*catalogue, error) {
	c := &catalogue{Targets: map[string]target{}, Calls: map[string]string{}}
	byDecl := map[string]impact.Symbol{}
	files := map[string]bool{}
	for _, pkg := range g.Packages {
		for _, s := range pkg.Symbols {
			path, _, _, err := splitPosition(s.Position)
			if err != nil {
				return nil, err
			}
			files[path] = true
			byDecl[s.Position] = s
		}
	}
	for path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read declaration source: %w", err)
		}
		fs := token.NewFileSet()
		f, err := parser.ParseFile(fs, path, src, 0)
		if err != nil {
			return nil, fmt.Errorf("parse declaration source: %w", err)
		}
		for _, decl := range f.Decls {
			d, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			s, ok := byDecl[fs.Position(d.Pos()).String()]
			if !ok {
				continue
			}
			name := s.Qualified()
			if s.Receiver == "" && s.Name != "init" {
				r, err := g.Impact(name)
				if err != nil {
					return nil, err
				}
				c.Targets[name] = target{Name: name, URI: fileURI(path), Position: point(src, fs.Position(d.Name.Pos()).Offset), Want: r.DirectCallers}
			}
			ast.Inspect(d.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id := calleeIdent(call.Fun)
				if id == nil {
					return true
				}
				p := point(src, fs.Position(id.Pos()).Offset)
				c.Calls[locKey(path, p)] = name
				return true
			})
		}
	}
	return c, nil
}

func selectTargets(c *catalogue, all bool) []target {
	names := make([]string, 0, len(c.Targets))
	for name := range c.Targets {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int {
		x, y := sha256.Sum256([]byte("competitor-v1:"+a)), sha256.Sum256([]byte("competitor-v1:"+b))
		return strings.Compare(fmt.Sprintf("%x", x), fmt.Sprintf("%x", y))
	})
	var out []target
	with, without := 0, 0
	for _, name := range names {
		t := c.Targets[name]
		if len(t.Want) > 0 {
			if with >= 6 && !all {
				continue
			}
			with++
		} else {
			if without >= 6 && !all {
				continue
			}
			without++
		}
		out = append(out, t)
	}
	return out
}

func (c *catalogue) normalize(calls []incomingCall, targetName string) ([]string, int, error) {
	set := map[string]bool{}
	dropped := 0
	for _, call := range calls {
		path, err := uriPath(call.From.URI)
		if err != nil {
			return nil, 0, err
		}
		for _, r := range call.FromRanges {
			name, ok := c.Calls[locKey(path, r.Start)]
			if !ok || name == targetName {
				dropped++
				continue
			}
			set[name] = true
		}
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	slices.Sort(out)
	return out, dropped, nil
}
