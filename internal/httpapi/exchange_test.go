package httpapi

import "testing"

func TestAssessBidRiskRequiresEnoughEvidence(t *testing.T) {
	if got := assessBidRisk(60, estimationStats{ObservationCount: minimumRiskAssessmentObservations - 1}); got != nil {
		t.Fatalf("assessBidRisk() = %#v, want nil", got)
	}
}

func TestAssessBidRiskExplainsExpectedTimeAndUncertainty(t *testing.T) {
	got := assessBidRisk(100, estimationStats{ObservationCount: 9, MeanRelativeError: 0.2, Stability: 0.3})
	if got == nil {
		t.Fatal("assessBidRisk() = nil, want assessment")
	}
	if got.ExpectedDurationMinutes != 120 {
		t.Fatalf("expected duration = %d, want 120", got.ExpectedDurationMinutes)
	}
	if got.UncertaintyBufferMinutes != 17 {
		t.Fatalf("uncertainty buffer = %d, want 17", got.UncertaintyBufferMinutes)
	}
	if got.AdjustedDurationMinutes != 137 {
		t.Fatalf("adjusted duration = %d, want 137", got.AdjustedDurationMinutes)
	}
}

func TestAssessBidRiskBoundsUnrealisticFastHistory(t *testing.T) {
	got := assessBidRisk(100, estimationStats{ObservationCount: 5, MeanRelativeError: -2})
	if got == nil || got.ExpectedDurationMinutes != 25 {
		t.Fatalf("assessBidRisk() = %#v, want expected duration floor of 25", got)
	}
}
