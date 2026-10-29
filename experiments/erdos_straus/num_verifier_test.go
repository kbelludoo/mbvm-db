package erdosstraus

import (
	"testing"
)

func TestCertifiedBenchmarkExamples(t *testing.T) {
	examples := CertifiedBenchmarkExamples()
	if len(examples) < 9 {
		t.Fatalf("expected at least 9 certified numerical benchmark instances, got %d", len(examples))
	}

	for _, ex := range examples {
		if err := CheckNumericalExample(ex); err != nil {
			t.Fatalf("Numerical example %s failed verification: %v", ex.Label, err)
		}
		t.Logf("PASS: %s certified exact: 4/%s = 1/%s + 1/%s + 1/%s",
			ex.Label, ex.N.String(), ex.X.String(), ex.Y.String(), ex.Z.String())
	}
}
