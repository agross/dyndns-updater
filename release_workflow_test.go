package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

type releaseWorkflow struct {
	On struct {
		Dispatch struct {
			Inputs map[string]struct {
				Required bool
				Default  string
			} `yaml:"inputs"`
		} `yaml:"workflow_dispatch"`
	} `yaml:"on"`
	Jobs map[string]struct {
		Steps []struct {
			Name string
			Run  string
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func readReleaseWorkflow(t *testing.T) releaseWorkflow {
	t.Helper()
	data, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow releaseWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow
}

func releaseCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Release Test", "GIT_AUTHOR_EMAIL=release@example.invalid", "GIT_COMMITTER_NAME=Release Test", "GIT_COMMITTER_EMAIL=release@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func runReleaseStep(t *testing.T, workflow releaseWorkflow, job, name, dir string, env ...string) (string, error) {
	t.Helper()
	for _, step := range workflow.Jobs[job].Steps {
		if step.Name == name {
			cmd := exec.Command("bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", step.Run)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), env...)
			out, err := cmd.CombinedOutput()
			return string(out), err
		}
	}
	t.Fatalf("missing release step %s/%s", job, name)
	return "", nil
}

func TestReleaseWorkflowResolve(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	if !workflow.On.Dispatch.Inputs["version"].Required || workflow.On.Dispatch.Inputs["commit"].Default != "HEAD" || workflow.On.Dispatch.Inputs["commit"].Required {
		t.Fatal("dispatch must require version and default optional commit to HEAD")
	}
	dir := t.TempDir()
	// A non-main default branch proves that resolution does not hard-code main.
	releaseCommand(t, dir, "init", "--initial-branch=trunk")
	releaseCommand(t, dir, "commit", "--allow-empty", "-m", "first")
	first := releaseCommand(t, dir, "rev-parse", "HEAD")
	releaseCommand(t, dir, "commit", "--allow-empty", "-m", "tip")
	tip := releaseCommand(t, dir, "rev-parse", "HEAD")
	releaseCommand(t, dir, "checkout", "-b", "unmerged", first)
	releaseCommand(t, dir, "commit", "--allow-empty", "-m", "unmerged")
	unmerged := releaseCommand(t, dir, "rev-parse", "HEAD")
	releaseCommand(t, dir, "checkout", "trunk")
	releaseCommand(t, dir, "checkout", "--detach", tip)
	for _, tc := range []struct{ name, input, want string }{
		{"ancestor", first, first},
		{"head", "HEAD", tip},
		{"unmerged", unmerged, ""},
		{"missing", "missing-commit", ""},
		{"option", "--all", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "output")
			out, err := runReleaseStep(t, workflow, "resolve", "Resolve commit", dir,
				"RELEASE_COMMIT="+tc.input, "GITHUB_OUTPUT="+output)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("invalid commit accepted: %s", out)
				}
				if data, _ := os.ReadFile(output); len(data) != 0 {
					t.Fatalf("rejected commit emitted output: %s", data)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve: %v\n%s", err, out)
			}
			got, err := os.ReadFile(output)
			if err != nil || string(got) != fmt.Sprintf("commit=%s\n", tc.want) {
				t.Fatalf("want release commit %s, got %q (%v)", tc.want, got, err)
			}
		})
	}
}

func TestReleaseWorkflowPublish(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	for _, existing := range []string{"absent", "matching", "conflicting"} {
		t.Run(existing, func(t *testing.T) {
			dir := t.TempDir()
			releaseCommand(t, dir, "init", "--initial-branch=trunk")
			releaseCommand(t, dir, "commit", "--allow-empty", "-m", "first")
			first := releaseCommand(t, dir, "rev-parse", "HEAD")
			releaseCommand(t, dir, "commit", "--allow-empty", "-m", "release")
			target := releaseCommand(t, dir, "rev-parse", "HEAD")
			remote := t.TempDir()
			releaseCommand(t, remote, "init", "--bare")
			releaseCommand(t, dir, "remote", "add", "origin", remote)
			if existing == "matching" {
				releaseCommand(t, dir, "tag", "v1.2.3", target)
			} else if existing == "conflicting" {
				releaseCommand(t, dir, "tag", "v1.2.3", first)
			}
			if existing != "absent" {
				releaseCommand(t, dir, "push", "origin", "refs/tags/v1.2.3")
			}
			bin := t.TempDir()
			calls := filepath.Join(bin, "calls")
			if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$RELEASE_TEST_CALLS\"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			out, err := runReleaseStep(t, workflow, "release", "Publish release", dir,
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "RELEASE_TAG=v1.2.3", "RELEASE_TEST_CALLS="+calls)
			if existing == "conflicting" {
				if err == nil {
					t.Fatalf("conflicting tag accepted: %s", out)
				}
				if _, err := os.Stat(calls); !os.IsNotExist(err) {
					t.Fatal("conflicting tag attempted publication")
				}
				return
			}
			if err != nil {
				t.Fatalf("publish: %v\n%s", err, out)
			}
			data, err := os.ReadFile(calls)
			want := "release\ncreate\nv1.2.3\ndist/*.tar.gz\ndist/checksums.txt\n--target\n" + target + "\n--generate-notes\n"
			if err != nil || string(data) != want {
				t.Fatalf("unexpected release request: %q, want %q (%v)", data, want, err)
			}
			if existing == "matching" {
				if got := releaseCommand(t, remote, "rev-parse", "refs/tags/v1.2.3^{commit}"); got != target {
					t.Fatalf("remote tag points to %s, want tested commit %s", got, target)
				}
			} else if got := releaseCommand(t, remote, "tag", "--list"); got != "" {
				t.Fatalf("workflow created a tag instead of delegating to gh: %s", got)
			}
		})
	}
}

func TestReleaseWorkflowVersion(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	for _, version := range []string{"v1.2.3", "v0.0.0", "1.2.3", "v01.2.3", "v1.2.3-rc.1", "v1.2.3\necho bad", ""} {
		t.Run(fmt.Sprintf("case_%d", len(version))+strings.ReplaceAll(version, "\n", "_"), func(t *testing.T) {
			_, err := runReleaseStep(t, workflow, "resolve", "Validate release tag", t.TempDir(), "RELEASE_TAG="+version)
			valid := version == "v1.2.3" || version == "v0.0.0"
			if (err == nil) != valid {
				t.Fatalf("version %q: expected valid=%v, got %v", version, valid, err)
			}
		})
	}
}
