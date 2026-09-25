package httpapi

import "testing"

func TestPasswordEmailConfigured(t *testing.T) {
	t.Setenv("WORK_GRAPH_SMTP_ADDRESS", "smtp.example.test:587")
	t.Setenv("WORK_GRAPH_EMAIL_FROM", "Work Graph <work@example.test>")
	t.Setenv("WORK_GRAPH_PUBLIC_URL", "https://work.example.test")
	if !passwordEmailConfigured() {
		t.Fatal("complete email settings should enable password email")
	}
	t.Setenv("WORK_GRAPH_SMTP_ADDRESS", "")
	if passwordEmailConfigured() {
		t.Fatal("missing SMTP address should disable password email")
	}
}
