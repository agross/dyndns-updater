package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestCLIFallbackResolutionTrace(t *testing.T) {
	for _, tc := range []struct {
		name    string
		modes   []int32
		success bool
	}{
		{"first", []int32{0, 1}, true},
		{"later", []int32{1, 2, 6, 0, 1}, true},
		{"exhausted", []int32{1, 2, 6}, false},
		{"timeout", []int32{4, 0}, false},
		{"disabled", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			system := startDNS(t, "127.0.0.1:0")
			system.mode.Store(1)
			localSystemDNS(t, system)
			var servers []*fakeDNS
			var addresses []string
			for _, mode := range tc.modes {
				server := startDNS(t, "127.0.0.1:0")
				server.mode.Store(mode)
				servers = append(servers, server)
				addresses = append(addresses, server.address)
			}
			calls := 0
			endpoint, _ := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") })
			j := newJourney(t)
			name := `Tunnel Broker: café "quoted" \n %s`
			j.write(profile(name, "desec", endpoint))
			j.fallback(addresses...)
			exit := 1
			if tc.success {
				exit = 0
			}
			out := j.run(exit)
			want := name + ": DNS lookup failed using system resolver (lookup error or no usable addresses)\n"
			stopped := false
			for i, mode := range tc.modes {
				server := servers[i]
				if stopped {
					if server.udp.Load() != 0 || strings.Contains(out, server.address) {
						t.Fatalf("unattempted resolver logged/contacted: %s", out)
					}
					continue
				}
				if server.udp.Load() == 0 {
					t.Fatal("logged resolver was not contacted")
				}
				resolver := fmt.Sprintf("fallback resolver %q", server.address)
				want += name + ": DNS lookup attempting " + resolver + "\n"
				if mode == 0 {
					want += name + ": DNS lookup succeeded using " + resolver + "\n"
					stopped = true
				} else {
					want += name + ": DNS lookup failed using " + resolver + " (lookup error or no usable addresses)\n"
					stopped = mode == 4
				}
			}
			var trace string
			for _, line := range strings.SplitAfter(out, "\n") {
				if strings.Contains(line, ": DNS lookup ") {
					trace += line
				}
			}
			if trace != want {
				t.Fatalf("trace=%q want=%q", trace, want)
			}
			if !strings.HasPrefix(out, name+": interface ") || strings.Contains(out, strconv.Quote(name)+":") {
				t.Fatalf("quoted name: %s", out)
			}
			if strings.Contains(out, "synthetic-secret") || strings.Contains(out, "provider.invalid") || strings.Contains(out, "https://") || strings.Contains(out, "lookup provider") {
				t.Fatalf("unsafe output: %s", out)
			}
			if strings.Contains(strings.Join(j.mails, ""), "DNS lookup") {
				t.Fatal("DNS trace entered notification")
			}
			if tc.success && (calls != 1 || j.state().Profiles[name].Address != j.ip) {
				t.Fatal("success not confirmed")
			}
			if !tc.success && (calls != 0 || j.state().Profiles[name].Address != "" || strings.Contains(out, "DNS lookup succeeded")) {
				t.Fatalf("false success: %s", out)
			}
		})
	}
}
