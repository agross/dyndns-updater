package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func sourceProfile(name, endpoint string, sources ...string) string {
	var entries []string
	for _, s := range sources {
		selector, value, _ := strings.Cut(s, ":")
		if selector == "web" && strings.HasPrefix(value, "http") {
			selector = "url"
		}
		entries = append(entries, fmt.Sprintf("{%s: %q}", selector, value))
	}
	return strings.Replace(profile(name, "desec", endpoint), "sources: [{if: wan}]", "sources: ["+strings.Join(entries, ", ")+"]", 1)
}

func TestCLIDiscoveryOrderedSources(t *testing.T) {
	for _, tc := range []struct {
		name, ip string
		missing  bool
		sources  []string
		wantWeb  int32
		want     string
	}{
		{"interface first", "8.8.4.4", false, []string{"if:wan", "web:URL"}, 0, "8.8.4.4"},
		{"private falls through", "10.0.0.1", false, []string{"if:wan", "web:URL"}, 1, "9.9.9.9"},
		{"missing falls through", "", true, []string{"if:wan", "web:URL"}, 1, "9.9.9.9"},
		{"web stops before interface", "8.8.4.4", true, []string{"web:URL", "if:wan", "web:URL"}, 1, "9.9.9.9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var web, provider atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/discover" {
					web.Add(1)
					if r.Header.Get("Authorization") != "" {
						t.Error("discovery received authentication")
					}
					fmt.Fprint(w, " \n9.9.9.9	")
					return
				}
				provider.Add(1)
				if r.URL.Query().Get("myipv4") != tc.want || r.URL.Query().Get("myipv6") != "preserve" {
					t.Error("incorrect provider address/preservation")
				}
				fmt.Fprint(w, "good")
			}))
			defer srv.Close()
			j := newJourney(t)
			j.ip = tc.ip
			j.missing = tc.missing
			var sources []string
			for _, s := range tc.sources {
				sources = append(sources, strings.ReplaceAll(s, "URL", srv.URL+"/discover"))
			}
			j.write(sourceProfile("dns", srv.URL+"/provider", sources...))
			j.run(0)
			if web.Load() != tc.wantWeb || provider.Load() != 1 || j.state().Profiles["dns"].Address != tc.want {
				t.Fatalf("web=%d provider=%d state=%+v", web.Load(), provider.Load(), j.state())
			}
		})
	}
}
