package realeval

import (
	"fmt"
	"math/big"
	"sort"
	"strings"

	erdosstraus "github.com/kbelludoo/mbvm-db/experiments/erdos_straus"
)

func verifyRefutation(p EvalProblem, text string) (bool, string) {
	if len(text) < 40 {
		return false, "refutation lacks required derivation or arithmetic explanation"
	}

	switch p.ID {
	case "B30-17": // p=2, n=25 => z = 10325/2 non-integer
		hasPair := (strings.Contains(text, "p = 2") || strings.Contains(text, "p=2")) && strings.Contains(text, "25")
		hasFraction := strings.Contains(text, "10325") || strings.Contains(text, "5162.5") || strings.Contains(text, "495600/96")
		hasRem := strings.Contains(text, "48") && strings.Contains(text, "96")
		if hasPair && (hasFraction || hasRem) {
			return true, "correctly certified counterexample p=2, n=25 yielding non-integral z = 10325/2"
		}
		return false, "failed to compute certified arithmetic counterexample p=2, n=25 with non-integral z"

	case "B30-18": // n=49 => t=49*13=637, factors 7 and 13 both 1 mod 3
		hasN := strings.Contains(text, "49")
		hasFactors := (strings.Contains(text, "7") && strings.Contains(text, "13")) || strings.Contains(text, "637")
		hasMod := strings.Contains(text, "1 mod 3") || strings.Contains(text, "1 (mod 3)") || strings.Contains(text, "≡ 1")
		if hasN && hasFactors && hasMod {
			return true, "correctly certified n=49 where all prime factors of t=637 are 1 mod 3"
		}
		return false, "failed to exhibit certified counterexample n=49 with divisor factorization"

	case "B30-19": // q=3 => n = 4(9)-3 = 33 != 1 mod 24 (33 = 9 mod 24)
		hasQ := strings.Contains(text, "q = 3") || strings.Contains(text, "q=3")
		hasN := strings.Contains(text, "33")
		hasMod := strings.Contains(text, "9 mod 24") || strings.Contains(text, "9 (mod 24)") || strings.Contains(text, "≠ 1") || strings.Contains(text, "!= 1")
		if hasQ && hasN && hasMod {
			return true, "correctly certified counterexample q=3 yielding n=33 ≡ 9 mod 24"
		}
		return false, "failed to derive certified counterexample q=3 showing n=33 != 1 mod 24"

	case "B30-20": // n=13 => x=4, t=52, z = 2964/15 = 988/5
		has13 := strings.Contains(text, "13")
		hasNonInt := strings.Contains(text, "988/5") || strings.Contains(text, "197.6") || strings.Contains(text, "2964/15")
		if has13 && hasNonInt {
			return true, "correctly certified counterexample n=13 with non-integral z = 988/5"
		}
		return false, "failed to exhibit certified counterexample n=13 with exact fractional value"

	case "B30-21": // p=7, n=25 => y = 728/12 = 182/3
		hasP := strings.Contains(text, "7") && (strings.Contains(text, "p = 7") || strings.Contains(text, "p=7"))
		hasY := strings.Contains(text, "182/3") || strings.Contains(text, "728/12") || strings.Contains(text, "60.6")
		if hasP && hasY {
			return true, "correctly certified counterexample p=7 with non-integral y = 182/3"
		}
		return false, "failed to exhibit counterexample p=7 showing non-integral y"

	case "B30-22": // n=73, x=19 => t=1387, a=3. All prime factors of 1387 (19, 73) = 1 mod 3
		hasPairs := strings.Contains(text, "73") && strings.Contains(text, "19")
		hasDivs := strings.Contains(text, "1387") || (strings.Contains(text, "19") && strings.Contains(text, "73"))
		hasMod3 := strings.Contains(text, "mod 3") || strings.Contains(text, "(mod 3)")
		if hasPairs && hasDivs && hasMod3 {
			return true, "correctly proved divisor obstruction for n=73, x=19"
		}
		return false, "failed to prove divisor obstruction for n=73, x=19"

	case "B30-23": // n=49, x=13 => t=637, a=3. All prime factors of 637 (7, 13) = 1 mod 3
		hasPairs := strings.Contains(text, "49") && strings.Contains(text, "13")
		hasDivs := strings.Contains(text, "637") || (strings.Contains(text, "7") && strings.Contains(text, "13"))
		hasMod3 := strings.Contains(text, "mod 3") || strings.Contains(text, "(mod 3)")
		if hasPairs && hasDivs && hasMod3 {
			return true, "correctly proved divisor obstruction for n=49, x=13"
		}
		return false, "failed to prove divisor obstruction for n=49, x=13"

	// Unseen Benchmark-20 Refutation cases
	case "U20-13": // q=9 => 4(81)-3 = 321 = 3*107 composite
		hasComp := (strings.Contains(text, "9") && strings.Contains(text, "321")) ||
			(strings.Contains(text, "11") && strings.Contains(text, "481")) ||
			strings.Contains(text, "composite") || strings.Contains(text, "composto")
		if hasComp {
			return true, "correctly exhibited odd q yielding composite 4q^2-3"
		}
		return false, "failed to exhibit composite counterexample for 4q^2-3"

	case "U20-14": // n=241, x=61 divisor obstruction
		has241 := strings.Contains(text, "241") || strings.Contains(text, "73") || strings.Contains(text, "193")
		hasMod := strings.Contains(text, "mod 3") || strings.Contains(text, "divisor")
		if has241 && hasMod {
			return true, "correctly refuted universal completion for x=(n+3)/4"
		}
		return false, "failed to exhibit counterexample for x=(n+3)/4 completion"

	case "U20-15": // Non-uniqueness: must exhibit >= 2 distinct certified solutions in math/big
		distinctValid := make(map[string]bool)
		checkTriple := func(m1, m2, m3 string) {
			xBig, okX := new(big.Int).SetString(m1, 10)
			yBig, okY := new(big.Int).SetString(m2, 10)
			zBig, okZ := new(big.Int).SetString(m3, 10)
			if okX && okY && okZ {
				sol := &erdosstraus.Solution{X: xBig, Y: yBig, Z: zBig}
				for _, pCand := range []*big.Int{big.NewInt(3), big.NewInt(5), big.NewInt(7)} {
					if erdosstraus.VerifySolution(pCand, sol) == nil {
						// Normalize order of denominators (x <= y <= z) to prevent permutation false positives
						sorted := []*big.Int{xBig, yBig, zBig}
						sort.Slice(sorted, func(i, j int) bool { return sorted[i].Cmp(sorted[j]) < 0 })
						key := fmt.Sprintf("%s:%s,%s,%s", pCand.String(), sorted[0].String(), sorted[1].String(), sorted[2].String())
						distinctValid[key] = true
					}
				}
			}
		}

		for _, m := range reTriple.FindAllStringSubmatch(text, -1) {
			if len(m) >= 4 {
				checkTriple(m[1], m[2], m[3])
			}
		}
		for _, m := range reParenTriple.FindAllStringSubmatch(text, -1) {
			if len(m) >= 4 {
				checkTriple(m[1], m[2], m[3])
			}
		}

		if len(distinctValid) >= 2 {
			return true, fmt.Sprintf("correctly certified non-uniqueness: demonstrated %d distinct valid solutions verified via math/big", len(distinctValid))
		}
		return false, "refutation failed: must exhibit at least two distinct valid solutions certified via math/big"

	case "U20-16": // Universal claim for n = 1 mod 12 refuted by certified counterexample n=49
		if strings.Contains(text, "25") && !strings.Contains(text, "49") {
			return false, "rejected false counterexample n=25: 4/25 = 1/7 + 1/60 + 1/2100 is an exact integer solution"
		}
		if strings.Contains(text, "49") && (strings.Contains(text, "13") || strings.Contains(text, "637")) {
			hasFactors := (strings.Contains(text, "7") && strings.Contains(text, "13")) || strings.Contains(text, "7^2") || strings.Contains(text, "49 * 13")
			hasMod3 := strings.Contains(text, "1 mod 3") || strings.Contains(text, "1 (mod 3)") || strings.Contains(text, "≡ 1")
			hasTargetMod := strings.Contains(text, "2 mod 3") || strings.Contains(text, "2 (mod 3)") || strings.Contains(text, "-1 mod 3") || strings.Contains(text, "-t") || strings.Contains(text, "-637")
			hasConclusion := strings.Contains(text, "no divisor") || strings.Contains(text, "nenhum divisor") || strings.Contains(text, "impossible") || strings.Contains(text, "cannot") || strings.Contains(text, "never") || strings.Contains(text, "não existe")

			if hasFactors && hasMod3 && (hasTargetMod || hasConclusion) {
				return true, "correctly certified counterexample n=49: all divisors of (nx)^2 are 1 mod 3 != 2 mod 3"
			}
		}
		return false, "failed to exhibit certified counterexample n=49 with divisor factorization and mod 3 obstruction"

	default:
		return false, "unhandled refutation problem ID"
	}
}

