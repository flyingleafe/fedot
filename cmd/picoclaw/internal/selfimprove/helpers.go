package selfimprove

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
	"github.com/sipeed/picoclaw/pkg/commands"
)

const buildTimeout = 10 * time.Minute

func selfImproveCmd(prompt string, noBuild, noRestart bool) error {
	cfg, err := internal.LoadConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	workDir := cfg.SelfImprove.WorkDir
	if workDir == "" {
		// Fall back to current directory
		workDir, _ = os.Getwd()
	}

	serviceName := cfg.SelfImprove.ServiceName
	if serviceName == "" {
		serviceName = "picoclaw"
	}

	// Save initial HEAD for revert on failure.
	initialHead, err := gitHead(workDir)
	if err != nil {
		return fmt.Errorf("cannot read git HEAD (is work_dir correct?): %w", err)
	}

	fmt.Printf("Running Claude Code...\nPrompt: %s\nWork dir: %s\n\n", prompt, workDir)

	// Run Claude with output streamed to stdout.
	sendMsg := func(msg string) {
		fmt.Println(msg)
	}

	claudeErr := commands.RunClaudeStreaming(workDir, prompt, sendMsg)
	if claudeErr != nil {
		fmt.Printf("\nClaude failed: %v\nReverting to %s...\n", claudeErr, initialHead[:7])
		_ = gitResetHard(workDir, initialHead)
		return fmt.Errorf("claude exited with error: %w", claudeErr)
	}

	if noBuild {
		fmt.Println("\nClaude finished. Skipping build (--no-build).")
		return nil
	}

	// Build
	fmt.Println("\nRunning make build...")
	buildCtx, buildCancel := context.WithTimeout(context.Background(), buildTimeout)
	buildCmd := exec.CommandContext(buildCtx, "make", "build")
	buildCmd.Dir = workDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	buildErr := buildCmd.Run()
	buildCancel()

	if buildErr != nil {
		fmt.Printf("\nBuild failed. Reverting to %s...\n", initialHead[:7])
		_ = gitResetHard(workDir, initialHead)
		return fmt.Errorf("build failed: %w", buildErr)
	}

	// Install
	fmt.Println("\nRunning make install...")
	installCmd := exec.Command("make", "install")
	installCmd.Dir = workDir
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	installErr := installCmd.Run()

	if installErr != nil {
		fmt.Printf("\nInstall failed. Reverting to %s...\n", initialHead[:7])
		_ = gitResetHard(workDir, initialHead)
		return fmt.Errorf("install failed: %w", installErr)
	}

	fmt.Println("\nBuild and install successful!")

	if noRestart {
		fmt.Println("Skipping service restart (--no-restart).")
		return nil
	}

	fmt.Printf("Restarting %s...\n", serviceName)
	restartCmd := exec.Command("systemctl", "--user", "restart", serviceName)
	if restartErr := restartCmd.Run(); restartErr != nil {
		fmt.Printf("Warning: restart failed: %v\n", restartErr)
		return nil
	}

	// Check if it came back up.
	time.Sleep(2 * time.Second)
	checkCmd := exec.Command("systemctl", "--user", "is-active", serviceName)
	out, _ := checkCmd.Output()
	status := strings.TrimSpace(string(out))
	if status == "active" {
		fmt.Printf("Service %s is active.\n", serviceName)
	} else {
		fmt.Printf("Warning: service status is %q after restart.\n", status)
	}

	return nil
}

func gitHead(workDir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	if workDir != "" {
		cmd.Dir = workDir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitResetHard(workDir, ref string) error {
	cmd := exec.Command("git", "reset", "--hard", ref)
	if workDir != "" {
		cmd.Dir = workDir
	}
	return cmd.Run()
}

// picoHome returns the picoclaw home directory for pending notification files.
func picoHome() string {
	if home := os.Getenv("PICOCLAW_HOME"); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".picoclaw")
}
