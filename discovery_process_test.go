package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Built CLI journeys cannot inject an OS interface address. Serve a synthetic
// public address locally instead; no public discovery or DNS traffic is needed.
func localDiscoverySetting(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "8.0.2.10") }))
	t.Cleanup(srv.Close)
	return fmt.Sprintf("sources: [{url: %q}]", srv.URL)
}