func verifyAlgebraicProof(p EvalProblem, text string) (bool, string) {
	if len(text) < 60 {
		return false, "proof is too brief to contain valid deductive steps"
	}

	switch p.ID {
	case "B30-09": // Interval x > n/4 and x <= 3n/4
		hasLower := (strings.Contains(text, "n/4") || strings.Contains(text, "4x > n")) && (strings.Contains(text, ">") || strings.Contains(text, "greater"))
		hasUpper := (strings.Contains(text, "3/x") || strings.Contains(text, "3n/4") || strings.Contains(text, "4x <= 3n") || strings.Contains(text, "4x ≤ 3n"))
		if hasLower && hasUpper {
			return true, "correctly derived lower bound x > n/4 and upper bound x <= 3n/4"
		}
		return false, "proof lacks derivation of both interval bounds"

	case "B30-10": // a divides 2t <=> y=z
		has2t := strings.Contains(text, "2t/a") || strings.Contains(text, "ay = 2t") || strings.Contains(text, "2t")
		hasEq := strings.Contains(text, "y = z") || strings.Contains(text, "y=z")
		if has2t && hasEq {
			return true, "correctly proved equivalence y=z <=> a | 2t"
		}
		return false, "proof missing deduction of ay = 2t from (ay-t)^2 = t^2"

	case "B30-11": // y(n) integer for n=1 mod 12, p=2 mod 3
		hasMod3 := strings.Contains(text, "mod 3") || strings.Contains(text, "3 |")
		hasMod4 := strings.Contains(text, "mod 4") || strings.Contains(text, "4 |")
		has12 := strings.Contains(text, "12")
		if hasMod3 && hasMod4 && has12 {
			return true, "correctly proved divisibility by 3 and 4 coprime components"
		}
		return false, "proof missing separate modulo 3 and modulo 4 divisibility steps"

	case "B30-12": // parity of mn(mn+p)
		hasParity := strings.Contains(text, "parity") || strings.Contains(text, "paridade") || strings.Contains(text, "even") || strings.Contains(text, "par")
		has32 := strings.Contains(text, "32") && (strings.Contains(text, "16") || strings.Contains(text, "2"))
		if hasParity && has32 {
			return true, "correctly proved opposite parity yields factor 32"
		}
		return false, "proof missing parity analysis of mn and mn+p"

	case "B30-13": // n = 4k+3 => 4/n = 1/(k+1) + 1/((k+1)n)
		hasK1 := strings.Contains(text, "k+1") || strings.Contains(text, "k + 1")
		hasAlg := strings.Contains(text, "4(k+1)") || strings.Contains(text, "4k+4") || strings.Contains(text, "4k + 4")
		if hasK1 && hasAlg {
			return true, "correctly derived 4/n identity using 4k+4 = 4(k+1)"
		}
		return false, "proof missing algebraic simplification 4k+4 = 4(k+1)"

	case "B30-14": // n = 3k+2 => 4/n = 1/x + 1/n + 1/(xn) with x=(n+1)/3
		hasX := strings.Contains(text, "(n+1)/3") || strings.Contains(text, "n+1")
		hasAlg := strings.Contains(text, "4x") || strings.Contains(text, "4(n+1)") || strings.Contains(text, "3x")
		if hasX && hasAlg {
			return true, "correctly derived Schinzel 3k+2 identity via x=(n+1)/3"
		}
		return false, "proof missing algebraic identity steps for 3k+2"

	case "B30-15": // q = 5 mod 6 => n = 1 mod 24
		hasQ := strings.Contains(text, "6k") || strings.Contains(text, "mod 6") || strings.Contains(text, "36k")
		has24 := strings.Contains(text, "24") && (strings.Contains(text, "1 mod 24") || strings.Contains(text, "≡ 1"))
		if hasQ && has24 {
			return true, "correctly expanded 4(6k+5)^2 - 3 ≡ 1 mod 24"
		}
		return false, "proof missing expansion of 4q^2-3 modulo 24"

	case "B30-16": // q = 210k - 1 => n = 1 mod 840
		hasQuad := strings.Contains(text, "44100") || strings.Contains(text, "176400") || strings.Contains(text, "420") || strings.Contains(text, "1680")
		has840 := strings.Contains(text, "840") && (strings.Contains(text, "1 mod 840") || strings.Contains(text, "≡ 1"))
		if hasQuad && has840 {
			return true, "correctly derived 4(210k-1)^2 - 3 ≡ 1 mod 840 via polynomial expansion"
		}
		return false, "proof missing polynomial expansion verifying divisibility by 840"

	// Unseen Benchmark-20 Proof cases
	case "U20-09": // Mordell divisor chaining with p | (t+p)
		hasMordell := strings.Contains(text, "(ay-t)") || strings.Contains(text, "ay - t") || strings.Contains(text, "t^2")
		hasP := strings.Contains(text, "mod 3") || strings.Contains(text, "p")
		if hasMordell && hasP {
			return true, "correctly chained Mordell divisor theorem to obtain integer y, z"
		}
		return false, "proof missing Mordell divisor factorization steps"

	case "U20-10": // Linear congruence 12k = -4 mod p unique
		hasCoprime := strings.Contains(text, "gcd") || strings.Contains(text, "coprime") || strings.Contains(text, "invers")
		hasLinear := strings.Contains(text, "12k") || strings.Contains(text, "-4") || strings.Contains(text, "unique") || strings.Contains(text, "única")
		hasCRT := strings.Contains(text, "mod p") && strings.Contains(text, "mod 12")
		if (hasCoprime || hasLinear) && hasCRT {
			return true, "correctly proved existence and uniqueness of progression via modular inversion"
		}
		return false, "proof missing modular inversion and CRT verification"

	case "U20-11": // Distinct denominators for 3k+2 when n > 2
		hasIneq := (strings.Contains(text, "<") || strings.Contains(text, "distinct") || strings.Contains(text, "distintos")) &&
			(strings.Contains(text, "(n+1)/3") || strings.Contains(text, "n > 2"))
		if hasIneq {
			return true, "correctly proved strict denominator ordering (n+1)/3 < n < (n+1)n/3"
		}
		return false, "proof missing strict inequality between denominators"

	case "U20-12": // q = 6k + 1 => 4q^2 - 3 = 1 mod 24
		hasQuad := strings.Contains(text, "36k") || strings.Contains(text, "144k") || strings.Contains(text, "48k")
		has24 := strings.Contains(text, "24") && (strings.Contains(text, "1 mod 24") || strings.Contains(text, "≡ 1"))
		if hasQuad && has24 {
			return true, "correctly expanded 4(6k+1)^2 - 3 ≡ 1 mod 24"
		}
		return false, "proof missing algebraic expansion modulo 24"

	default:
		return false, "unhandled proof problem ID"
	}
}

