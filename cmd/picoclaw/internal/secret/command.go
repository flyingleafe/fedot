package secret

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
	"github.com/sipeed/picoclaw/pkg/commands"
	"github.com/sipeed/picoclaw/pkg/config"
)

func NewSecretCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manage API keys in .security.yml (deterministic, never sent to AI)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newSetCommand(),
		newListCommand(),
		newRemoveCommand(),
	)

	return cmd
}

func newSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <service> <key>",
		Short: "Save an API key for a service",
		Long:  "Save an API key to .security.yml.\nSupported services: " + strings.Join(commands.SupportedServiceNames(), ", "),
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return secretSetCmd(args[0], args[1])
		},
	}
}

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show configured API keys and their status",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return secretListCmd()
		},
	}
}

func newRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <service>",
		Short: "Remove an API key for a service",
		Long:  "Remove an API key from .security.yml.\nSupported services: " + strings.Join(commands.SupportedServiceNames(), ", "),
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return secretRemoveCmd(args[0])
		},
	}
}

func secretSetCmd(service, key string) error {
	cfg, configPath, err := loadConfig()
	if err != nil {
		return err
	}

	entry, ok := commands.GetServiceEntry(service)
	if !ok {
		return fmt.Errorf("unknown service: %s\nsupported: %s", service, strings.Join(commands.SupportedServiceNames(), ", "))
	}

	entry.SetKey(cfg, key)

	if err := config.SaveConfig(configPath, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("API key for %s (%s) saved to .security.yml\n", service, entry.Description)
	return nil
}

func secretListCmd() error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}

	fmt.Println("Configured API keys:")
	for _, name := range commands.SupportedServiceNames() {
		entry, _ := commands.GetServiceEntry(name)
		status := "not set"
		if k := entry.GetKey(cfg); k != "" {
			if len(k) > 12 {
				status = k[:4] + "..." + k[len(k)-4:]
			} else {
				status = "****"
			}
		}
		fmt.Printf("  %-14s %-30s %s\n", name, entry.Description, status)
	}
	return nil
}

func secretRemoveCmd(service string) error {
	cfg, configPath, err := loadConfig()
	if err != nil {
		return err
	}

	entry, ok := commands.GetServiceEntry(service)
	if !ok {
		return fmt.Errorf("unknown service: %s\nsupported: %s", service, strings.Join(commands.SupportedServiceNames(), ", "))
	}

	entry.ClearKey(cfg)

	if err := config.SaveConfig(configPath, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("API key for %s removed.\n", service)
	return nil
}

func loadConfig() (*config.Config, string, error) {
	configPath := internal.GetConfigPath()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load config: %w", err)
	}
	return cfg, configPath, nil
}
