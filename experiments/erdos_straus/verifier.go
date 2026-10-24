package erdosstraus

import (
	"fmt"
	"math/big"
)

var (
	zero = big.NewInt(0)
	four = big.NewInt(4)
)

// VerifySolution validates whether positive integers x, y, z strictly satisfy:
// 4/n = 1/x + 1/y + 1/z  <=>  4*x*y*z = n*(y*z + x*z + x*y).
func VerifySolution(n *big.Int, sol *Solution) error {
	if n == nil || sol == nil || sol.X == nil || sol.Y == nil || sol.Z == nil {
		return fmt.Errorf("verify failed: nil argument provided: %w", ErrInvalidInput)
	}
	if n.Cmp(big.NewInt(2)) < 0 {
		return fmt.Errorf("verify failed: n must be >= 2, got %s: %w", n.String(), ErrInvalidInput)
	}
	if sol.X.Cmp(zero) <= 0 || sol.Y.Cmp(zero) <= 0 || sol.Z.Cmp(zero) <= 0 {
		return fmt.Errorf("verify failed: x, y, z must be strictly positive: %w", ErrNonPositiveDenominator)
	}

	// LHS: 4 * x * y * z
	lhs := new(big.Int).Mul(four, sol.X)
	lhs.Mul(lhs, sol.Y)
	lhs.Mul(lhs, sol.Z)

	// RHS: n * (y*z + x*z + x*y)
	yz := new(big.Int).Mul(sol.Y, sol.Z)
	xz := new(big.Int).Mul(sol.X, sol.Z)
	xy := new(big.Int).Mul(sol.X, sol.Y)

	sumTerms := new(big.Int).Add(yz, xz)
	sumTerms.Add(sumTerms, xy)

	rhs := new(big.Int).Mul(n, sumTerms)

	if lhs.Cmp(rhs) != 0 {
		return fmt.Errorf("identity mismatch for n=%s: LHS(4xyz)=%s != RHS(n(xy+yz+xz))=%s: %w",
			n.String(), lhs.String(), rhs.String(), ErrIdentityMismatch)
	}

	return nil
}

// ErrInvalidInput is returned when arguments are out of domain.
var ErrInvalidInput = fmt.Errorf("invalid input domain")

// ErrNonPositiveDenominator is returned when a denominator <= 0.
var ErrNonPositiveDenominator = fmt.Errorf("denominator must be positive integer")

// ErrIdentityMismatch is returned when 4/n != 1/x + 1/y + 1/z.
var ErrIdentityMismatch = fmt.Errorf("Erdos-Straus equation does not hold")
