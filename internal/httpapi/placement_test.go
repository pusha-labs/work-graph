package httpapi

import (
	"strings"
	"testing"
)

func TestRankPlacementExplainsKnowledgeAndTextMatches(t *testing.T) {
	rootID, childID := "root", "child"
	nodes := []workNode{
		{ID: rootID, RootID: rootID, Title: "Build a house"},
		{ID: childID, RootID: rootID, ParentID: &rootID, Title: "Prepare construction site", DesiredOutcome: "Utilities and ground are ready"},
	}
	signals := map[string]placementSignals{
		childID: {Knowledge: map[string]string{"subject": "Construction site"}, Capabilities: map[string]string{}},
	}
	result := rankPlacement("Prepare ground utilities", nodes, signals, nil, map[string]string{"subject": "Construction site"})
	if len(result) == 0 || result[0].ParentID != childID || result[0].Confidence != "medium" {
		t.Fatalf("unexpected recommendations: %#v", result)
	}
	joined := strings.Join(result[0].Reasons, " ")
	if !strings.Contains(joined, "Construction site") || !strings.Contains(joined, "Shared terms") {
		t.Fatalf("missing explanation: %q", joined)
	}
	if len(result[0].Path) != 2 || result[0].Path[0] != "Build a house" {
		t.Fatalf("unexpected path: %#v", result[0].Path)
	}
}

func TestRankPlacementReturnsNoGuessWithoutEvidence(t *testing.T) {
	result := rankPlacement("Deploy ClickHouse", []workNode{{ID: "root", RootID: "root", Title: "Build a house"}}, nil, nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected no unsupported guess, got %#v", result)
	}
}

func TestPlacementTermsAreNormalized(t *testing.T) {
	got := placementTerms("The Python API, python service")
	if strings.Join(got, ",") != "api,python,service" {
		t.Fatalf("placementTerms() = %#v", got)
	}
}
