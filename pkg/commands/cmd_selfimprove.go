package commands

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	maxSelfImproveAttempts       = 3
	selfImprovePendingFile       = "pending_restart_notify.json"
	selfImproveClaudeTimeout     = 30 * time.Minute
	selfImproveBuildTimeout      = 10 * time.Minute
	selfImprovePreRestartWaitSec = 3 // seconds to wait after "restarting" message before restart

	// Streaming output flush interval and limits.
	selfImproveFlushInterval = 10 * time.Second
	selfImproveMaxChunkLen   = 3500 // Telegram message limit is 4096; leave room for prefix
)

// selfImprovePendingNotify is written to disk before a service restart so that
// the new process can send a "back online" notification to the requesting user.
type selfImprovePendingNotify struct {
	Channel   string    `json:"channel"`
	ChatID    string    `json:"chat_id"`
	Timestamp time.Time `json:"timestamp"`
	Prompt    string    `json:"prompt"`
}

func selfimproveCommand() Definition {
	return Definition{
		Name:        "selfimprove",
		Description: "Improve picoclaw by running a Claude Code prompt against the source",
		Usage:       "/selfimprove <prompt>",
		Handler: func(ctx context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.SendMessage == nil {
				return req.Reply(unavailableMsg)
			}

			// Parse prompt
			parts := strings.SplitN(strings.TrimSpace(req.Text), " ", 2)
			if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
				return req.Reply("Usage: /selfimprove <prompt describing what to improve>")
			}
			prompt := strings.TrimSpace(parts[1])

			cfg := rt.Config

			// Authorization check
			if cfg != nil && len(cfg.SelfImprove.AllowFrom) > 0 {
				allowed := false
				for _, id := range cfg.SelfImprove.AllowFrom {
					if id == req.SenderID {
						allowed = true
						break
					}
				}
				if !allowed {
					return req.Reply("You are not authorized to use /selfimprove.")
				}
			}

			workDir := ""
			serviceName := "picoclaw"
			if cfg != nil {
				workDir = cfg.SelfImprove.WorkDir
				if cfg.SelfImprove.ServiceName != "" {
					serviceName = cfg.SelfImprove.ServiceName
				}
			}

			// Derive picoclaw home for the pending notification file.
			picoHome := ""
			if cfg != nil && cfg.Agents.Defaults.Workspace != "" {
				picoHome = filepath.Dir(cfg.Agents.Defaults.Workspace)
			} else {
				if home, err := os.UserHomeDir(); err == nil {
					picoHome = filepath.Join(home, ".picoclaw")
				}
			}

			channel := req.Channel
			chatID := req.ChatID
			send := rt.SendMessage

			if err := req.Reply(fmt.Sprintf(
				"Starting self-improvement (up to %d attempts)...\nPrompt: %q\n\nThis may take several minutes.",
				maxSelfImproveAttempts, prompt,
			)); err != nil {
				return err
			}

			go runSelfImprove(channel, chatID, prompt, workDir, serviceName, picoHome, send)
			return nil
		},
	}
}

func runSelfImprove(channel, chatID, prompt, workDir, serviceName, picoHome string, send func(string, string, string) error) {
	sendMsg := func(msg string) {
		_ = send(channel, chatID, msg)
		time.Sleep(300 * time.Millisecond)
	}

	// Save initial git HEAD so we can always revert to it.
	initialHead, err := gitHead(workDir)
	if err != nil {
		sendMsg("Error: cannot read git HEAD. Is work_dir configured correctly?\n" + err.Error())
		return
	}

	for attempt := 1; attempt <= maxSelfImproveAttempts; attempt++ {
		if attempt > 1 {
			sendMsg(fmt.Sprintf("Retrying... (attempt %d/%d)", attempt, maxSelfImproveAttempts))
		}

		// ── Step 1: run claude with streaming output ────────────────────────
		claudeErr := runClaudeStreaming(workDir, prompt, sendMsg)

		if claudeErr != nil {
			revertAndNotify(workDir, initialHead, attempt, fmt.Sprintf("claude exited: %v", claudeErr), sendMsg)
			if attempt == maxSelfImproveAttempts {
				sendMsg("Sorry, self-improvement failed after 3 attempts. The codebase has been reverted to its original state.")
			}
			continue
		}

		// ── Step 2: verify the build ────────────────────────────────────────
		buildCtx, buildCancel := context.WithTimeout(context.Background(), selfImproveBuildTimeout)
		buildCmd := exec.CommandContext(buildCtx, "make", "build")
		if workDir != "" {
			buildCmd.Dir = workDir
		}
		buildOut, buildErr := buildCmd.CombinedOutput()
		buildCancel()

		if buildErr != nil {
			if len(buildOut) > 0 {
				out := truncate(string(buildOut), 2000)
				sendMsg("Build verification failed:\n" + out)
			}
			revertAndNotify(workDir, initialHead, attempt, "build failed", sendMsg)
			if attempt == maxSelfImproveAttempts {
				sendMsg("Sorry, self-improvement failed after 3 attempts. The codebase has been reverted to its original state.")
			}
			continue
		}

		// ── Step 3: install ─────────────────────────────────────────────────
		installCmd := exec.Command("make", "install")
		if workDir != "" {
			installCmd.Dir = workDir
		}
		installOut, installErr := installCmd.CombinedOutput()
		if installErr != nil {
			if len(installOut) > 0 {
				out := truncate(string(installOut), 1000)
				sendMsg("make install failed:\n" + out)
			}
			revertAndNotify(workDir, initialHead, attempt, "make install failed", sendMsg)
			if attempt == maxSelfImproveAttempts {
				sendMsg("Sorry, self-improvement failed after 3 attempts. The codebase has been reverted to its original state.")
			}
			continue
		}

		// ── Step 4: persist notification, then restart ───────────────────────
		if picoHome != "" {
			pendingPath := filepath.Join(picoHome, selfImprovePendingFile)
			notify := selfImprovePendingNotify{
				Channel:   channel,
				ChatID:    chatID,
				Timestamp: time.Now(),
				Prompt:    prompt,
			}
			if data, marshalErr := json.Marshal(notify); marshalErr == nil {
				_ = os.WriteFile(pendingPath, data, 0o600)
			}
		}

		sendMsg(fmt.Sprintf(
			"Build and install successful! Restarting service %q now... I'll be back shortly.",
			serviceName,
		))
		time.Sleep(selfImprovePreRestartWaitSec * time.Second)

		_ = exec.Command("systemctl", "--user", "restart", serviceName).Run()
		// If the restart kills this process we never reach here; if for some
		// reason it doesn't (e.g., running outside of systemd), we're done.
		return
	}
}

