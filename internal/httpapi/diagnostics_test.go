package httpapi

import "testing"

func TestJoinDiagnosticNames(t *testing.T) {
	if got := joinDiagnosticNames([]string{"A", "B"}); got != "A, B" {
		t.Fatalf("joinDiagnosticNames() = %q", got)
	}
	if got := joinDiagnosticNames([]string{"A", "B", "C", "D", "E", "F"}); got != "A, B, C, D, and 2 more" {
		t.Fatalf("joinDiagnosticNames() with limit = %q", got)
	}
}

func TestClassifyKnowledgeRisk(t *testing.T) {
	tests := []struct {
		holders  int
		wantKind string
		want     bool
	}{
		{holders: 0, wantKind: "uncovered_knowledge", want: true},
		{holders: 1, wantKind: "concentrated_knowledge", want: true},
		{holders: 2, wantKind: "", want: false},
	}
	for _, test := range tests {
		kind, _, _, _, report := classifyKnowledgeRisk(test.holders)
		if kind != test.wantKind || report != test.want {
			t.Fatalf("classifyKnowledgeRisk(%d) = (%q, %t), want (%q, %t)", test.holders, kind, report, test.wantKind, test.want)
		}
	}
}
