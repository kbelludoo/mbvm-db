package erdosstraus

import (
	"fmt"
	"math/big"
)

// NumericExample represents a candidate numerical solution (n, x, y, z) to be certified.
type NumericExample struct {
	Label string
	N     *big.Int
	X     *big.Int
	Y     *big.Int
	Z     *big.Int
}

// CheckNumericalExample validates that positive integers x, y, z satisfy 4/n = 1/x + 1/y + 1/z.
func CheckNumericalExample(ex NumericExample) error {
	if ex.N == nil || ex.X == nil || ex.Y == nil || ex.Z == nil {
		return fmt.Errorf("[%s] nil integer values in example: %w", ex.Label, ErrInvalidInput)
	}
	if ex.N.Cmp(big.NewInt(2)) < 0 || ex.X.Sign() <= 0 || ex.Y.Sign() <= 0 || ex.Z.Sign() <= 0 {
		return fmt.Errorf("[%s] all values must be positive integers: %w", ex.Label, ErrNonPositiveDenominator)
	}

	// 4 * x * y * z
	lhs := new(big.Int).Mul(four, ex.X)
	lhs.Mul(lhs, ex.Y)
	lhs.Mul(lhs, ex.Z)

	// n * (xy + yz + xz)
	xy := new(big.Int).Mul(ex.X, ex.Y)
	yz := new(big.Int).Mul(ex.Y, ex.Z)
	xz := new(big.Int).Mul(ex.X, ex.Z)
	sum := new(big.Int).Add(xy, yz)
	sum.Add(sum, xz)
	rhs := new(big.Int).Mul(ex.N, sum)

	if lhs.Cmp(rhs) != 0 {
		diff := new(big.Int).Sub(lhs, rhs)
		return fmt.Errorf("[%s] verification failed: 4xyz != n(xy+yz+xz), diff=%s: %w",
			ex.Label, diff.String(), ErrIdentityMismatch)
	}

	return nil
}

// CertifiedBenchmarkExamples returns the complete certified catalog of numerical instances from MBVM-MATH-30.
func CertifiedBenchmarkExamples() []NumericExample {
	return []NumericExample{
		{
			Label: "MATH-06 (n=97)",
			N:     big.NewInt(97),
			X:     big.NewInt(25),
			Y:     big.NewInt(810),
			Z:     big.NewInt(392850),
		},
		{
			Label: "MATH-07 (n=8401)",
			N:     big.NewInt(8401),
			X:     big.NewInt(2101),
			Y:     big.NewInt(5883504),
			Z:     big.NewInt(9440617566864),
		},
		{
			Label: "MATH-08 (n=1009)",
			N:     big.NewInt(1009),
			X:     big.NewInt(253),
			Y:     big.NewInt(85096),
			Z:     big.NewInt(1974822872),
		},
		{
			Label: "MATH-09 (n=697, C=35, Corrected)",
			N:     big.NewInt(697),
			X:     big.NewInt(175),
			Y:     big.NewInt(40670),
			Z:     big.NewInt(141734950),
		},
		{
			Label: "MATH-17 (n=3649)",
			N:     big.NewInt(3649),
			X:     big.NewInt(913),
			Y:     big.NewInt(1110516),
			Z:     big.NewInt(336338649372),
		},
		{
			Label: "MATH-18 (n=13801, p=17, Corrected)",
			N:     big.NewInt(13801),
			X:     big.NewInt(3451),
			Y:     big.NewInt(15875756),
			Z:     big.NewInt(44477565636868),
		},
		{
			Label: "MATH-23 (q=119, n=56641, Corrected)",
			N:     big.NewInt(56641),
			X:     big.NewInt(14161),
			Y:     big.NewInt(267364440),
			Z:     big.NewInt(1802110920278760),
		},
		{
			Label: "MATH-28 (n=73)",
			N:     big.NewInt(73),
			X:     big.NewInt(20),
			Y:     big.NewInt(219),
			Z:     big.NewInt(4380),
		},
		{
			Label: "MATH-29 (n=49)",
			N:     big.NewInt(49),
			X:     big.NewInt(14),
			Y:     big.NewInt(147),
			Z:     big.NewInt(294),
		},
	}
}