// runClaudeStreaming runs `claude -p <prompt> --permission-mode auto` and streams
// its combined stdout+stderr to the user in periodic chunks. Returns the
// process error (nil on exit 0).
func runClaudeStreaming(workDir, prompt string, sendMsg func(string)) error {
	ctx, cancel := context.WithTimeout(context.Background(), selfImproveClaudeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "-p", prompt, "--permission-mode", "auto")
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Merge stdout and stderr into a single pipe.
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		return err
	}

	// Collect output in a streaming buffer that flushes to the user periodically.
	var (
		mu      sync.Mutex
		pending strings.Builder // unflushed lines
	)

	flush := func(label string) {
		mu.Lock()
		text := pending.String()
		pending.Reset()
		mu.Unlock()
		if text == "" {
			return
		}
		// Split into Telegram-safe chunks.
		for len(text) > 0 {
			chunk := text
			if len(chunk) > selfImproveMaxChunkLen {
				chunk = text[:selfImproveMaxChunkLen]
				text = text[selfImproveMaxChunkLen:]
			} else {
				text = ""
			}
			sendMsg(label + chunk)
		}
	}

	// Background reader: accumulates lines from the pipe.
	go func() {
		scanner := bufio.NewScanner(pr)
		// Allow long lines (Claude can produce large output).
		scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
		for scanner.Scan() {
			mu.Lock()
			pending.WriteString(scanner.Text())
			pending.WriteByte('\n')
			mu.Unlock()
		}
	}()

	// Periodic flush while the process is running.
	ticker := time.NewTicker(selfImproveFlushInterval)
	defer ticker.Stop()

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		pw.Close() // unblock the scanner
	}()

	for {
		select {
		case <-ticker.C:
			flush("[claude] ")
		case cmdErr := <-waitCh:
			// Give the scanner a moment to drain remaining bytes.
			time.Sleep(200 * time.Millisecond)
			flush("[claude] ")
			return cmdErr
		}
	}
}

// revertAndNotify resets the work tree to initialHead and reports the failure.
func revertAndNotify(workDir, initialHead string, attempt int, reason string, sendMsg func(string)) {
	if revertErr := gitResetHard(workDir, initialHead); revertErr != nil {
		sendMsg(fmt.Sprintf(
			"Attempt %d failed (%s). Revert also failed: %v",
			attempt, reason, revertErr,
		))
	} else {
		sendMsg(fmt.Sprintf(
			"Attempt %d failed (%s). Reverted to %s.",
			attempt, reason, shortRef(initialHead),
		))
	}
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
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func shortRef(ref string) string {
	if len(ref) > 7 {
		return ref[:7]
	}
	return ref
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}

// SendStartupRestartNotification checks for a pending self-improve notification and
// sends it if one exists. Call this once channels are ready after a restart.
// picoHome is the ~/.picoclaw directory.
func SendStartupRestartNotification(picoHome string, sendMsg func(channel, chatID, content string) error) {
	if picoHome == "" {
		return
	}
	pendingPath := filepath.Join(picoHome, selfImprovePendingFile)
	data, err := os.ReadFile(pendingPath)
	if err != nil {
		return
	}

	var notify selfImprovePendingNotify
	if err := json.Unmarshal(data, &notify); err != nil {
		_ = os.Remove(pendingPath)
		return
	}

	// Skip stale notifications (e.g. after a manual restart)
	if time.Since(notify.Timestamp) > 10*time.Minute {
		_ = os.Remove(pendingPath)
		return
	}

	_ = os.Remove(pendingPath)
	_ = sendMsg(notify.Channel, notify.ChatID,
		fmt.Sprintf("Self-improvement complete! I'm back online.\nOriginal prompt: %q", notify.Prompt),
	)
}
