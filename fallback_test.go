package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIInvalidFallbackBeforeSideEffects(t *testing.T) {
	for _, value := range []string{"resolver.example", "127.0.0.1:0", "127.0.0.1:65536", "[::1]:bad", "[::1]", "127.0.0.1:", "", "127.0.0.1:domain", "192.0.2.1%zone"} {
		t.Run(value, func(t *testing.T) {
			j := newJourney(t)
			j.write(profile("dns", "desec", "https://example.invalid/"))
			b, err := os.ReadFile(j.config)
			if err != nil {
				t.Fatal(err)
			}
			b = append(b, []byte(fmt.Sprintf("fallback_dns: [\"192.0.2.53\", %q]\n", value))...)
			if err := os.WriteFile(j.config, b, 0600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			called := false
			d := productionDependencies()
			d.address = func(string) (string, error) { called = true; return "192.0.2.1", nil }
			d.mail = func(Config, string) error { called = true; return nil }
			if runCLI([]string{"--config", j.config}, &out, d) == 0 || called {
				t.Fatalf("invalid config accepted: %s", out.String())
			}
			if !strings.Contains(out.String(), "fallback DNS") {
				t.Fatalf("missing safe diagnostic: %s", out.String())
			}
			if _, err := os.Stat(filepath.Join(j.dir, "state.json.lock")); !os.IsNotExist(err) {
				t.Fatal("state touched")
			}
		})
	}
}
