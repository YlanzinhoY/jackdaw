package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func main() {
	disableExplorerMousetrap()
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		os.Exit(1)
	}
}

func disableExplorerMousetrap() {
	cobra.MousetrapHelpText = ""
}

func newRootCommand() *cobra.Command {
	command := &cobra.Command{
		Use:           "acbfr",
		Short:         "Instala a atualização do Assassin's Creed Black Flag Resynced",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, _ []string) error {
			program := tea.NewProgram(newAppModel(), tea.WithAltScreen())
			finalModel, err := program.Run()
			if err != nil {
				return fmt.Errorf("não foi possível abrir a interface: %w", err)
			}

			model, ok := finalModel.(appModel)
			if ok && model.screen == resultScreen && model.selectedPath != "" {
				fmt.Fprintf(command.OutOrStdout(), "Atualização instalada em: %s\n", model.selectedPath)
			}
			return nil
		},
	}
	command.CompletionOptions.DisableDefaultCmd = true
	return command
}
