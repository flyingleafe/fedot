package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
)

// serviceEntry describes a configurable secret slot.
type serviceEntry struct {
	// Description shown in /secret list output.
	Description string
	// SetKey writes the API key into cfg.
	SetKey func(cfg *config.Config, key string)
	// GetKey returns the current API key (empty string if unset).
	GetKey func(cfg *config.Config) string
	// ClearKey removes the API key from cfg.
	ClearKey func(cfg *config.Config)
}

// serviceRegistry maps service names to their config accessors.
// Only services that store keys in .security.yml are included here;
// model provider API keys use `picoclaw auth login` instead.
var serviceRegistry = map[string]serviceEntry{
	"brave": {
		Description: "Brave Search API",
		SetKey:      func(cfg *config.Config, key string) { cfg.Tools.Web.Brave.SetAPIKey(key) },
		GetKey:      func(cfg *config.Config) string { return cfg.Tools.Web.Brave.APIKey() },
		ClearKey:    func(cfg *config.Config) { cfg.Tools.Web.Brave.APIKeys = nil },
	},
	"tavily": {
		Description: "Tavily Search API",
		SetKey:      func(cfg *config.Config, key string) { cfg.Tools.Web.Tavily.SetAPIKey(key) },
		GetKey:      func(cfg *config.Config) string { return cfg.Tools.Web.Tavily.APIKey() },
		ClearKey:    func(cfg *config.Config) { cfg.Tools.Web.Tavily.APIKeys = nil },
	},
	"perplexity": {
		Description: "Perplexity API",
		SetKey:      func(cfg *config.Config, key string) { cfg.Tools.Web.Perplexity.SetAPIKey(key) },
		GetKey:      func(cfg *config.Config) string { return cfg.Tools.Web.Perplexity.APIKey() },
		ClearKey:    func(cfg *config.Config) { cfg.Tools.Web.Perplexity.APIKeys = nil },
	},
	"glm_search": {
		Description: "GLM/Zhipu Web Search API",
		SetKey: func(cfg *config.Config, key string) {
			cfg.Tools.Web.GLMSearch.APIKey = *config.NewSecureString(key)
		},
		GetKey:   func(cfg *config.Config) string { return cfg.Tools.Web.GLMSearch.APIKey.String() },
		ClearKey: func(cfg *config.Config) { cfg.Tools.Web.GLMSearch.APIKey = config.SecureString{} },
	},
	"baidu_search": {
		Description: "Baidu Search API",
		SetKey: func(cfg *config.Config, key string) {
			cfg.Tools.Web.BaiduSearch.APIKey = *config.NewSecureString(key)
		},
		GetKey:   func(cfg *config.Config) string { return cfg.Tools.Web.BaiduSearch.APIKey.String() },
		ClearKey: func(cfg *config.Config) { cfg.Tools.Web.BaiduSearch.APIKey = config.SecureString{} },
	},
	"github": {
		Description: "GitHub token (skill installation)",
		SetKey: func(cfg *config.Config, key string) {
			cfg.Tools.Skills.Github.Token = *config.NewSecureString(key)
		},
		GetKey:   func(cfg *config.Config) string { return cfg.Tools.Skills.Github.Token.String() },
		ClearKey: func(cfg *config.Config) { cfg.Tools.Skills.Github.Token = config.SecureString{} },
	},
	"clawhub": {
		Description: "ClawHub registry auth token",
		SetKey: func(cfg *config.Config, key string) {
			cfg.Tools.Skills.Registries.ClawHub.AuthToken = *config.NewSecureString(key)
		},
		GetKey: func(cfg *config.Config) string {
			return cfg.Tools.Skills.Registries.ClawHub.AuthToken.String()
		},
		ClearKey: func(cfg *config.Config) {
			cfg.Tools.Skills.Registries.ClawHub.AuthToken = config.SecureString{}
		},
	},
}

// GetServiceEntry returns the service entry for the given name, if it exists.
func GetServiceEntry(name string) (serviceEntry, bool) {
	e, ok := serviceRegistry[name]
	return e, ok
}

