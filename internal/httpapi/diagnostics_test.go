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

func TestFormatDiagnosticDuration(t *testing.T) {
	tests := map[int]string{45: "45 min", 125: "2h 05m", 3060: "2d 3h"}
	for minutes, want := range tests {
		if got := formatDiagnosticDuration(minutes); got != want {
			t.Fatalf("formatDiagnosticDuration(%d) = %q, want %q", minutes, got, want)
		}
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
