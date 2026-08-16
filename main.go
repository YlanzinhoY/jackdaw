package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func main() {
	disableExplorerMousetrap()
	if err := loadApplicationTexts(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func disableExplorerMousetrap() {
	cobra.MousetrapHelpText = ""
}

func newRootCommand() *cobra.Command {
	command := &cobra.Command{
		Use:           "jackdaw",
		Short:         "Installs the Assassin's Creed Black Flag Resynced update",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, _ []string) error {
			return runApplication(command, newAppModel())
		},
	}
	command.CompletionOptions.DisableDefaultCmd = true
	command.AddCommand(newVoicesCommand())
	return command
}

func runInstaller(command *cobra.Command, config installerConfig) error {
	if err := config.validate(); err != nil {
		return err
	}
	return runApplication(command, newAppModelWithConfig(config))
}

func runApplication(command *cobra.Command, initialModel appModel) error {
	program := tea.NewProgram(initialModel, tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return fmt.Errorf("could not open the interface: %w", err)
	}

	model, ok := finalModel.(appModel)
	if !ok {
		return nil
	}
	if model.screen == resultScreen && model.selectedPath != "" {
		fmt.Fprintf(command.OutOrStdout(), "%s installed in: %s\n", model.config.outputName, model.selectedPath)
	}
	if model.screen == saveResultScreen && model.saveResult.FileCount > 0 {
		fmt.Fprintf(
			command.OutOrStdout(),
			"%d save(s) resigned. Backup in: %s\n",
			model.saveResult.FileCount,
			model.saveResult.BackupDir,
		)
	}
	return nil
}
