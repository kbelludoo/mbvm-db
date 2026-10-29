package mbvmdb

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCognitiveMemory_BasicFlow(t *testing.T) {
	cm := NewCognitiveMemory()

	// 1. Seed domain knowledge for github.com
	pb := DomainPlaybook{
		Domain: "github.com",
		FastPaths: map[string]string{
			"search": "https://github.com/search?q={query}",
			"issues": "https://github.com/{repo}/issues",
		},
		KnownSelectors: map[string]string{
			"search_input": "input.header-search-input, input[name='q']",
			"login_button": "a[href*='/login']",
		},
		DismissModals: []string{
			".js-cookie-consent-banner button",
		},
		RequiresWARP: false,
		InsecureTLS:  false,
		SuccessCount: 15,
		FailureCount: 1,
		LastVisited:  time.Now(),
	}

	if err := cm.PutPlaybook(pb); err != nil {
		t.Fatalf("PutPlaybook failed: %v", err)
	}

	// 2. Resolve Action: FastPath
	act, err := cm.ResolveAction("github.com", "search")
	if err != nil {
		t.Fatalf("ResolveAction failed: %v", err)
	}
	if act.Kind != KindFastPath || act.Target != "https://github.com/search?q={query}" {
		t.Errorf("Unexpected fastpath action: %+v", act)
	}

	// 3. Resolve Action: Selector
	actSel, err := cm.ResolveAction("github.com", "search_input")
	if err != nil {
		t.Fatalf("ResolveAction selector failed: %v", err)
	}
	if actSel.Kind != KindSelector || !strings.Contains(actSel.Target, "header-search-input") {
		t.Errorf("Unexpected selector action: %+v", actSel)
	}

	// 4. Resolve Action: Cookie modal dismiss
	actModal, err := cm.ResolveAction("github.com", "cookie_consent")
	if err != nil {
		t.Fatalf("ResolveAction modal failed: %v", err)
	}
	if actModal.Kind != KindDismissModal {
		t.Errorf("Expected KindDismissModal, got: %s", actModal.Kind)
	}

	// 5. Record Feedback (Self-healing)
	err = cm.RecordFeedback(ActionFeedback{
		Domain:    "github.com",
		Action:    "search_input",
		Selector:  "input[data-target='qbsearch-input.inputButtonText']",
		Success:   true,
		LatencyMs: 42,
	})
	if err != nil {
		t.Fatalf("RecordFeedback failed: %v", err)
	}

	updatedPb, ok := cm.GetPlaybook("github.com")
	if !ok {
		t.Fatalf("GetPlaybook returned false")
	}
	if updatedPb.SuccessCount != 16 {
		t.Errorf("Expected SuccessCount=16, got %d", updatedPb.SuccessCount)
	}
	if updatedPb.KnownSelectors["search_input"] != "input[data-target='qbsearch-input.inputButtonText']" {
		t.Errorf("Expected updated selector, got: %s", updatedPb.KnownSelectors["search_input"])
	}

	// 6. Token Slice Prompt Verification
	promptSlice := cm.SliceDomainPrompt("github.com")
	if !strings.Contains(promptSlice, "MBVM Cognitive Memory: github.com") {
		t.Errorf("Prompt slice missing header: %s", promptSlice)
	}
	// Verify token count (approx 1 token per 4 characters)
	tokenEstimate := len(promptSlice) / 4
	if tokenEstimate > 100 {
		t.Errorf("Token slice too large: %d tokens (len=%d)", tokenEstimate, len(promptSlice))
	}
}

func TestCognitiveMemory_SaveAndLoad(t *testing.T) {
	cm := NewCognitiveMemory()

	pb := DomainPlaybook{
		Domain: "gov.br",
		FastPaths: map[string]string{
			"search": "https://www.gov.br/pt-br/busca?q={query}",
		},
		KnownSelectors: map[string]string{
			"search_bar": "#barra-busca-campo",
		},
		RequiresWARP: true,
		InsecureTLS:  true,
		SuccessCount: 42,
		FailureCount: 2,
	}

	if err := cm.PutPlaybook(pb); err != nil {
		t.Fatalf("PutPlaybook failed: %v", err)
	}

	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "cognitive_memory.mbvmdb")

	if err := cm.SaveToFile(savePath); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Load DB from file and verify
	loadedDB, err := LoadFromFile(savePath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	sym := loadedDB.Lookup("mem_gov_br.Playbook")
	if sym == nil {
		t.Fatalf("Symbol mem_gov_br.Playbook not found in loaded DB")
	}
	if len(sym.Fields) == 0 {
		t.Fatalf("Expected fields on Playbook struct, got empty")
	}
}

func BenchmarkCognitiveMemory_ResolveAction(b *testing.B) {
	cm := NewCognitiveMemory()
	_ = cm.PutPlaybook(DomainPlaybook{
		Domain: "google.com",
		FastPaths: map[string]string{
			"search": "https://www.google.com/search?q={query}",
		},
		KnownSelectors: map[string]string{
			"search_input": "textarea[name='q'], input[name='q']",
			"submit":       "input[name='btnK']",
		},
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := cm.ResolveAction("google.com", "search_input")
		if err != nil {
			b.Fatalf("ResolveAction failed: %v", err)
		}
	}
}

func BenchmarkCognitiveMemory_PutPlaybook(b *testing.B) {
	cm := NewCognitiveMemory()
	pb := DomainPlaybook{
		Domain: "google.com",
		FastPaths: map[string]string{
			"search": "https://www.google.com/search?q={query}",
		},
		KnownSelectors: map[string]string{
			"search_input": "textarea[name='q']",
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = cm.PutPlaybook(pb)
	}
}
