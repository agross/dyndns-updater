package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

// Execute the workflow's actual shell at its dispatch/Git boundary using only
// local repositories. No GitHub token, real tags or release publication is used.
func releaseStep(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct{ Steps []struct{ Name, Run string } }
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if step.Name == name {
				return step.Run
			}
		}
	}
	t.Fatalf("workflow step %q not found", name)
	return ""
}

func releaseGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func releaseFixture(t *testing.T) (string, string, string, string) {
	t.Helper()
	root := t.TempDir()
	remote, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	releaseGit(t, root, "init", "--bare", "--initial-branch=trunk", remote)
	releaseGit(t, root, "clone", remote, work)
	releaseGit(t, work, "config", "user.name", "Local Test")
	releaseGit(t, work, "config", "user.email", "local@example.invalid")
	releaseGit(t, work, "commit", "--allow-empty", "-m", "first")
	first := releaseGit(t, work, "rev-parse", "HEAD")
	releaseGit(t, work, "commit", "--allow-empty", "-m", "second")
	latest := releaseGit(t, work, "rev-parse", "HEAD")
	releaseGit(t, work, "push", "origin", "trunk")
	// Simulate dispatching from another branch: HEAD must still mean trunk.
	releaseGit(t, work, "checkout", "-b", "unmerged")
	releaseGit(t, work, "commit", "--allow-empty", "-m", "unmerged")
	return remote, work, first, latest
}

func runReleaseStep(t *testing.T, dir, name string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", releaseStep(t, name))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestReleaseVersion(t *testing.T) {
	for _, version := range []string{"v0.0.0", "v1.2.3", "v10.20.300", "1.2.3", "v01.2.3", "v1.2.3-rc1", "v1.2.3\ncommit=bad", "$(touch injected)"} {
		t.Run(version, func(t *testing.T) {
			_, err := runReleaseStep(t, t.TempDir(), "Validate release tag", "RELEASE_TAG="+version)
			valid := version == "v0.0.0" || version == "v1.2.3" || version == "v10.20.300"
			if (err == nil) != valid {
				t.Fatalf("version %q error %v; valid=%v", version, err, valid)
			}
		})
	}
}

func TestReleaseEnsureTag(t *testing.T) {
	for _, existing := range []string{"absent", "same", "annotated", "different"} {
		t.Run(existing, func(t *testing.T) {
			remote, work, first, latest := releaseFixture(t)
			if existing != "absent" {
				target := latest
				if existing == "different" {
					target = first
				}
				if existing == "annotated" {
					releaseGit(t, work, "tag", "-a", "v1.2.3", "-m", "release", target)
				} else {
					releaseGit(t, work, "tag", "v1.2.3", target)
				}
				releaseGit(t, work, "push", "origin", "refs/tags/v1.2.3")
			}
			before := releaseGit(t, remote, "for-each-ref", "--format=%(objectname)", "refs/tags/v1.2.3")
			releaseGit(t, work, "checkout", "--detach", latest)
			out, err := runReleaseStep(t, work, "Ensure release tag", "RELEASE_TAG=v1.2.3", "RELEASE_COMMIT="+latest)
			if existing == "different" {
				if err == nil {
					t.Fatalf("conflicting tag accepted: %s", out)
				}
			} else if err != nil {
				t.Fatalf("ensure tag: %v\n%s", err, out)
			}
			after := releaseGit(t, remote, "rev-parse", "refs/tags/v1.2.3")
			if existing != "absent" && before != after {
				t.Fatal("existing tag overwritten")
			}
			want := latest
			if existing == "different" {
				want = first
			}
			if got := releaseGit(t, remote, "rev-parse", "refs/tags/v1.2.3^{commit}"); got != want {
				t.Fatalf("tag points to %s, want %s", got, want)
			}
		})
	}
}

func TestReleaseWorkflowGates(t *testing.T) {
	data, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		On struct {
			Dispatch struct {
				Inputs map[string]struct {
					Required bool
					Default  string
				}
			} `yaml:"workflow_dispatch"`
		}
		Jobs map[string]struct {
			Needs any
			Uses  string
			With  map[string]string
			Steps []struct {
				Name      string
				With, Env map[string]string
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	inputs := workflow.On.Dispatch.Inputs
	if !inputs["version"].Required || inputs["commit"].Required || inputs["commit"].Default != "HEAD" {
		t.Fatal("dispatch input contract changed")
	}
	test := workflow.Jobs["test"]
	if test.Uses != "./.github/workflows/ci.yml" || test.With["ref"] != "${{ needs.resolve.outputs.commit }}" {
		t.Fatal("CI must test the pinned resolved commit")
	}
	release := workflow.Jobs["release"]
	if needs, ok := release.Needs.([]any); !ok || len(needs) != 2 || needs[0] != "resolve" || needs[1] != "test" {
		t.Fatal("release must depend on resolution and passing tests")
	}
	buildIndex, tagIndex, publishIndex := -1, -1, -1
	for i, step := range release.Steps {
		if i == 0 && step.With["ref"] != "${{ needs.resolve.outputs.commit }}" {
			t.Fatal("release checkout must use tested commit")
		}
		switch step.Name {
		case "Build release archives":
			buildIndex = i
		case "Ensure release tag":
			tagIndex = i
			if step.Env["RELEASE_COMMIT"] != "${{ needs.resolve.outputs.commit }}" {
				t.Fatal("tag must use tested commit")
			}
		case "Publish release":
			publishIndex = i
		}
	}
	if buildIndex < 0 || tagIndex <= buildIndex || publishIndex <= tagIndex {
		t.Fatal("build, tag and publish must run in order after tests")
	}
}

func TestReleaseTagInspectionFailure(t *testing.T) {
	remote, work, _, latest := releaseFixture(t)
	if err := os.Rename(remote, remote+".unavailable"); err != nil {
		t.Fatal(err)
	}
	out, err := runReleaseStep(t, work, "Ensure release tag", "RELEASE_TAG=v1.2.3", "RELEASE_COMMIT="+latest)
	if err == nil || !strings.Contains(out, "Could not inspect remote release tag") {
		t.Fatalf("must distinguish inaccessible remote from absent tag: %v\n%s", err, out)
	}
	if tags := releaseGit(t, remote+".unavailable", "tag", "--list"); tags != "" {
		t.Fatalf("unexpected tags: %s", tags)
	}
}

func TestReleaseResolveCommit(t *testing.T) {
	for _, selection := range []string{"HEAD", "", "older", "unmerged", "missing", "--help", "HEAD\ncommit=bad"} {
		t.Run(selection, func(t *testing.T) {
			_, work, first, latest := releaseFixture(t)
			input, want := selection, latest
			if selection == "older" {
				input, want = first, first
			}
			output := filepath.Join(t.TempDir(), "output")
			out, err := runReleaseStep(t, work, "Resolve commit", "DEFAULT_BRANCH=trunk", "RELEASE_COMMIT="+input, "GITHUB_OUTPUT="+output, "RELEASE_TAG=v1.2.3")
			valid := selection == "HEAD" || selection == "" || selection == "older"
			if !valid {
				if err == nil {
					t.Fatalf("unsafe/unreachable selection accepted: %s", out)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve %q: %v\n%s", input, err, out)
			}
			data, err := os.ReadFile(output)
			if err != nil || string(data) != "commit="+want+"\n" {
				t.Fatalf("resolved output %q, error %v; want %s", data, err, want)
			}
		})
	}
}
