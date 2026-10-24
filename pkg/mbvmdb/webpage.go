package mbvmdb

import (
	"fmt"
	"net/url"
	"strings"
)

// WebFormField represents an input or field inside a web form.
type WebFormField struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // text, password, email, checkbox, submit, etc.
	Value       string `json:"value,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Ref         string `json:"ref,omitempty"` // #ref:1
}

// WebForm represents an interactive HTML form.
type WebForm struct {
	ID     string         `json:"id"`
	Action string         `json:"action"`
	Method string         `json:"method"` // GET, POST
	Fields []WebFormField `json:"fields"`
}

// WebInteractiveElement represents a button or link on the page.
type WebInteractiveElement struct {
	Ref   string `json:"ref"`   // #ref:1
	Tag   string `json:"tag"`   // a, button
	Type  string `json:"type"`  // link, submit, button
	Text  string `json:"text"`  // visible text or label
	Href  string `json:"href,omitempty"`
	Target string `json:"target,omitempty"`
}

// WebPageData represents the structural elements of a web page for MBVM-DB indexing.
type WebPageData struct {
	URL        string                  `json:"url"`
	Title      string                  `json:"title"`
	StatusCode int                     `json:"status_code"`
	Domain     string                  `json:"domain"`
	Forms      []WebForm               `json:"forms"`
	Elements   []WebInteractiveElement `json:"elements"`
	Sections   map[string]string       `json:"sections,omitempty"` // Heading -> Excerpt
}

// IngestWebPage parses a web page data structure and indexes it into the MBVM-DB graph.
func (db *DB) IngestWebPage(page WebPageData) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	domain := page.Domain
	if domain == "" {
		if u, err := url.Parse(page.URL); err == nil && u.Host != "" {
			domain = u.Host
		} else {
			domain = "page"
		}
	}
	// Sanitize package name
	pkgName := strings.ReplaceAll(domain, ".", "_")
	pkgName = strings.ReplaceAll(pkgName, ":", "_")
	pkgName = strings.ReplaceAll(pkgName, "-", "_")

	// 1. Page Metadata Struct
	titleClean := page.Title
	if len(titleClean) > 60 {
		titleClean = titleClean[:60] + "..."
	}
	pageSym := &SymbolRecord{
		ID:      uint16(len(db.symbols)),
		Kind:    "struct",
		Package: pkgName,
		Name:    "PageMetadata",
		Fields: []string{
			fmt.Sprintf("URL: %q", page.URL),
			fmt.Sprintf("Title: %q", titleClean),
			fmt.Sprintf("Status: %d", page.StatusCode),
		},
	}
	db.symbols = append(db.symbols, pageSym)
	db.indexByName[pageSym.Name] = pageSym
	db.indexByName[fmt.Sprintf("%s.%s", pkgName, pageSym.Name)] = pageSym

	// 2. Forms as Structs with Fields and Submit Method
	for idx, form := range page.Forms {
		formName := form.ID
		if formName == "" {
			formName = fmt.Sprintf("Form_%d", idx+1)
		} else {
			formName = "Form_" + sanitizeIdent(formName)
		}

		var fieldStrs []string
		for _, f := range form.Fields {
			fLabel := f.Name
			if fLabel == "" {
				fLabel = f.Type
			}
			refMark := ""
			if f.Ref != "" {
				refMark = fmt.Sprintf(" [%s]", f.Ref)
			}
			fieldStrs = append(fieldStrs, fmt.Sprintf("%s: %s%s", sanitizeIdent(fLabel), f.Type, refMark))
		}

		methodName := fmt.Sprintf("Submit_%s", formName)
		actionMethod := &SymbolRecord{
			ID:        uint16(len(db.symbols) + 1),
			Kind:      "func",
			Package:   pkgName,
			Receiver:  "*" + formName,
			Name:      methodName,
			Signature: fmt.Sprintf("(method: %s, action: %q)", form.Method, form.Action),
		}

		formSym := &SymbolRecord{
			ID:            uint16(len(db.symbols)),
			Kind:          "struct",
			Package:       pkgName,
			Name:          formName,
			Fields:        fieldStrs,
			OutgoingCalls: []string{methodName},
		}
		db.symbols = append(db.symbols, formSym)
		db.indexByName[formSym.Name] = formSym
		db.indexByName[fmt.Sprintf("%s.%s", pkgName, formSym.Name)] = formSym

		actionMethod.IncomingCalls = []string{formSym.Name}
		db.symbols = append(db.symbols, actionMethod)
		db.indexByName[actionMethod.Name] = actionMethod
		recvKey := fmt.Sprintf("%s.%s", formName, methodName)
		db.indexByName[recvKey] = actionMethod
		db.methodsByReceiver["*"+formName] = append(db.methodsByReceiver["*"+formName], actionMethod)
	}

	// 3. Interactive Elements (Buttons and Links)
	for idx, elem := range page.Elements {
		elemName := sanitizeIdent(elem.Text)
		if elemName == "" || len(elemName) < 2 {
			elemName = fmt.Sprintf("Action_%d", idx+1)
		} else if len(elemName) > 30 {
			elemName = elemName[:30]
		}
		elemName = fmt.Sprintf("Elem_%s_%d", elemName, idx+1)

		sig := fmt.Sprintf("(%s %s)", elem.Tag, elem.Ref)
		if elem.Href != "" {
			sig = fmt.Sprintf("(%s %s -> %s)", elem.Tag, elem.Ref, elem.Href)
		}

		elemSym := &SymbolRecord{
			ID:        uint16(len(db.symbols)),
			Kind:      "func",
			Package:   pkgName,
			Name:      elemName,
			Signature: sig,
		}
		db.symbols = append(db.symbols, elemSym)
		db.indexByName[elemSym.Name] = elemSym
		db.indexByName[fmt.Sprintf("%s.%s", pkgName, elemSym.Name)] = elemSym
	}

	// 4. Content Sections (Headings and Excerpts)
	for heading, excerpt := range page.Sections {
		cleanHead := sanitizeIdent(heading)
		if cleanHead == "" {
			continue
		}
		if len(excerpt) > 120 {
			excerpt = excerpt[:120] + "..."
		}
		secSym := &SymbolRecord{
			ID:      uint16(len(db.symbols)),
			Kind:    "struct",
			Package: pkgName,
			Name:    "Section_" + cleanHead,
			Fields:  []string{fmt.Sprintf("Summary: %q", excerpt)},
		}
		db.symbols = append(db.symbols, secSym)
		db.indexByName[secSym.Name] = secSym
	}

	db.TotalFiles = 1
	db.TotalRawBytes = int64(len(page.URL) + len(page.Title) + 1024)

	return db.compileBytecode()
}

func sanitizeIdent(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		} else if r == ' ' || r == '-' {
			sb.WriteRune('_')
		}
	}
	res := sb.String()
	for strings.Contains(res, "__") {
		res = strings.ReplaceAll(res, "__", "_")
	}
	return strings.Trim(res, "_")
}

// CompileWebPageToBytecode converts a WebPageData directly into compressed MBVM bytecode.
func CompileWebPageToBytecode(page WebPageData) (*DB, []byte, error) {
	db := New()
	if err := db.IngestWebPage(page); err != nil {
		return nil, nil, err
	}
	return db, db.bytecode, nil
}
