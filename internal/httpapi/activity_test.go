package httpapi

import (
	"encoding/json"
	"testing"
)

func TestDescribeActivity(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		state     string
		want      string
	}{
		{"stage added", "workflow_step.added", `{"workflowStep":{"name":"Design the kitchen","position":2,"stepType":"human"}}`, "Design the kitchen · stage 2 · human"},
		{"step moved", "workflow_step.moved", `{"direction":"earlier"}`, "Moved earlier in the route"},
		{"stage moved with positions", "workflow_step.moved", `{"name":"Design the kitchen","fromPosition":3,"toPosition":2}`, "Design the kitchen · stage 3 → 2"},
		{"step renamed", "workflow_step.updated", `{"previousName":"Draft drawings","name":"Review drawings"}`, "Name: Draft drawings → Review drawings"},
		{"step requirements changed", "workflow_step.updated", `{"previousName":"Implement","name":"Implement","previousCapability":"Python","capability":"ClickHouse","previousKnowledge":"Product A","knowledge":"Product B","previousDistributionMode":"simple","distributionMode":"exchange"}`, "Capability: Python → ClickHouse · Knowledge: Product A → Product B · Distribution: simple → exchange"},
		{"module settings changed", "workflow_step.updated", `{"name":"Call API","previousName":"Call API","configurationChanged":true}`, "Module settings updated"},
		{"stage deleted", "workflow_step.deleted", `{"name":"Obsolete review","position":4}`, "Obsolete review · removed stage 4"},
		{"bid submitted", "workflow_step_bid.submitted", `{"actor":{"displayName":"Demo Designer"},"promisedDurationMinutes":180}`, "Demo Designer · 3 hours"},
		{"bid selected", "workflow_step_bid.selected", `{"promisedDurationMinutes":2880}`, "Shortest estimate selected · 2 days"},
		{"returned", "workflow.returned", `{"workflowStep":{"name":"Review drawings"},"reason":"Missing measurements"}`, "Review drawings · Missing measurements"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := describeActivity(test.eventType, json.RawMessage(test.state), "fallback"); got != test.want {
				t.Fatalf("describeActivity() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDescribeActivityKeepsFallbackForUnknownEvent(t *testing.T) {
	if got := describeActivity("unknown", json.RawMessage(`{"value":"ignored"}`), "original detail"); got != "original detail" {
		t.Fatalf("describeActivity() = %q", got)
	}
}
