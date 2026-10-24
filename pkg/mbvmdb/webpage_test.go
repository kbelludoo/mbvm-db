package mbvmdb_test

import (
	"strings"
	"testing"

	"github.com/kbelludoo/mbvm-db/pkg/mbvmdb"
)

func TestIngestWebPageAndSlice(t *testing.T) {
	page := mbvmdb.WebPageData{
		URL:        "https://news.ycombinator.com/login",
		Title:      "Hacker News Login",
		StatusCode: 200,
		Domain:     "news.ycombinator.com",
		Forms: []mbvmdb.WebForm{
			{
				ID:     "login_form",
				Action: "/login",
				Method: "POST",
				Fields: []mbvmdb.WebFormField{
					{Name: "acct", Type: "text", Placeholder: "username", Ref: "#ref:1"},
					{Name: "pw", Type: "password", Placeholder: "password", Ref: "#ref:2"},
					{Name: "submit", Type: "submit", Value: "login", Ref: "#ref:3"},
				},
			},
			{
				ID:     "create_account_form",
				Action: "/user/create",
				Method: "POST",
				Fields: []mbvmdb.WebFormField{
					{Name: "new_acct", Type: "text", Placeholder: "desired username", Ref: "#ref:4"},
					{Name: "new_pw", Type: "password", Placeholder: "password", Ref: "#ref:5"},
					{Name: "create", Type: "submit", Value: "create account", Ref: "#ref:6"},
				},
			},
		},
		Elements: []mbvmdb.WebInteractiveElement{
			{Ref: "#ref:7", Tag: "a", Type: "link", Text: "Forgot your password?", Href: "/forgot"},
			{Ref: "#ref:8", Tag: "a", Type: "link", Text: "Guidelines", Href: "/newsguidelines.html"},
			{Ref: "#ref:9", Tag: "a", Type: "link", Text: "FAQ", Href: "/newsfaq.html"},
			{Ref: "#ref:10", Tag: "a", Type: "link", Text: "Security", Href: "/security.html"},
		},
		Sections: map[string]string{
			"LoginNotice": "Please enter your username and password to log in to Y Combinator Hacker News.",
		},
	}

	db, bin, err := mbvmdb.CompileWebPageToBytecode(page)
	if err != nil {
		t.Fatalf("CompileWebPageToBytecode failed: %v", err)
	}

	if len(bin) == 0 {
		t.Fatalf("expected non-empty bytecode, got 0 bytes")
	}

	// 1. Test Goal Slicing: AI wants to "login"
	slice := db.ExtractSlice("Form_login_form", 2)
	proj := db.ProjectSlice(slice.Symbols)

	t.Logf("═════════════════════════════════════════════════════════════════════════════")
	t.Logf("⚡ [TESTE REAL] WEB PAGE MBVM-DB COMPILATION & SUB-GRAPH SLICING")
	t.Logf("═════════════════════════════════════════════════════════════════════════════")
	t.Logf("🌐 Página: %s (Título: %s)", page.URL, page.Title)
	t.Logf("📦 Bytecode gerado: %d bytes (vs ~15.000 B HTML)", len(bin))
	t.Logf("🎯 Slice fatiado com foco em 'login' (apenas 2 nós):")
	t.Logf("Tokens estimados do slice: ~%d tokens (vs 1.050 tokens no CSO inteiro!)", slice.TokensEstimate)
	t.Logf("Projeção entregue à IA:\n%s", proj)
	t.Logf("═════════════════════════════════════════════════════════════════════════════")

	if !strings.Contains(proj, "type Form_login_form struct") {
		t.Fatalf("expected Form_login_form in slice, got:\n%s", proj)
	}
	if !strings.Contains(proj, "func (*Form_login_form) Submit") {
		t.Fatalf("expected Submit method in slice, got:\n%s", proj)
	}
	// Verify other form is NOT in the slice (zero noise)
	if strings.Contains(proj, "Form_create_account_form") {
		t.Fatalf("expected noise Form_create_account_form to be filtered out of login slice")
	}

	// 2. Test Token Budget Slicing
	budgetSlice := db.ExtractBudgetSlice("Form_login_form", 2, 80)
	if budgetSlice.TokensEstimate > 80 && len(budgetSlice.Symbols) > 1 {
		t.Fatalf("expected tokens <= 80, got %d", budgetSlice.TokensEstimate)
	}
}
