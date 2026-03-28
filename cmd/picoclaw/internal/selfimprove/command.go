package selfimprove

import (
	"github.com/spf13/cobra"
)

func NewSelfImproveCommand() *cobra.Command {
	var (
		noRestart bool
		noBuild   bool
	)

	cmd := &cobra.Command{
		Use:     "selfimprove [prompt]",
		Aliases: []string{"si"},
		Short:   "Run Claude Code to improve the picoclaw codebase",
		Long: `Run Claude Code against the picoclaw source with a given prompt.
Builds, installs, and optionally restarts the service after changes.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			prompt := joinArgs(args)
			return selfImproveCmd(prompt, noBuild, noRestart)
		},
	}

	cmd.Flags().BoolVar(&noRestart, "no-restart", false, "Skip service restart after install")
	cmd.Flags().BoolVar(&noBuild, "no-build", false, "Skip build and install (just run Claude)")

	return cmd
}

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}
