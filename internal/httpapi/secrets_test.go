package httpapi

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestSecretEncryptionRoundTripAndWorkspaceBinding(t *testing.T) {
	t.Setenv("WORK_GRAPH_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	ciphertext, nonce, err := encryptSecret("workspace-a", "very-sensitive-value")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("very-sensitive-value")) {
		t.Fatal("ciphertext contains plaintext")
	}
	value, err := decryptSecret("workspace-a", ciphertext, nonce)
	if err != nil || value != "very-sensitive-value" {
		t.Fatalf("round trip failed: %q, %v", value, err)
	}
	if _, err := decryptSecret("workspace-b", ciphertext, nonce); err == nil {
		t.Fatal("ciphertext was accepted in a different workspace")
	}
}

func TestSecretEncryptionRequiresValidMasterKey(t *testing.T) {
	t.Setenv("WORK_GRAPH_MASTER_KEY", "")
	if _, _, err := encryptSecret("workspace-a", "value"); err == nil {
		t.Fatal("missing master key was accepted")
	}
}
