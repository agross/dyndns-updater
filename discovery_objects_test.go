package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLIDiscoverySourceObjects(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
	defer srv.Close()
	j := newJourney(t)
	j.write(strings.Replace(profile("dns", "desec", srv.URL), "sources: [{if: wan}]", "sources: [{if: wan}, {web: ipify}, {url: 'https://example.invalid/'}]", 1))
	j.run(0)
	if calls != 1 || j.state().Profiles["dns"].Address != j.ip {
		t.Fatal("source objects not used")
	}
}