// SupportedServiceNames returns the sorted list of supported service names.
func SupportedServiceNames() []string {
	names := make([]string, 0, len(serviceRegistry))
	for name := range serviceRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func secretCommand() Definition {
	return Definition{
		Name:        "secret",
		Description: "Manage API keys (never sent to AI)",
		Sensitive:   true,
		SubCommands: []SubCommand{
			{
				Name:        "set",
				Description: "Save an API key",
				ArgsUsage:   "<service> <key>",
				Handler:     secretSetHandler,
			},
			{
				Name:        "list",
				Description: "Show configured services",
				Handler:     secretListHandler,
			},
			{
				Name:        "remove",
				Description: "Remove an API key",
				ArgsUsage:   "<service>",
				Handler:     secretRemoveHandler,
			},
		},
	}
}

func secretSetHandler(_ context.Context, req Request, rt *Runtime) error {
	// Parse: /secret set <service> <key>
	service := nthToken(req.Text, 2)
	key := nthToken(req.Text, 3)

	if service == "" || key == "" {
		return req.Reply("Usage: /secret set <service> <key>\nServices: " + strings.Join(SupportedServiceNames(), ", "))
	}

	entry, ok := serviceRegistry[service]
	if !ok {
		return req.Reply("Unknown service: " + service + "\nSupported: " + strings.Join(SupportedServiceNames(), ", "))
	}

	// Delete the original message to remove the key from chat history
	if req.DeleteMessage != nil {
		_ = req.DeleteMessage()
	}

	cfg, configPath, err := loadConfigForSecret(rt)
	if err != nil {
		return req.Reply("Failed to load config: " + err.Error())
	}

	entry.SetKey(cfg, key)

	if err := config.SaveConfig(configPath, cfg); err != nil {
		return req.Reply("Failed to save config: " + err.Error())
	}

	return req.Reply(fmt.Sprintf("API key for %s (%s) saved to .security.yml", service, entry.Description))
}

func secretListHandler(_ context.Context, req Request, rt *Runtime) error {
	cfg := rt.Config
	if cfg == nil {
		var err error
		cfg, _, err = loadConfigForSecret(rt)
		if err != nil {
			return req.Reply("Failed to load config: " + err.Error())
		}
	}

	var sb strings.Builder
	sb.WriteString("Configured API keys:\n")

	names := SupportedServiceNames()
	for _, name := range names {
		entry := serviceRegistry[name]
		status := "not set"
		if k := entry.GetKey(cfg); k != "" {
			// Show only first 4 and last 4 chars
			if len(k) > 12 {
				status = k[:4] + "..." + k[len(k)-4:]
			} else {
				status = "****"
			}
		}
		sb.WriteString(fmt.Sprintf("  %s (%s): %s\n", name, entry.Description, status))
	}

	return req.Reply(sb.String())
}

func secretRemoveHandler(_ context.Context, req Request, rt *Runtime) error {
	service := nthToken(req.Text, 2)
	if service == "" {
		return req.Reply("Usage: /secret remove <service>\nServices: " + strings.Join(SupportedServiceNames(), ", "))
	}

	entry, ok := serviceRegistry[service]
	if !ok {
		return req.Reply("Unknown service: " + service + "\nSupported: " + strings.Join(SupportedServiceNames(), ", "))
	}

	cfg, configPath, err := loadConfigForSecret(rt)
	if err != nil {
		return req.Reply("Failed to load config: " + err.Error())
	}

	entry.ClearKey(cfg)

	if err := config.SaveConfig(configPath, cfg); err != nil {
		return req.Reply("Failed to save config: " + err.Error())
	}

	return req.Reply(fmt.Sprintf("API key for %s removed.", service))
}

func loadConfigForSecret(rt *Runtime) (*config.Config, string, error) {
	configPath := ""
	if rt != nil {
		configPath = rt.ConfigPath
	}
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, "", err
	}
	return cfg, configPath, nil
}
