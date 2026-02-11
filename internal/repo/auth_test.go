package repo

import (
	"os"
	"testing"
)

func TestAuthFromEnvEmpty(t *testing.T) {
	auth, err := AuthFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if auth != nil {
		t.Fatalf("expected nil auth, got %T", auth)
	}
}

func TestAuthFromEnvHTTPToken(t *testing.T) {
	t.Setenv("THULE_GIT_HTTP_TOKEN", "token")
	t.Setenv("THULE_GIT_HTTP_USER", "oauth2")
	auth, err := AuthFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if auth == nil {
		t.Fatal("expected auth")
	}
}

func TestAuthFromEnvHTTPUserPass(t *testing.T) {
	t.Setenv("THULE_GIT_HTTP_USER", "user")
	t.Setenv("THULE_GIT_HTTP_PASSWORD", "pass")
	auth, err := AuthFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if auth == nil {
		t.Fatal("expected auth")
	}
}

func TestAuthFromEnvInvalidSSHKey(t *testing.T) {
	t.Setenv("THULE_GIT_SSH_KEY", "not-a-key")
	_, err := AuthFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid ssh key")
	}
}

func TestAuthFromEnvSSHKeyFile(t *testing.T) {
	path := t.TempDir() + "/id_rsa"
	if err := os.WriteFile(path, []byte("not-a-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THULE_GIT_SSH_KEY_PATH", path)
	if _, err := AuthFromEnv(); err == nil {
		t.Fatal("expected error for invalid ssh key file")
	}
}