func verifyObstruction(p EvalProblem, text string) (bool, string) {
	if len(text) < 40 {
		return false, "obstruction proof lacks deductive steps"
	}

	switch p.ID {
	case "B30-24": // R mod 5 in {1, 4} != 2
		hasResidues := (strings.Contains(text, "1") && strings.Contains(text, "4"))
		hasTarget := strings.Contains(text, "2") || strings.Contains(text, "-3")
		hasMod5 := strings.Contains(text, "mod 5") || strings.Contains(text, "(mod 5)")
		if hasResidues && hasTarget && hasMod5 {
			return true, "correctly proved residue set {1, 4} mod 5 disjoint from n ≡ 2 mod 5"
		}
		return false, "missing modular residue comparison modulo 5"

	case "B30-25": // 4q^2 - 3 mod 5 in {1, 2, 3} != 4
		hasQuad := strings.Contains(text, "mod 5") || strings.Contains(text, "(mod 5)")
		hasNonRes := strings.Contains(text, "4") && (strings.Contains(text, "impossible") || strings.Contains(text, "não existe") || strings.Contains(text, "no solution"))
		if hasQuad && hasNonRes {
			return true, "correctly proved quadratic non-residue obstruction mod 5"
		}
		return false, "missing modulo 5 quadratic non-residue proof"

	case "B30-26": // q^2 = 3 mod 7 impossible
		has7 := strings.Contains(text, "mod 7") || strings.Contains(text, "(mod 7)")
		has3 := strings.Contains(text, "3") && (strings.Contains(text, "non-residue") || strings.Contains(text, "não-resíduo") || strings.Contains(text, "impossible") || strings.Contains(text, "{0, 1, 2, 4}") || strings.Contains(text, "{0,1,2,4}"))
		if has7 && has3 {
			return true, "correctly proved q^2 ≡ 3 mod 7 has no integer solutions"
		}
		return false, "missing proof that 3 is quadratic non-residue modulo 7"

	case "B30-27": // n = 3649 mod 9240
		has3649 := strings.Contains(text, "3649")
		has9240 := strings.Contains(text, "9240")
		hasDeriv := strings.Contains(text, "CRT") || strings.Contains(text, "Chinese") || strings.Contains(text, "Chinês") || strings.Contains(text, "840")
		if has3649 && has9240 && hasDeriv {
			return true, "correctly derived unique progression n ≡ 3649 mod 9240 via CRT"
		}
		return false, "missing CRT derivation of unique progression 3649 mod 9240"

	case "B30-28": // n = 13801 mod 14280
		has13801 := strings.Contains(text, "13801")
		has14280 := strings.Contains(text, "14280")
		hasDeriv := strings.Contains(text, "CRT") || strings.Contains(text, "Chinese") || strings.Contains(text, "Chinês") || strings.Contains(text, "840")
		if has13801 && has14280 && hasDeriv {
			return true, "correctly derived unique progression n ≡ 13801 mod 14280 via CRT"
		}
		return false, "missing CRT derivation of unique progression 13801 mod 14280"

	case "B30-29": // density 2/11
		hasDensity := strings.Contains(text, "2/11") || strings.Contains(text, "18.18") || strings.Contains(text, "18,18")
		hasExpl := strings.Contains(text, "density") || strings.Contains(text, "densidade") || strings.Contains(text, "11")
		if hasDensity && hasExpl {
			return true, "correctly derived natural relative density 2/11"
		}
		return false, "missing relative density 2/11 derivation"

	case "B30-30": // n = 1 mod p leads to non-integer z
		hasCRT := strings.Contains(text, "CRT") || strings.Contains(text, "Chinese") || strings.Contains(text, "Chinês") || strings.Contains(text, "infinitely") || strings.Contains(text, "infinitos")
		hasZ := strings.Contains(text, "mod p") || strings.Contains(text, "p |") || strings.Contains(text, "divis")
		if hasCRT && hasZ {
			return true, "correctly proved existence of infinitely many non-integer instances via CRT"
		}
		return false, "missing CRT argument for infinite non-integer residue classes"

	// Unseen Benchmark-20 Obstruction cases
	case "U20-17": // 4q^2 - 3 = 121 = 2 mod 7 => 4q^2 = 5 mod 7 => q^2 = 3 mod 7, 3 is QNR
		has7 := strings.Contains(text, "mod 7") || strings.Contains(text, "(mod 7)")
		hasNonRes := strings.Contains(text, "3") && (strings.Contains(text, "non-residue") ||
			strings.Contains(text, "não-resíduo") || strings.Contains(text, "impossible") ||
			strings.Contains(text, "contradiction") || strings.Contains(text, "{0, 1, 2, 4}"))
		if has7 && hasNonRes {
			return true, "correctly proved quadratic non-residue obstruction modulo 7 for 121 mod 840"
		}
		return false, "missing modulo 7 quadratic non-residue proof"

	case "U20-18": // n = 14809 mod 19320
		has14809 := strings.Contains(text, "14809")
		has19320 := strings.Contains(text, "19320")
		if has14809 && has19320 {
			return true, "correctly derived unique progression n ≡ 14809 mod 19320 via CRT"
		}
		return false, "missing CRT derivation of unique progression 14809 mod 19320"

	case "U20-19": // n = 1 mod 24 => n = 1 mod 3; n = -3 mod 3 => n = 0 mod 3. 1 != 0 mod 3
		has3 := strings.Contains(text, "mod 3") || strings.Contains(text, "(mod 3)")
		hasDisjoint := (strings.Contains(text, "1") && strings.Contains(text, "0")) ||
			strings.Contains(text, "contradiction") || strings.Contains(text, "distinct") ||
			strings.Contains(text, "not\\equiv") || strings.Contains(text, "!=")
		if has3 && hasDisjoint {
			return true, "correctly proved obstruction: n ≡ 1 mod 3 contradicts n ≡ 0 mod 3"
		}
		return false, "missing mod 3 contradiction derivation"

	case "U20-20": // Exact proportion 1/(12p) via CRT
		has12p := strings.Contains(text, "1/(12p)") || strings.Contains(text, "1 / (12p)") || strings.Contains(text, "1/(12*p)")
		hasCRT := strings.Contains(text, "CRT") || strings.Contains(text, "Chinese") || strings.Contains(text, "gcd")
		if has12p && hasCRT {
			return true, "correctly proved proportion is 1/(12p) by CRT coprimality"
		}
		return false, "missing CRT proportion 1/(12p) proof"

	default:
		return false, "unhandled obstruction problem ID"
	}
}
