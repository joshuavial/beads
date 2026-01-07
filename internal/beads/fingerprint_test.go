package beads

import (
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestComputeRepoID_PrefersUpstreamRemote(t *testing.T) {
	repoDir := initTestGitRepo(t)

	originURL := "https://github.com/example-fork/beads.git"
	upstreamURL := "https://github.com/steveyegge/beads.git"

	runGit(t, repoDir, "remote", "add", "origin", originURL)
	runGit(t, repoDir, "remote", "add", "upstream", upstreamURL)

	t.Chdir(repoDir)

	got, err := ComputeRepoID()
	if err != nil {
		t.Fatalf("ComputeRepoID() error: %v", err)
	}

	want := expectedRepoIDFromURL(t, upstreamURL)
	if got != want {
		t.Fatalf("ComputeRepoID() = %q, want %q (from upstream remote)", got, want)
	}
}

func TestComputeRepoID_UsesOriginWhenNoUpstream(t *testing.T) {
	repoDir := initTestGitRepo(t)

	originURL := "https://github.com/steveyegge/beads.git"
	runGit(t, repoDir, "remote", "add", "origin", originURL)

	t.Chdir(repoDir)

	got, err := ComputeRepoID()
	if err != nil {
		t.Fatalf("ComputeRepoID() error: %v", err)
	}

	want := expectedRepoIDFromURL(t, originURL)
	if got != want {
		t.Fatalf("ComputeRepoID() = %q, want %q (from origin remote)", got, want)
	}
}

func TestComputeRepoID_FallsBackToPathWhenNoRemote(t *testing.T) {
	repoDir := initTestGitRepo(t)
	t.Chdir(repoDir)

	got, err := ComputeRepoID()
	if err != nil {
		t.Fatalf("ComputeRepoID() error: %v", err)
	}

	absPath, err := filepath.Abs(repoDir)
	if err != nil {
		absPath = repoDir
	}
	evalPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		evalPath = absPath
	}

	normalized := filepath.ToSlash(evalPath)
	sum := sha256.Sum256([]byte(normalized))
	want := hex.EncodeToString(sum[:16])

	if got != want {
		t.Fatalf("ComputeRepoID() = %q, want %q (path fingerprint)", got, want)
	}
}

func expectedRepoIDFromURL(t *testing.T, rawURL string) string {
	t.Helper()

	canonical, err := canonicalizeGitURL(rawURL)
	if err != nil {
		t.Fatalf("canonicalizeGitURL(%q): %v", rawURL, err)
	}

	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:16])
}

func initTestGitRepo(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Skipf("git not available: %v", err)
	}

	// Configure git user for the test repo (some git operations require identity).
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	return repoDir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
