package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func namedProfile(key, endpoint string) string {
	return fmt.Sprintf("  %s:\n    provider: desec\n    interface: wan\n    target: example.invalid\n    token: synthetic-secret\n    endpoint: %s\n", key, endpoint)
}

func TestCLIMappingNames(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
	defer srv.Close()
	name := "Home DNS"
	j.write(namedProfile(name, srv.URL))
	want := "\"Home DNS\": interface \"wan\" detected IPv4 192.0.2.10\n\"Home DNS\": updating provider: no confirmed state\n\"Home DNS\": confirmed 192.0.2.10\n"
	if got := j.run(0); got != want {
		t.Fatalf("output=%q want=%q", got, want)
	}
	if j.state().Profiles[name].Address != j.ip || len(j.mails) != 1 || j.mails[0] != "Summary generated at: 2026-01-01T00:00:00Z\nHome DNS: confirmed 192.0.2.10\n" {
		t.Fatal("name not retained across state and notification")
	}
	j.write(namedProfile(strconv.Quote(name), srv.URL))
	if got := j.run(0); got != "\"Home DNS\": interface \"wan\" detected IPv4 192.0.2.10\n\"Home DNS\": skipping update: address unchanged and refresh not due\n\"Home DNS\": unchanged\n" {
		t.Fatalf("reuse output=%q", got)
	}
	if calls != 1 || len(j.mails) != 1 {
		t.Fatal("YAML spelling changed identity")
	}
}

func TestCLIRejectUnsafeMappingNames(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
	defer srv.Close()
	for _, key := range []string{"''", "17", "true", "null", "[one, two]", `"line\nBcc: forged"`, `"nul\u0000"`, `"delete\u007f"`, `"next\u0085"`, `"bidi\u202e"`, `"separator\u2028"`} {
		t.Run(key, func(t *testing.T) {
			j.write(namedProfile(key, srv.URL))
			out := j.run(1)
			if !strings.HasPrefix(out, "[error] configuration ") || strings.Contains(out, "forged") {
				t.Fatalf("unsafe diagnostic=%q", out)
			}
		})
	}
	for _, profiles := range []string{
		namedProfile("dns", srv.URL) + namedProfile(`"dns"`, srv.URL),
		namedProfile("dns", srv.URL) + "    name: synthetic-secret\n",
		namedProfile("dns", srv.URL) + "    unknown: synthetic-secret\n",
		"  - name: dns\n    token: synthetic-secret\n",
	} {
		j.write(profiles)
		out := j.run(1)
		if strings.Contains(profiles, "- name:") && !strings.Contains(out, "mapping") {
			t.Fatalf("missing migration diagnostic=%q", out)
		}
	}
	if calls != 0 || len(j.mails) != 0 {
		t.Fatal("invalid configuration performed external actions")
	}
	if _, err := os.Stat(filepath.Join(j.dir, "state.json")); !os.IsNotExist(err) {
		t.Fatal("invalid configuration wrote state")
	}
}

func TestCLISafeNameRenderingAndOrder(t *testing.T) {
	j := newJourney(t)
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.URL.Path == "/bad" {
			fmt.Fprint(w, "badauth synthetic-secret")
			return
		}
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	names := []string{"z: \\\"quoted", "[error] Bcc: person", " Home DNS "}
	j.write(namedProfile(strconv.Quote(names[0]), srv.URL+"/bad") + namedProfile(strconv.Quote(names[1]), srv.URL+"/good") + namedProfile(strconv.Quote(names[2]), srv.URL+"/first"))
	out := j.run(1)
	summary := strconv.Quote(names[2]) + ": confirmed 192.0.2.10\n" + strconv.Quote(names[1]) + ": confirmed 192.0.2.10\n" + strconv.Quote(names[0]) + ": provider rejected update or returned malformed success\n"
	if !strings.HasSuffix(out, summary) || len(j.mails) != 1 || j.mails[0] != "Summary generated at: 2026-01-01T00:00:00Z\n"+names[2]+": confirmed 192.0.2.10\n"+names[1]+": confirmed 192.0.2.10\n"+names[0]+": provider rejected update or returned malformed success\n" || strings.Join(requests, ",") != "/first,/good,/bad" {
		t.Fatalf("ordering/rendering: output=%q mail=%q requests=%v", out, j.mails, requests)
	}
	s := j.state()
	if s.Profiles[names[2]].Address != j.ip || s.Profiles[names[0]].Address != "" || !s.Profiles[names[0]].Failing {
		t.Fatal("independent outcomes lost")
	}
	j.run(1)
	if len(requests) != 4 || requests[3] != "/bad" || len(j.mails) != 1 {
		t.Fatal("retry repeated success or mail")
	}
}

func TestCLIUnsafeStatePreserved(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
	defer srv.Close()
	j.write(namedProfile(`"Home DNS"`, srv.URL))
	path := filepath.Join(j.dir, "state.json")
	for _, bad := range []string{
		`{"version":1,"profiles":{"bad\nname":{"identity":"x"}}}`,
		`{"version":1,"profiles":{"":{"identity":"x"}}}`,
		`{"version":1,"profiles":{},"pending":["dns: unchanged\nBcc: forged\n"]}`,
		`{"version":1,"profiles":{},"pending":["dns: unchanged\r\n"]}`,
		`{"version":1,"profiles":{},"pending":["\"bad\nname\": unchanged\n"]}`,
		`{"version":1,"profiles":{},"pending":["dns: synthetic-secret\n"]}`,
		`{"version":1,"profiles":{},"pending":[""]}`,
		`{"version":1,"profiles":{"dns":{"identity":"old"},"\u0064ns":{"identity":"new"}}}`,
		`{"version":1,"profiles":{},"pending":[],"pending":["dns: unchanged\n"]}`,
		"{\"version\":1,\"profiles\":{\"\xff\":{\"identity\":\"x\"}}}",
	} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if got := j.run(1); !strings.Contains(got, "preserved") || strings.Contains(got, "forged") {
			t.Fatalf("unsafe state diagnostic=%q", got)
		}
		b, err := os.ReadFile(path)
		if err != nil || string(b) != bad || calls != 0 || len(j.mails) != 0 {
			t.Fatal("unsafe state overwritten or external action taken")
		}
	}
}
