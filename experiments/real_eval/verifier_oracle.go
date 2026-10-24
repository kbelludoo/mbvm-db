package realeval

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"

	erdosstraus "github.com/kbelludoo/mbvm-db/experiments/erdos_straus"
)

// VerifyResponse rigorously evaluates an LLM response without keyword heuristics.
func VerifyResponse(p EvalProblem, respText string) (bool, string) {
	clean := strings.TrimSpace(respText)
	if len(clean) < 5 {
		return false, "response too short to contain mathematical proof or solution"
	}

	switch p.Type {
	case ProblemConstruction:
		return verifyConstruction(p, clean)
	case ProblemProof:
		return verifyAlgebraicProof(p, clean)
	case ProblemRefutation:
		return verifyRefutation(p, clean)
	case ProblemObstruction:
		return verifyObstruction(p, clean)
	default:
		return false, "unknown problem type"
	}
}

// Regex for extracting triples across single-line, multi-line, LaTeX, Markdown, etc.
var reTriple = regexp.MustCompile(`(?i)(?:^|[\s,;$\*\\])x\s*[:=]\s*(\d+)[\s,;$\*\n\r]+(?:y|\$y|\\textbf\{y\})[\s\*]*[:=]\s*(\d+)[\s,;$\*\n\r]+(?:z|\$z|\\textbf\{z\})[\s\*]*[:=]\s*(\d+)`)

// Regex for extracting parenthesized triples like (x, y, z)
var reParenTriple = regexp.MustCompile(`\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)`)

func verifyConstruction(p EvalProblem, text string) (bool, string) {
	// 1. Scan for explicit triples (x=..., y=..., z=...) from end to beginning
	allTriples := reTriple.FindAllStringSubmatch(text, -1)
	for i := len(allTriples) - 1; i >= 0; i-- {
		m := allTriples[i]
		if len(m) >= 4 {
			xBig, okX := new(big.Int).SetString(m[1], 10)
			yBig, okY := new(big.Int).SetString(m[2], 10)
			zBig, okZ := new(big.Int).SetString(m[3], 10)
			if okX && okY && okZ {
				sol := &erdosstraus.Solution{X: xBig, Y: yBig, Z: zBig}
				if err := erdosstraus.VerifySolution(p.N, sol); err == nil {
					return true, fmt.Sprintf("exact identity verified: 4/%s = 1/%s + 1/%s + 1/%s", p.N.String(), m[1], m[2], m[3])
				}
			}
		}
	}

	// 2. Scan for parenthesized triples (x, y, z) from end to beginning
	parenTriples := reParenTriple.FindAllStringSubmatch(text, -1)
	for i := len(parenTriples) - 1; i >= 0; i-- {
		m := parenTriples[i]
		if len(m) >= 4 {
			xBig, okX := new(big.Int).SetString(m[1], 10)
			yBig, okY := new(big.Int).SetString(m[2], 10)
			zBig, okZ := new(big.Int).SetString(m[3], 10)
			if okX && okY && okZ {
				sol := &erdosstraus.Solution{X: xBig, Y: yBig, Z: zBig}
				if err := erdosstraus.VerifySolution(p.N, sol); err == nil {
					return true, fmt.Sprintf("exact identity verified: 4/%s = 1/%s + 1/%s + 1/%s", p.N.String(), m[1], m[2], m[3])
				}
			}
		}
	}

	// 3. Fallback: extract last individual x, y, z occurrences
	x, y, z := extractLastXYZ(text)
	if x != "" && y != "" && z != "" {
		xBig, okX := new(big.Int).SetString(x, 10)
		yBig, okY := new(big.Int).SetString(y, 10)
		zBig, okZ := new(big.Int).SetString(z, 10)
		if okX && okY && okZ {
			sol := &erdosstraus.Solution{X: xBig, Y: yBig, Z: zBig}
			if err := erdosstraus.VerifySolution(p.N, sol); err == nil {
				return true, fmt.Sprintf("exact identity verified: 4/%s = 1/%s + 1/%s + 1/%s", p.N.String(), x, y, z)
			}
		}
	}

	return false, fmt.Sprintf("no valid integer triplet (x, y, z) satisfying 4/%s was found in response", p.N.String())
}

// Regex matching individual x, y, z
var (
	reX = regexp.MustCompile(`(?i)(?:^|[\s,;$\*\\])x\s*[:=]\s*(\d+)`)
	reY = regexp.MustCompile(`(?i)(?:^|[\s,;$\*\\])y\s*[:=]\s*(\d+)`)
	reZ = regexp.MustCompile(`(?i)(?:^|[\s,;$\*\\])z\s*[:=]\s*(\d+)`)
)

func extractXYZ(text string) (string, string, string) {
	mx := reX.FindStringSubmatch(text)
	my := reY.FindStringSubmatch(text)
	mz := reZ.FindStringSubmatch(text)

	var x, y, z string
	if len(mx) > 1 {
		x = mx[1]
	}
	if len(my) > 1 {
		y = my[1]
	}
	if len(mz) > 1 {
		z = mz[1]
	}
	return x, y, z
}

func extractLastXYZ(text string) (string, string, string) {
	allX := reX.FindAllStringSubmatch(text, -1)
	allY := reY.FindAllStringSubmatch(text, -1)
	allZ := reZ.FindAllStringSubmatch(text, -1)

	var x, y, z string
	if len(allX) > 0 && len(allX[len(allX)-1]) > 1 {
		x = allX[len(allX)-1][1]
	}
	if len(allY) > 0 && len(allY[len(allY)-1]) > 1 {
		y = allY[len(allY)-1][1]
	}
	if len(allZ) > 0 && len(allZ[len(allZ)-1]) > 1 {
		z = allZ[len(allZ)-1][1]
	}
	return x, y, z
}
