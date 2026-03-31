package commands

import (
	"context"

	"github.com/sipeed/picoclaw/pkg/config"
)

// Runtime provides runtime dependencies to command handlers. It is constructed
// per-request by the agent loop so that per-request state (like session scope)
// can coexist with long-lived callbacks (like GetModelInfo).
type Runtime struct {
	Config             *config.Config
	ConfigPath         string
	GetModelInfo       func() (name, provider string)
	ListAgentIDs       func() []string
	ListDefinitions    func() []Definition
	ListSkillNames     func() []string
	GetEnabledChannels func() []string
	GetActiveTurn      func() any // Returning any to avoid circular dependency with agent package
	SwitchModel        func(value string) (oldModel string, err error)
	SwitchChannel      func(value string) error
	ClearHistory       func() error
	ReloadConfig       func() error
	// SendMessage sends an outbound message to a channel/chat from background goroutines.
	SendMessage func(channel, chatID, content string) error
	// ProcessDirect sends a prompt through the agent loop and returns the response.
	// Used by commands that need to leverage the default agent (e.g. for web search elaboration).
	ProcessDirect func(ctx context.Context, content, sessionKey string) (string, error)
}
