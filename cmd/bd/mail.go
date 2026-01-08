package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/config"
	"github.com/steveyegge/beads/internal/storage/sqlite"
)

// mailCmd delegates to an external mail provider.
// This enables agents to use 'bd mail' consistently, while the actual
// mail implementation is provided by the orchestrator.
var mailCmd = &cobra.Command{
	Use:   "mail [subcommand] [args...]",
	Short: "Delegate to mail provider (e.g., gt mail)",
	Long: `Delegates mail operations to an external mail provider.

Agents often type 'bd mail' when working with beads, but mail functionality
is typically provided by the orchestrator. This command bridges that gap
by delegating to the configured mail provider.

Configuration (checked in order):
  1. BEADS_MAIL_DELEGATE or BD_MAIL_DELEGATE environment variable
  2. 'mail.delegate' config setting (bd config set mail.delegate "gt mail")

Examples:
  # Configure delegation (one-time setup)
  export BEADS_MAIL_DELEGATE="gt mail"
  # or
  bd config set mail.delegate "gt mail"

  # Then use bd mail as if it were gt mail
  bd mail inbox                    # Lists inbox
  bd mail send mayor/ -s "Hi"      # Sends mail
  bd mail read msg-123             # Reads a message`,
	DisableFlagParsing: true, // Pass all args through to delegate
	Run: func(cmd *cobra.Command, args []string) {
		// Handle --help and -h ourselves since flag parsing is disabled
		for _, arg := range args {
			if arg == "--help" || arg == "-h" {
				_ = cmd.Help()
				return
			}
		}

		// Find the mail delegate command
		delegate := findMailDelegate()
		if delegate == "" {
			fmt.Fprintf(os.Stderr, "Error: no mail delegate configured\n\n")
			fmt.Fprintf(os.Stderr, "bd mail delegates to an external mail provider.\n")
			fmt.Fprintf(os.Stderr, "Configure one of:\n")
			fmt.Fprintf(os.Stderr, "  export BEADS_MAIL_DELEGATE=\"gt mail\"   # Environment variable\n")
			fmt.Fprintf(os.Stderr, "  bd config set mail.delegate \"gt mail\"  # Per-project config\n")
			os.Exit(1)
		}

		// Parse the delegate command (e.g., "gt mail" -> ["gt", "mail"])
		parts := strings.Fields(delegate)
		if len(parts) == 0 {
			fmt.Fprintf(os.Stderr, "Error: invalid mail delegate: %q\n", delegate)
			os.Exit(1)
		}

		// Build the full command with our args appended
		cmdName := parts[0]
		cmdArgs := append(parts[1:], args...)

		// Execute the delegate command
		// #nosec G204 - cmdName comes from user configuration (mail_delegate setting)
		execCmd := exec.Command(cmdName, cmdArgs...)
		execCmd.Stdin = os.Stdin
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		if err := execCmd.Run(); err != nil {
			// Try to preserve the exit code
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "Error running %s: %v\n", delegate, err)
			os.Exit(1)
		}
	},
}

// findMailDelegate checks for mail delegation configuration
// Priority: env vars > bd config
func findMailDelegate() string {
	// Check environment variables first
	if delegate := os.Getenv("BEADS_MAIL_DELEGATE"); delegate != "" {
		return delegate
	}
	if delegate := os.Getenv("BD_MAIL_DELEGATE"); delegate != "" {
		return delegate
	}

	// Check config.yaml / BD_* env vars via viper
	if delegate := strings.TrimSpace(config.GetString("mail.delegate")); delegate != "" {
		return delegate
	}

	// Check bd config (SQLite)
	if store != nil {
		if delegate, err := store.GetConfig(rootCtx, "mail.delegate"); err == nil && delegate != "" {
			return delegate
		}
		return ""
	}

	// Store isn't initialized (e.g., DisableFlagParsing). Open SQLite directly to read config.
	ctx := rootCtx
	if ctx == nil {
		ctx = context.Background()
	}

	db := dbPath
	if db == "" {
		db = strings.TrimSpace(config.GetString("db"))
	}
	if db == "" {
		db = beads.FindDatabasePath()
	}
	if db == "" {
		if beadsDir := beads.FindBeadsDir(); beadsDir != "" {
			db = filepath.Join(beadsDir, beads.CanonicalDatabaseName)
		}
	}
	if db == "" {
		return ""
	}

	roStore, err := sqlite.NewReadOnly(ctx, db)
	if err != nil {
		return ""
	}
	defer func() { _ = roStore.Close() }()

	delegate, err := roStore.GetConfig(ctx, "mail.delegate")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(delegate)

}

func init() {
	rootCmd.AddCommand(mailCmd)
}
