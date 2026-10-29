package realeval

import (
	"math/big"
	"testing"
)

func TestCRTDevelopmentPairedRetrievalAndCertificates(t *testing.T) {
	lab := NewCRTDevelopmentLab()
	if len(lab.Tasks) != 12 {
		t.Fatalf("got %d tasks, want 12", len(lab.Tasks))
	}
	rows, err := lab.RunCRTDevelopmentPaired()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 12 {
		t.Fatalf("got %d rows, want 12", len(rows))
	}
	for _, row := range rows {
		if !row.Conventional.Certified || !row.CSR.Certified || !row.SameContext || !row.SameCertificate {
			t.Fatalf("retrieval arms disagree on %s: %+v", row.TaskID, row)
		}
		if len(row.CSR.RetrievedIDs) != 2 || row.CSR.ContextBytes == 0 {
			t.Fatalf("missing required premises in %s", row.TaskID)
		}
		cert := row.CSR.Certificate
		if cert == nil {
			t.Fatalf("missing certificate in %s", row.TaskID)
		}
		R, _ := new(big.Int).SetString(cert.Residue, 10)
		M, _ := new(big.Int).SetString(cert.Modulus, 10)
		if R == nil || M == nil || R.Sign() < 0 || R.Cmp(M) >= 0 {
			t.Fatalf("invalid class in %s", row.TaskID)
		}
	}
}

func TestCRTDevelopmentRejectsForgedCertificate(t *testing.T) {
	lab := NewCRTDevelopmentLab()
	task := lab.Tasks[0]
	rules, err := lab.retrieveCSR(task.GoalID)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := composeCRTDevelopment(rules)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyCRTDevelopment(lab.byID[task.GoalID], cert, lab.byID); err != nil {
		t.Fatal(err)
	}
	copyCert := *cert
	copyCert.Residue = "0"
	if err := verifyCRTDevelopment(lab.byID[task.GoalID], &copyCert, lab.byID); err == nil {
		t.Fatal("forged residue was accepted")
	}
	copyCert = *cert
	copyCert.Modulus = "1"
	if err := verifyCRTDevelopment(lab.byID[task.GoalID], &copyCert, lab.byID); err == nil {
		t.Fatal("forged modulus was accepted")
	}
	copyCert = *cert
	copyCert.ParentIDs = []string{"DEV-CRT-02-L1", "DEV-CRT-02-L2"}
	if err := verifyCRTDevelopment(lab.byID[task.GoalID], &copyCert, lab.byID); err == nil {
		t.Fatal("unrelated parent IDs were accepted")
	}
}
