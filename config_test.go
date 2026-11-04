package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigurationValidationAtCLI(t *testing.T) {
	dir := t.TempDir()
	base := "state: " + filepath.Join(dir, "state.json") + "\nprofiles:\n" + profile("dns", "desec", "https://example.invalid/")
	cases := []string{
		"refresh: 25d\n" + base,
		"refresh: 0s\n" + base,
		"timeout: -1s\n" + base,
		"recipient: '-option'\n" + base,
		"recipient: \"root\\nBcc: synthetic-secret\"\n" + base,
		"sendmail: relative\n" + base,
		strings.Replace(base, "state: "+filepath.Join(dir, "state.json"), "state: relative", 1),
		base + profile("dns", "desec", "https://example.invalid/"),
		strings.Replace(base, "provider: desec", "provider: unsupported", 1),
		strings.Replace(base, "token: synthetic-secret", "token: ''", 1),
		strings.Replace(base, "target: example.invalid", "target: 'invalid,target'", 1),
		strings.Replace(base, `"dns":`, `"":`, 1),
		base + "refresh: 1h\nrefresh: 2h\n",
		base + strings.Repeat(" ", 1<<20),
	}
	for i, content := range cases {
		cfg := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(cfg, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if code := runCLI([]string{"--config", cfg}, &out, dependencies{}); code == 0 {
			t.Fatalf("case %d accepted", i)
		}
		if strings.Contains(out.String(), "synthetic-secret") {
			t.Fatal("credential reflected")
		}
	}
}
