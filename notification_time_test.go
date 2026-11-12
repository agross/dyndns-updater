package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCLINotificationGenerationTime(t *testing.T) {
	j := newJourney(t)
	calls := 0
	bad := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if bad {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	j.write(profile("dns", "desec", srv.URL))
	j.now = time.Date(2026, 2, 3, 4, 5, 6, 987654321, time.FixedZone("local", 7200))
	j.mailError = true
	out := j.run(1)
	first := "Summary generated at: 2026-02-03T02:05:06Z\ndns: confirmed 8.0.2.10\n"
	if !strings.Contains(out, "[error] mail failed") || strings.Contains(out, "Summary generated") || !reflect.DeepEqual(j.state().Pending, []string{first}) {
		t.Fatalf("output=%q pending=%q", out, j.state().Pending)
	}
	j.now = j.now.Add(time.Hour)
	j.run(1)
	if calls != 1 || !reflect.DeepEqual(j.state().Pending, []string{first}) {
		t.Fatal("retry changed queued summary or repeated provider")
	}
	j.ip = "8.0.2.20"
	j.run(1)
	second := "Summary generated at: 2026-02-03T03:05:06Z\ndns: confirmed 8.0.2.20\n"
	j.now = j.now.Add(time.Hour)
	j.mailError = false
	j.run(0)
	if calls != 2 || !reflect.DeepEqual(j.mails, []string{first, second}) || !reflect.DeepEqual(j.mailAttempts, []string{first, first, first, first, second}) || len(j.state().Pending) != 0 {
		t.Fatalf("delivery=%q calls=%d", j.mails, calls)
	}
	// Simulate interruption after provider checkpoint, before summary creation.
	s := j.state()
	s.Dirty = true
	if err := saveState(filepath.Join(j.dir, "state.json"), s); err != nil {
		t.Fatal(err)
	}
	j.now = j.now.Add(time.Hour)
	j.run(0)
	recovered := "Summary generated at: 2026-02-03T05:05:06Z\ndns: unchanged\n"
	if calls != 2 || len(j.mails) != 3 || j.mails[2] != recovered {
		t.Fatalf("recovery=%q calls=%d", j.mails, calls)
	}
	// Failure/recovery each queues a newly timed summary; repeated errors and no-ops do not.
	bad = true
	j.ip = "8.0.2.30"
	j.now = j.now.Add(time.Hour)
	j.run(1)
	failure := "Summary generated at: 2026-02-03T06:05:06Z\ndns: provider HTTP failure (503)\n"
	j.now = j.now.Add(time.Hour)
	j.run(1)
	bad = false
	j.run(0)
	recovery := "Summary generated at: 2026-02-03T07:05:06Z\ndns: confirmed 8.0.2.30\n"
	j.run(0)
	if calls != 5 || !reflect.DeepEqual(j.mails, []string{first, second, recovered, failure, recovery}) {
		t.Fatalf("transitions=%q calls=%d", j.mails, calls)
	}
}
