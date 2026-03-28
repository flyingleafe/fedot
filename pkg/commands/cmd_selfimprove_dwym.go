package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	dwymElaborationTimeout = 5 * time.Minute
	dwymElaborationPrompt  = `You are a prompt elaborator for a self-improvement coding system. The user wants to modify the picoclaw codebase. Your job is to:

1. Analyze the user's request carefully
2. Use web_search to find relevant documentation, best practices, API references, or examples that would help implement this request
3. Read relevant source files in the codebase to understand the current implementation
4. Produce a detailed, actionable implementation plan

The user's request is:
%s

Produce a plan that includes:
- What the change is about (context from web search if relevant)
- Which files need to be modified or created
- Step-by-step implementation details with code-level specifics
- Any gotchas or edge cases to watch out for

Be thorough but concise. The plan will be passed to Claude Code for implementation.
Output ONLY the plan, no preamble.`
)

// pendingDWYMPlan stores an elaborated plan awaiting user confirmation.
type pendingDWYMPlan struct {
	Plan        string
	OrigPrompt  string
	WorkDir     string
	ServiceName string
	PicoHome    string
	Channel     string
	ChatID      string
	Send        func(string, string, string) error
	CreatedAt   time.Time
}

// dwymPendingPlans maps "channel:chatID" to a pending plan awaiting confirmation.
var dwymPendingPlans sync.Map

func selfimproveDWYMCommand() Definition {
	return Definition{
		Name:        "selfimprove_dwym",
		Description: "Self-improve with AI-elaborated prompt (Do What You Mean)",
		Usage:       "/selfimprove_dwym <prompt> — or /selfimprove_dwym go — to confirm",
		Aliases:     []string{"selfimprovedwym"},
		Handler: func(ctx context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.SendMessage == nil {
				return req.Reply(unavailableMsg)
			}

			parts := strings.SplitN(strings.TrimSpace(req.Text), " ", 2)
			if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
				return req.Reply("Usage: /selfimprove_dwym <prompt>\nAfter reviewing the plan, use /selfimprove_dwym go to confirm.")
			}
			arg := strings.TrimSpace(parts[1])

			cfg := rt.Config

			// Authorization check (same as selfimprove)
			if cfg != nil && len(cfg.SelfImprove.AllowFrom) > 0 {
				allowed := false
				for _, id := range cfg.SelfImprove.AllowFrom {
					if id == req.SenderID {
						allowed = true
						break
					}
				}
				if !allowed {
					return req.Reply("You are not authorized to use /selfimprove_dwym.")
				}
			}

			key := req.Channel + ":" + req.ChatID

			// Handle "go" / "yes" / "confirm" — execute pending plan
			lower := strings.ToLower(arg)
			if lower == "go" || lower == "yes" || lower == "confirm" {
				val, ok := dwymPendingPlans.LoadAndDelete(key)
				if !ok {
					return req.Reply("No pending plan to confirm. Use /selfimprove_dwym <prompt> first.")
				}
				pending := val.(*pendingDWYMPlan)
				if time.Since(pending.CreatedAt) > 30*time.Minute {
					return req.Reply("The pending plan has expired (>30min). Please run /selfimprove_dwym again.")
				}

				if err := req.Reply("Confirmed! Starting Claude Code with the elaborated plan..."); err != nil {
					return err
				}
				go runSelfImprove(
					pending.Channel, pending.ChatID, pending.Plan,
					pending.WorkDir, pending.ServiceName, pending.PicoHome, pending.Send,
				)
				return nil
			}

			// Handle "cancel"
			if lower == "cancel" || lower == "no" {
				if _, ok := dwymPendingPlans.LoadAndDelete(key); ok {
					return req.Reply("Plan cancelled.")
				}
				return req.Reply("No pending plan to cancel.")
			}

			// Otherwise — this is a new prompt to elaborate
			prompt := arg

			workDir := ""
			serviceName := "picoclaw"
			if cfg != nil {
				workDir = cfg.SelfImprove.WorkDir
				if cfg.SelfImprove.ServiceName != "" {
					serviceName = cfg.SelfImprove.ServiceName
				}
			}

			picoHome := ""
			if cfg != nil && cfg.Agents.Defaults.Workspace != "" {
				picoHome = filepath.Dir(cfg.Agents.Defaults.Workspace)
			} else {
				if home, err := os.UserHomeDir(); err == nil {
					picoHome = filepath.Join(home, ".picoclaw")
				}
			}

			if rt.ProcessDirect == nil {
				return req.Reply("ProcessDirect is not available. Cannot elaborate prompt.")
			}

			if err := req.Reply(fmt.Sprintf(
				"Elaborating your request using web search and code analysis...\nPrompt: %q\n\nThis may take a few minutes.",
				prompt,
			)); err != nil {
				return err
			}

			channel := req.Channel
			chatID := req.ChatID
			send := rt.SendMessage
			processDirect := rt.ProcessDirect

			go func() {
				sendMsg := func(msg string) {
					_ = send(channel, chatID, msg)
					time.Sleep(300 * time.Millisecond)
				}

				elaborationCtx, cancel := context.WithTimeout(context.Background(), dwymElaborationTimeout)
				defer cancel()

				sessionKey := fmt.Sprintf("dwym-elaboration:%s:%d", key, time.Now().UnixNano())
				fullPrompt := fmt.Sprintf(dwymElaborationPrompt, prompt)

				plan, err := processDirect(elaborationCtx, fullPrompt, sessionKey)
				if err != nil {
					sendMsg(fmt.Sprintf("Elaboration failed: %v", err))
					return
				}

				if strings.TrimSpace(plan) == "" {
					sendMsg("Elaboration produced an empty plan. Please try again with more detail.")
					return
				}

				// Store the pending plan
				dwymPendingPlans.Store(key, &pendingDWYMPlan{
					Plan:        plan,
					OrigPrompt:  prompt,
					WorkDir:     workDir,
					ServiceName: serviceName,
					PicoHome:    picoHome,
					Channel:     channel,
					ChatID:      chatID,
					Send:        send,
					CreatedAt:   time.Now(),
				})

				// Send the plan to the user
				sendMsg("=== Elaborated Plan ===")
				// Split plan into Telegram-safe chunks
				remaining := plan
				for len(remaining) > 0 {
					chunk := remaining
					if len(chunk) > selfImproveMaxChunkLen {
						chunk = remaining[:selfImproveMaxChunkLen]
						remaining = remaining[selfImproveMaxChunkLen:]
					} else {
						remaining = ""
					}
					sendMsg(chunk)
				}
				sendMsg("=== End of Plan ===\n\nReply /selfimprove_dwym go to execute, or /selfimprove_dwym cancel to abort.")
			}()

			return nil
		},
	}
}
