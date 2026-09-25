package httpapi

import "testing"

func TestRegistrationEnabled(t *testing.T) {
	t.Setenv("WORK_GRAPH_REGISTRATION_MODE", "open")
	if !registrationEnabled() {
		t.Fatal("open registration mode should enable registration")
	}

	t.Setenv("WORK_GRAPH_REGISTRATION_MODE", "invite_only")
	if registrationEnabled() {
		t.Fatal("invite-only registration mode should disable registration")
	}
}
