package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredGitHubClient(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("test-only-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	variables := []string{"GITHUB_TOKEN", "GITHUB_TOKEN_FILE", "GITHUB_APP_ID", "GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY_FILE"}
	app := map[string]string{"GITHUB_APP_ID": "123", "GITHUB_APP_INSTALLATION_ID": "456", "GITHUB_APP_PRIVATE_KEY_FILE": keyPath}
	for _, test := range []struct {
		name      string
		values    map[string]string
		wantError bool
	}{
		{"missing", nil, true},
		{"token", map[string]string{"GITHUB_TOKEN": "test-only-token"}, false},
		{"token file", map[string]string{"GITHUB_TOKEN_FILE": tokenPath}, false},
		{"token conflict", map[string]string{"GITHUB_TOKEN": "test-only-token", "GITHUB_TOKEN_FILE": tokenPath}, true},
		{"partial app", map[string]string{"GITHUB_APP_ID": "123"}, true},
		{"invalid installation", map[string]string{"GITHUB_APP_ID": "123", "GITHUB_APP_INSTALLATION_ID": "not-an-id", "GITHUB_APP_PRIVATE_KEY_FILE": keyPath}, true},
		{"app key missing", map[string]string{"GITHUB_APP_ID": "123", "GITHUB_APP_INSTALLATION_ID": "456", "GITHUB_APP_PRIVATE_KEY_FILE": filepath.Join(dir, "absent")}, true},
		{"app token conflict", map[string]string{"GITHUB_APP_ID": "123", "GITHUB_TOKEN": "test-only-token"}, true},
		{"app", app, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, variable := range variables {
				t.Setenv(variable, "")
			}
			for variable, value := range test.values {
				t.Setenv(variable, value)
			}
			client, err := configuredGitHubClient()
			if (err != nil) != test.wantError {
				t.Fatalf("expected error %v, got %v", test.wantError, err)
			}
			if !test.wantError && client == nil {
				t.Fatal("expected configured client")
			}
		})
	}
}
