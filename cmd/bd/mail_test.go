package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/config"
	"github.com/steveyegge/beads/internal/storage/sqlite"
)

func TestFindMailDelegate_ReadsSQLiteConfigWithoutGlobalStore(t *testing.T) {
	ensureCleanGlobalState(t)

	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	// Re-init viper so config lookups are scoped to the temp project.
	if err := config.Initialize(); err != nil {
		t.Fatalf("config.Initialize: %v", err)
	}

	// Ensure env vars don't override.
	_ = os.Unsetenv("BEADS_MAIL_DELEGATE")
	_ = os.Unsetenv("BD_MAIL_DELEGATE")

	dbPath = ""
	rootCtx = nil
	store = nil

	dbFile := filepath.Join(tmpDir, ".beads", beads.CanonicalDatabaseName)
	if err := os.MkdirAll(filepath.Dir(dbFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	st, err := sqlite.New(context.Background(), dbFile)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	if err := st.SetConfig(context.Background(), "issue_prefix", "test"); err != nil {
		_ = st.Close()
		t.Fatalf("SetConfig(issue_prefix): %v", err)
	}
	if err := st.SetConfig(context.Background(), "mail.delegate", "gt mail"); err != nil {
		_ = st.Close()
		t.Fatalf("SetConfig(mail.delegate): %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got := findMailDelegate()
	if got != "gt mail" {
		t.Fatalf("findMailDelegate()=%q, want %q", got, "gt mail")
	}
}
