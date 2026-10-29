package mbvmdb

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ActionKind defines the category of an action target resolved from cognitive memory.
type ActionKind string

const (
	KindFastPath     ActionKind = "fast_path"
	KindSelector     ActionKind = "selector"
	KindDismissModal ActionKind = "dismiss_modal"
)

// DomainPlaybook stores learned knowledge, successful selectors, routes, and connection profiles for a web domain.
type DomainPlaybook struct {
	Domain         string            `json:"domain"`
	FastPaths      map[string]string `json:"fast_paths"`      // e.g. "search": "https://domain.com/search?q={query}"
	KnownSelectors map[string]string `json:"known_selectors"` // e.g. "search_input": "input[name='q']", "submit": "button[type='submit']"
	DismissModals  []string          `json:"dismiss_modals"`  // Known cookie/popup dismiss selectors
	RequiresWARP   bool              `json:"requires_warp"`   // Datacenter IP blocked; requires Cloudflare WARP
	InsecureTLS    bool              `json:"insecure_tls"`    // Broken x509 chain requiring TLS bypass
	LastStatusCode int               `json:"last_status_code"`
	LastDiagnosis  string            `json:"last_diagnosis"`
	SuccessCount   int               `json:"success_count"`
	FailureCount   int               `json:"failure_count"`
	LastVisited    time.Time         `json:"last_visited"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// ActionFeedback represents real-time feedback from an agent's interaction on a domain.
type ActionFeedback struct {
	Domain    string `json:"domain"`
	Action    string `json:"action"`   // e.g. "search_input", "submit_button", "cookie_consent"
	Selector  string `json:"selector"` // CSS selector attempted
	Success   bool   `json:"success"`
	LatencyMs int64  `json:"latency_ms"`
}

// ResolvedAction is the fast zero-token action target resolved from memory.
type ResolvedAction struct {
	Domain     string     `json:"domain"`
	Intent     string     `json:"intent"`
	Kind       ActionKind `json:"kind"`
	Target     string     `json:"target"` // URL pattern or CSS selector
	Confidence float64    `json:"confidence"`
	Notes      string     `json:"notes,omitempty"`
}

// CognitiveMemory stores and compiles domain navigation knowledge into an MBVM-DB graph.
type CognitiveMemory struct {
	mu        sync.RWMutex
	playbooks map[string]*DomainPlaybook
	db        *DB
}

// NewCognitiveMemory creates a new empty cognitive memory engine.
func NewCognitiveMemory() *CognitiveMemory {
	return &CognitiveMemory{
		playbooks: make(map[string]*DomainPlaybook),
		db:        New(),
	}
}

// PutPlaybook stores or updates a domain playbook and compiles its graph symbols.
func (cm *CognitiveMemory) PutPlaybook(playbook DomainPlaybook) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	domain := strings.ToLower(strings.TrimSpace(playbook.Domain))
	if domain == "" {
		return fmt.Errorf("mbvmdb: domain cannot be empty")
	}

	// Normalize domain if a full URL was provided
	if strings.Contains(domain, "://") {
		if u, err := url.Parse(domain); err == nil && u.Host != "" {
			domain = u.Host
		}
	}

	if playbook.FastPaths == nil {
		playbook.FastPaths = make(map[string]string)
	}
	if playbook.KnownSelectors == nil {
		playbook.KnownSelectors = make(map[string]string)
	}
	if playbook.UpdatedAt.IsZero() {
		playbook.UpdatedAt = time.Now()
	}

	cm.playbooks[domain] = &playbook
	return cm.recompileDomainLocked(domain)
}

// GetPlaybook retrieves a stored domain playbook.
func (cm *CognitiveMemory) GetPlaybook(domain string) (*DomainPlaybook, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	domain = strings.ToLower(strings.TrimSpace(domain))
	if strings.Contains(domain, "://") {
		if u, err := url.Parse(domain); err == nil && u.Host != "" {
			domain = u.Host
		}
	}

	pb, ok := cm.playbooks[domain]
	if !ok {
		return nil, false
	}
	// Return a copy to avoid concurrent mutations
	cp := *pb
	cp.FastPaths = make(map[string]string, len(pb.FastPaths))
	for k, v := range pb.FastPaths {
		cp.FastPaths[k] = v
	}
	cp.KnownSelectors = make(map[string]string, len(pb.KnownSelectors))
	for k, v := range pb.KnownSelectors {
		cp.KnownSelectors[k] = v
	}
	cp.DismissModals = append([]string(nil), pb.DismissModals...)
	return &cp, true
}

// ResolveAction retrieves an exact fast-path URL or CSS selector for a given domain and intent.
// Runs in sub-microsecond latency via CPU lookups with ZERO tokens sent to the LLM.
func (cm *CognitiveMemory) ResolveAction(domain, intent string) (*ResolvedAction, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	domain = strings.ToLower(strings.TrimSpace(domain))
	if strings.Contains(domain, "://") {
		if u, err := url.Parse(domain); err == nil && u.Host != "" {
			domain = u.Host
		}
	}
	normIntent := strings.ToLower(strings.TrimSpace(intent))

	pb, ok := cm.playbooks[domain]
	if !ok {
		return nil, fmt.Errorf("mbvmdb: no cognitive memory playbook found for domain %q", domain)
	}

	// 1. Direct Fast-Path match
	if target, exists := pb.FastPaths[normIntent]; exists {
		conf := 1.0
		if pb.FailureCount > 0 && pb.SuccessCount > 0 {
			conf = float64(pb.SuccessCount) / float64(pb.SuccessCount+pb.FailureCount)
		}
		return &ResolvedAction{
			Domain:     domain,
			Intent:     intent,
			Kind:       KindFastPath,
			Target:     target,
			Confidence: conf,
			Notes:      "direct fast-path URL match",
		}, nil
	}

	// 2. Direct Known Selector match
	if sel, exists := pb.KnownSelectors[normIntent]; exists {
		conf := 0.95
		if pb.FailureCount > 0 && pb.SuccessCount > 0 {
			conf = float64(pb.SuccessCount) / float64(pb.SuccessCount+pb.FailureCount)
		}
		return &ResolvedAction{
			Domain:     domain,
			Intent:     intent,
			Kind:       KindSelector,
			Target:     sel,
			Confidence: conf,
			Notes:      "direct known selector match",
		}, nil
	}

	// 3. Dismiss Modals intent
	if strings.Contains(normIntent, "cookie") || strings.Contains(normIntent, "modal") || strings.Contains(normIntent, "consent") {
		if len(pb.DismissModals) > 0 {
			return &ResolvedAction{
				Domain:     domain,
				Intent:     intent,
				Kind:       KindDismissModal,
				Target:     strings.Join(pb.DismissModals, ", "),
				Confidence: 0.90,
				Notes:      "cookie/modal dismiss selectors",
			}, nil
		}
	}

	// 4. Substring / Graph Match in Known Selectors
	for key, sel := range pb.KnownSelectors {
		if strings.Contains(key, normIntent) || strings.Contains(normIntent, key) {
			return &ResolvedAction{
				Domain:     domain,
				Intent:     intent,
				Kind:       KindSelector,
				Target:     sel,
				Confidence: 0.85,
				Notes:      fmt.Sprintf("fuzzy match on selector key %q", key),
			}, nil
		}
	}

	// 5. Substring match in Fast-Paths
	for key, target := range pb.FastPaths {
		if strings.Contains(key, normIntent) || strings.Contains(normIntent, key) {
			return &ResolvedAction{
				Domain:     domain,
				Intent:     intent,
				Kind:       KindFastPath,
				Target:     target,
				Confidence: 0.85,
				Notes:      fmt.Sprintf("fuzzy match on fastpath key %q", key),
			}, nil
		}
	}

	return nil, fmt.Errorf("mbvmdb: no action matching intent %q for domain %q", intent, domain)
}

// RecordFeedback updates the domain's cognitive model based on execution success or failure (self-healing).
func (cm *CognitiveMemory) RecordFeedback(fb ActionFeedback) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	domain := strings.ToLower(strings.TrimSpace(fb.Domain))
	if strings.Contains(domain, "://") {
		if u, err := url.Parse(domain); err == nil && u.Host != "" {
			domain = u.Host
		}
	}

	pb, ok := cm.playbooks[domain]
	if !ok {
		pb = &DomainPlaybook{
			Domain:         domain,
			FastPaths:      make(map[string]string),
			KnownSelectors: make(map[string]string),
		}
		cm.playbooks[domain] = pb
	}

	if fb.Success {
		pb.SuccessCount++
		if fb.Action != "" && fb.Selector != "" {
			pb.KnownSelectors[fb.Action] = fb.Selector
		}
		pb.LastDiagnosis = fmt.Sprintf("Action %s succeeded in %dms", fb.Action, fb.LatencyMs)
	} else {
		pb.FailureCount++
		pb.LastDiagnosis = fmt.Sprintf("Action %s failed with selector %s", fb.Action, fb.Selector)
	}
	pb.UpdatedAt = time.Now()

	return cm.recompileDomainLocked(domain)
}

// recompileDomainLocked compiles the domain memory into MBVM graph symbols and bytecode.
func (cm *CognitiveMemory) recompileDomainLocked(domain string) error {
	pb := cm.playbooks[domain]
	if pb == nil {
		return nil
	}

	pkgName := "mem_" + sanitizeIdent(domain)

	// Playbook summary struct
	successRate := 100.0
	total := pb.SuccessCount + pb.FailureCount
	if total > 0 {
		successRate = (float64(pb.SuccessCount) / float64(total)) * 100.0
	}

	fields := []string{
		fmt.Sprintf("RequiresWARP: %t", pb.RequiresWARP),
		fmt.Sprintf("InsecureTLS: %t", pb.InsecureTLS),
		fmt.Sprintf("SuccessRate: %.1f%% (%d/%d)", successRate, pb.SuccessCount, total),
	}
	if pb.LastStatusCode > 0 {
		fields = append(fields, fmt.Sprintf("LastStatus: %d", pb.LastStatusCode))
	}
	if pb.LastDiagnosis != "" {
		fields = append(fields, fmt.Sprintf("Diagnosis: %q", pb.LastDiagnosis))
	}

	// 1. Playbook Struct Symbol
	pbSym := &SymbolRecord{
		ID:      uint16(len(cm.db.symbols)),
		Kind:    "struct",
		Package: pkgName,
		Name:    "Playbook",
		Fields:  fields,
	}
	cm.db.symbols = append(cm.db.symbols, pbSym)
	cm.db.indexByName[fmt.Sprintf("%s.Playbook", pkgName)] = pbSym

	// 2. Fast-Paths as Action Methods
	for intent, target := range pb.FastPaths {
		actionSym := &SymbolRecord{
			ID:        uint16(len(cm.db.symbols)),
			Kind:      "func",
			Package:   pkgName,
			Receiver:  "*Playbook",
			Name:      "FastPath_" + sanitizeIdent(intent),
			Signature: fmt.Sprintf("(fastpath: %q)", target),
			IncomingCalls: []string{pbSym.Name},
		}
		cm.db.symbols = append(cm.db.symbols, actionSym)
		cm.db.indexByName[fmt.Sprintf("%s.%s", pkgName, actionSym.Name)] = actionSym
		pbSym.OutgoingCalls = append(pbSym.OutgoingCalls, actionSym.Name)
	}

	// 3. Known Selectors as Functions
	for actionName, sel := range pb.KnownSelectors {
		selSym := &SymbolRecord{
			ID:        uint16(len(cm.db.symbols)),
			Kind:      "func",
			Package:   pkgName,
			Receiver:  "*Playbook",
			Name:      "Selector_" + sanitizeIdent(actionName),
			Signature: fmt.Sprintf("(selector: %q)", sel),
			IncomingCalls: []string{pbSym.Name},
		}
		cm.db.symbols = append(cm.db.symbols, selSym)
		cm.db.indexByName[fmt.Sprintf("%s.%s", pkgName, selSym.Name)] = selSym
		pbSym.OutgoingCalls = append(pbSym.OutgoingCalls, selSym.Name)
	}

	cm.db.TotalFiles = len(cm.playbooks)
	return cm.db.compileBytecode()
}

// SliceDomainPrompt returns a dense cognitive context slice formatted for an LLM (typically 30-50 tokens).
func (cm *CognitiveMemory) SliceDomainPrompt(domain string) string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	domain = strings.ToLower(strings.TrimSpace(domain))
	if strings.Contains(domain, "://") {
		if u, err := url.Parse(domain); err == nil && u.Host != "" {
			domain = u.Host
		}
	}

	pb, ok := cm.playbooks[domain]
	if !ok {
		return fmt.Sprintf("// No prior cognitive memory for %s", domain)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("// MBVM Cognitive Memory: %s\n", domain))
	if pb.RequiresWARP {
		sb.WriteString("RequiresWARP: true\n")
	}
	if pb.InsecureTLS {
		sb.WriteString("InsecureTLS: true\n")
	}
	for intent, route := range pb.FastPaths {
		sb.WriteString(fmt.Sprintf("FastPath[%s]: %s\n", intent, route))
	}
	for action, sel := range pb.KnownSelectors {
		sb.WriteString(fmt.Sprintf("Selector[%s]: %s\n", action, sel))
	}
	if len(pb.DismissModals) > 0 {
		sb.WriteString(fmt.Sprintf("DismissModals: %s\n", strings.Join(pb.DismissModals, ", ")))
	}
	return strings.TrimSpace(sb.String())
}

// Bytecode returns the compiled bytecode of the cognitive memory database.
func (cm *CognitiveMemory) Bytecode() []byte {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.db.bytecode
}

// SaveToFile writes the cognitive memory to disk in .mbvmdb binary format.
func (cm *CognitiveMemory) SaveToFile(filename string) error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.db.SaveToFile(filename)
}
