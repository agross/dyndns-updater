package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestCLIDNSFailureDiagnostics(t *testing.T) {
	for _, mode := range []int32{1, 2, 6} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			system := startDNS(t, "127.0.0.1:0")
			system.mode.Store(mode)
			localSystemDNS(t, system)
			first := startDNS(t, "127.0.0.1:0")
			first.mode.Store(mode)
			second := startDNS(t, "127.0.0.1:0")
			endpoint, _ := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "good") })
			j := newJourney(t)
			j.write(profile("dns", "desec", endpoint))
			j.fallback(first.address, second.address)
			out := j.run(0)
			systemLog := `dns: DNS lookup failed using system resolver`
			fallbackLog := fmt.Sprintf(`dns: DNS lookup failed using fallback resolver %q`, first.address)
			a, b := strings.Index(out, systemLog), strings.Index(out, fallbackLog)
			if a < 0 || b <= a || strings.Count(out, "DNS lookup failed using") != 2 {
				t.Fatalf("missing/incorrect DNS diagnostics: %s", out)
			}
			if strings.Contains(out, "synthetic-secret") || strings.Contains(strings.Join(j.mails, ""), "DNS lookup failed using") {
				t.Fatal("DNS diagnostics leaked credentials or entered mail")
			}
			// Disabled fallback must still log the failed system lookup.
			j.write(profile("dns", "desec", endpoint))
			j.fallback()
			j.ip = "192.0.2.11"
			out = j.run(1)
			if !strings.Contains(out, systemLog) || strings.Contains(out, "fallback resolver") {
				t.Fatalf("disabled fallback diagnostics: %s", out)
			}
		})
	}
}

func TestCLIDNSDeadlineDiagnostics(t *testing.T) {
	for _, stallSystem := range []bool{true, false} {
		t.Run(fmt.Sprint(stallSystem), func(t *testing.T) {
			system := startDNS(t, "127.0.0.1:0")
			system.mode.Store(1)
			if stallSystem {
				system.mode.Store(4)
			}
			localSystemDNS(t, system)
			fallback := startDNS(t, "127.0.0.1:0")
			fallback.mode.Store(4)
			endpoint, _ := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) { t.Error("provider reached despite DNS timeout") })
			j := newJourney(t)
			j.write(profile("dns", "desec", endpoint))
			j.fallback(fallback.address)
			out := j.run(1)
			want := 2
			if stallSystem {
				want = 1
			}
			if strings.Count(out, "DNS lookup failed using") != want || !strings.Contains(out, "system resolver") || strings.Contains(out, "DNS lookup succeeded") || (stallSystem && strings.Contains(out, "fallback resolver")) {
				t.Fatalf("missing deadline diagnostic: %s", out)
			}
		})
	}
}

func TestCLISuccessfulDNSDoesNotLogFailure(t *testing.T) {
	system := startDNS(t, "127.0.0.1:0")
	localSystemDNS(t, system)
	endpoint, _ := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "good") })
	j := newJourney(t)
	j.write(profile("dns", "desec", endpoint))
	if out := j.run(0); strings.Contains(out, "DNS lookup") || strings.Contains(out, "fallback resolver") {
		t.Fatal(out)
	}
}
