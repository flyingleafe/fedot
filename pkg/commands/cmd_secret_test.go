package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestSecretCommand_IsSensitive(t *testing.T) {
	def := secretCommand()
	if !def.Sensitive {
		t.Fatal("secret command must be marked Sensitive")
	}
}

func TestSensitiveDefinitions_ContainsSecret(t *testing.T) {
	defs := SensitiveDefinitions()
	found := false
	for _, d := range defs {
		if d.Name == "secret" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("SensitiveDefinitions() should include the secret command")
	}
}

func TestSupportedServiceNames(t *testing.T) {
	names := SupportedServiceNames()
	if len(names) == 0 {
		t.Fatal("expected at least one supported service")
	}
	// Check sorted
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Fatalf("names not sorted: %v", names)
		}
	}
	// Check known services exist
	known := []string{"brave", "tavily", "perplexity", "github", "clawhub", "glm_search", "baidu_search"}
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, k := range known {
		if !nameSet[k] {
			t.Errorf("expected service %q in SupportedServiceNames()", k)
		}
	}
}

func TestGetServiceEntry(t *testing.T) {
	entry, ok := GetServiceEntry("brave")
	if !ok {
		t.Fatal("expected brave to be a valid service")
	}
	if entry.Description == "" {
		t.Fatal("expected non-empty description")
	}

	_, ok = GetServiceEntry("nonexistent")
	if ok {
		t.Fatal("expected nonexistent service to return false")
	}
}

func TestSecretSetHandler(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	securityPath := filepath.Join(tmpDir, ".security.yml")

	// Write a minimal config
	os.WriteFile(configPath, []byte(`{"version":1}`), 0o600)

	rt := &Runtime{
		ConfigPath: configPath,
	}

	var reply string
	req := Request{
		Text: "/secret set brave BSA-test-key-1234567890",
		Reply: func(text string) error {
			reply = text
			return nil
		},
	}

	err := secretSetHandler(context.Background(), req, rt)
	if err != nil {
		t.Fatalf("secretSetHandler error: %v", err)
	}

	if reply == "" {
		t.Fatal("expected a reply")
	}
	if !contains(reply, "brave") {
		t.Errorf("reply should mention brave: %s", reply)
	}

	// Verify the security file was written
	data, err := os.ReadFile(securityPath)
	if err != nil {
		t.Fatalf("security file not created: %v", err)
	}
	if !contains(string(data), "brave") {
		t.Errorf("security file should contain brave: %s", string(data))
	}
}

func TestSecretListHandler(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	os.WriteFile(configPath, []byte(`{"version":1}`), 0o600)

	cfg, _ := config.LoadConfig(configPath)
	cfg.Tools.Web.Brave.SetAPIKey("BSA-test-key-1234567890")

	rt := &Runtime{
		Config:     cfg,
		ConfigPath: configPath,
	}

	var reply string
	req := Request{
		Text: "/secret list",
		Reply: func(text string) error {
			reply = text
			return nil
		},
	}

	err := secretListHandler(context.Background(), req, rt)
	if err != nil {
		t.Fatalf("secretListHandler error: %v", err)
	}

	if !contains(reply, "brave") {
		t.Errorf("reply should list brave: %s", reply)
	}
	if !contains(reply, "BSA-") {
		t.Errorf("reply should show masked key: %s", reply)
	}
	if contains(reply, "BSA-test-key-1234567890") {
		t.Error("reply should NOT show full key")
	}
}

func TestSecretRemoveHandler(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	os.WriteFile(configPath, []byte(`{"version":1}`), 0o600)

	// First set a key
	cfg, _ := config.LoadConfig(configPath)
	cfg.Tools.Web.Brave.SetAPIKey("BSA-test-key")
	config.SaveConfig(configPath, cfg)

	rt := &Runtime{ConfigPath: configPath}

	var reply string
	req := Request{
		Text: "/secret remove brave",
		Reply: func(text string) error {
			reply = text
			return nil
		},
	}

	err := secretRemoveHandler(context.Background(), req, rt)
	if err != nil {
		t.Fatalf("secretRemoveHandler error: %v", err)
	}

	if !contains(reply, "removed") {
		t.Errorf("reply should confirm removal: %s", reply)
	}

	// Verify key is gone
	cfg2, _ := config.LoadConfig(configPath)
	if cfg2.Tools.Web.Brave.APIKey() != "" {
		t.Error("brave API key should be empty after removal")
	}
}

func TestSecretSetHandler_DeletesMessage(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	os.WriteFile(configPath, []byte(`{"version":1}`), 0o600)

	rt := &Runtime{ConfigPath: configPath}

	deleted := false
	req := Request{
		Text: "/secret set brave BSA-test-key-1234567890",
		Reply: func(string) error {
			return nil
		},
		DeleteMessage: func() error {
			deleted = true
			return nil
		},
	}

	err := secretSetHandler(context.Background(), req, rt)
	if err != nil {
		t.Fatalf("secretSetHandler error: %v", err)
	}

	if !deleted {
		t.Error("expected DeleteMessage to be called for /secret set")
	}
}

func TestSecretSetHandler_UnknownService(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	os.WriteFile(configPath, []byte(`{"version":1}`), 0o600)

	rt := &Runtime{ConfigPath: configPath}

	var reply string
	req := Request{
		Text: "/secret set nonexistent some-key",
		Reply: func(text string) error {
			reply = text
			return nil
		},
	}

	err := secretSetHandler(context.Background(), req, rt)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if !contains(reply, "Unknown service") {
		t.Errorf("expected unknown service error: %s", reply)
	}
}

func TestSecretSetHandler_MissingArgs(t *testing.T) {
	rt := &Runtime{}

	var reply string
	req := Request{
		Text: "/secret set",
		Reply: func(text string) error {
			reply = text
			return nil
		},
	}

	err := secretSetHandler(context.Background(), req, rt)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if !contains(reply, "Usage") {
		t.Errorf("expected usage message: %s", reply)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
