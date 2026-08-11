package main

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	sourceScreen screen = iota
	manualPathScreen
	steamInstallationsScreen
	installScreen
	resultScreen
	errorScreen
)

type installationFinishedMsg struct {
	err error
}

type installationProgressMsg struct {
	progress installationProgress
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("51"))
	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("213"))
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("42"))
	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196"))
	pathStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229"))
)

type appModel struct {
	screen         screen
	previousScreen screen
	cursor         int
	pathInput      string
	steamPaths     []string
	selectedPath   string
	errorMessage   string
	installEvents  chan tea.Msg
	progress       installationProgress
	width          int
}

func newAppModel() appModel {
	return appModel{screen: sourceScreen}
}

func (appModel) Init() tea.Cmd {
	return nil
}

func (model appModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		return model, nil
	case installationFinishedMsg:
		model.installEvents = nil
		if message.err != nil {
			model.showError(model.previousScreen, message.err.Error())
			return model, nil
		}
		model.screen = resultScreen
		return model, nil
	case installationProgressMsg:
		model.progress = message.progress
		if model.installEvents == nil {
			return model, nil
		}
		return model, waitForInstallEvent(model.installEvents)
	case tea.KeyMsg:
		if message.Type == tea.KeyCtrlC {
			return model, tea.Quit
		}

		switch model.screen {
		case sourceScreen:
			return model.updateSourceScreen(message)
		case manualPathScreen:
			return model.updateManualPathScreen(message)
		case steamInstallationsScreen:
			return model.updateSteamInstallationsScreen(message)
		case resultScreen:
			if message.Type == tea.KeyEnter || message.Type == tea.KeyEsc || message.String() == "q" {
				return model, tea.Quit
			}
		case errorScreen:
			if message.Type == tea.KeyEnter || message.Type == tea.KeyEsc {
				model.screen = model.previousScreen
				model.errorMessage = ""
				return model, nil
			}
		}
	}
	return model, nil
}

func (model appModel) updateSourceScreen(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyUp:
		if model.cursor > 0 {
			model.cursor--
		}
	case tea.KeyDown:
		if model.cursor < 1 {
			model.cursor++
		}
	case tea.KeyEnter:
		if model.cursor == 0 {
			model.steamPaths = findSteamGameFolders(gameFolderName)
			switch len(model.steamPaths) {
			case 0:
				model.showError(sourceScreen, fmt.Sprintf(
					"%s não foi encontrado nas bibliotecas da Steam.", gameFolderName,
				))
			case 1:
				model.selectedPath = model.steamPaths[0]
				return model.beginInstallation(sourceScreen)
			default:
				model.cursor = 0
				model.screen = steamInstallationsScreen
			}
			return model, nil
		}
		model.pathInput = ""
		model.screen = manualPathScreen
		return model, nil
	case tea.KeyEsc:
		return model, tea.Quit
	}
	return model, nil
}

func (model appModel) updateManualPathScreen(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		model.screen = sourceScreen
		model.cursor = 1
		return model, nil
	case tea.KeyEnter:
		path, err := validateGamePath(model.pathInput)
		if err != nil {
			model.showError(manualPathScreen, err.Error())
			return model, nil
		}
		model.selectedPath = path
		return model.beginInstallation(manualPathScreen)
	case tea.KeyBackspace, tea.KeyDelete:
		runes := []rune(model.pathInput)
		if len(runes) > 0 {
			model.pathInput = string(runes[:len(runes)-1])
		}
		return model, nil
	case tea.KeyCtrlU:
		model.pathInput = ""
		return model, nil
	case tea.KeyRunes:
		model.pathInput += string(key.Runes)
		return model, nil
	}
	return model, nil
}

func (model appModel) updateSteamInstallationsScreen(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyUp:
		if model.cursor > 0 {
			model.cursor--
		}
	case tea.KeyDown:
		if model.cursor < len(model.steamPaths)-1 {
			model.cursor++
		}
	case tea.KeyEnter:
		model.selectedPath = model.steamPaths[model.cursor]
		return model.beginInstallation(steamInstallationsScreen)
	case tea.KeyEsc:
		model.cursor = 0
		model.screen = sourceScreen
	}
	return model, nil
}

func (model appModel) beginInstallation(previous screen) (tea.Model, tea.Cmd) {
	model.previousScreen = previous
	model.screen = installScreen
	model.progress = installationProgress{Stage: stageDownloading}
	model.installEvents = make(chan tea.Msg)
	return model, installFiles(model.selectedPath, model.installEvents)
}

func installFiles(gamePath string, events chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			err := downloadFilesWithProgress(gamePath, func(progress installationProgress) {
				events <- installationProgressMsg{progress: progress}
			})
			events <- installationFinishedMsg{err: err}
			close(events)
		}()
		return <-events
	}
}

func waitForInstallEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

func (model *appModel) showError(previous screen, message string) {
	model.previousScreen = previous
	model.errorMessage = message
	model.screen = errorScreen
}

func (model appModel) View() string {
	var content string
	switch model.screen {
	case sourceScreen:
		content = model.sourceView()
	case manualPathScreen:
		content = model.manualPathView()
	case steamInstallationsScreen:
		content = model.steamInstallationsView()
	case installScreen:
		content = model.installView()
	case resultScreen:
		content = model.resultView()
	case errorScreen:
		content = model.errorView()
	}

	container := lipgloss.NewStyle().Padding(1, 2)
	if model.width > 0 {
		container = container.MaxWidth(model.width)
	}
	return container.Render(content)
}

func (model appModel) sourceView() string {
	options := []string{
		"Procurar automaticamente na Steam",
		"Informar o caminho completo",
	}

	var builder strings.Builder
	builder.WriteString(titleStyle.Render("ASSASSIN'S CREED BLACK FLAG RESYNCED"))
	builder.WriteString("\n")
	builder.WriteString(mutedStyle.Render("Instalador da atualização 1.0.6"))
	builder.WriteString("\n\nOnde o jogo está instalado?\n\n")
	for index, option := range options {
		prefix := "  "
		style := lipgloss.NewStyle()
		if index == model.cursor {
			prefix = "› "
			style = selectedStyle
		}
		builder.WriteString(style.Render(prefix + option))
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	builder.WriteString(mutedStyle.Render("↑/↓ navegar • Enter selecionar • Esc sair"))
	return builder.String()
}

func (model appModel) manualPathView() string {
	input := model.pathInput
	if input == "" {
		input = mutedStyle.Render(`D:\SteamLibrary\steamapps\common\Assassin's Creed Black Flag Resynced`)
	} else {
		input = pathStyle.Render(input)
	}
	return titleStyle.Render("CAMINHO DO JOGO") +
		"\n\nCole ou digite o caminho completo:\n\n" +
		selectedStyle.Render("> ") + input + selectedStyle.Render("█") +
		"\n\n" + mutedStyle.Render("Enter confirmar • Ctrl+U limpar • Esc voltar")
}

func (model appModel) steamInstallationsView() string {
	var builder strings.Builder
	builder.WriteString(titleStyle.Render("INSTALAÇÕES ENCONTRADAS"))
	builder.WriteString("\n\n")
	for index, path := range model.steamPaths {
		prefix := "  "
		style := pathStyle
		if index == model.cursor {
			prefix = "› "
			style = selectedStyle
		}
		builder.WriteString(style.Render(prefix + path))
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	builder.WriteString(mutedStyle.Render("↑/↓ navegar • Enter selecionar • Esc voltar"))
	return builder.String()
}

func (model appModel) resultView() string {
	return successStyle.Render("ATUALIZAÇÃO INSTALADA") +
		"\n\n" + pathStyle.Render(model.selectedPath) +
		"\n\n" + mutedStyle.Render("Todos os lotes foram extraídos e copiados para a pasta do jogo.") +
		"\n\n" + mutedStyle.Render("Enter ou q para sair")
}

func (model appModel) installView() string {
	var status string
	switch model.progress.Stage {
	case stageDownloading:
		status = model.downloadProgressView()
	case stageListing:
		status = "Lendo a lista de arquivos do RAR..."
	case stageExtracting:
		status = fmt.Sprintf(
			"Extraindo lote %d/%d (até %d arquivos por lote)...",
			model.progress.CurrentBatch,
			model.progress.TotalBatches,
			extractionBatchSize,
		)
	case stageCopying:
		status = fmt.Sprintf(
			"Copiando lote %d/%d... %d/%d arquivos",
			model.progress.CurrentBatch,
			model.progress.TotalBatches,
			model.progress.CompletedFiles,
			model.progress.TotalFiles,
		)
	}

	return titleStyle.Render("BAIXANDO E INSTALANDO") +
		"\n\n" + pathStyle.Render(model.selectedPath) +
		"\n\n" + status +
		"\n\n" + mutedStyle.Render("O arquivo tem aproximadamente 3,6 GiB. Ctrl+C para cancelar")
}

func (model appModel) downloadProgressView() string {
	if model.progress.TotalBytes == 0 && model.progress.Downloaded == 0 {
		return mutedStyle.Render("Conectando ao servidor...")
	}
	if model.progress.TotalBytes <= 0 {
		return mutedStyle.Render("Baixado: " + formatBytes(model.progress.Downloaded))
	}

	percentage := int(float64(model.progress.Downloaded) / float64(model.progress.TotalBytes) * 100)
	if percentage > 100 {
		percentage = 100
	}
	if percentage < 0 {
		percentage = 0
	}
	barWidth := 42
	if model.width > 0 && model.width-10 < barWidth {
		barWidth = model.width - 10
	}
	if barWidth < 10 {
		barWidth = 10
	}
	filled := barWidth * percentage / 100
	bar := successStyle.Render(strings.Repeat("█", filled)) +
		mutedStyle.Render(strings.Repeat("░", barWidth-filled))
	return bar + fmt.Sprintf(
		"  %d%%\n%s",
		percentage,
		mutedStyle.Render(formatBytes(model.progress.Downloaded)+" / "+formatBytes(model.progress.TotalBytes)),
	)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	units := []string{"KB", "MB", "GB", "TB"}
	unitIndex := -1
	for value >= unit && unitIndex < len(units)-1 {
		value /= unit
		unitIndex++
	}
	return fmt.Sprintf("%.1f %s", value, units[unitIndex])
}

func (model appModel) errorView() string {
	messageWidth := model.width - 4
	if messageWidth < 20 {
		messageWidth = 80
	}
	return errorStyle.Render("NÃO FOI POSSÍVEL CONTINUAR") +
		"\n\n" + wrapPlainText(model.errorMessage, messageWidth) +
		"\n\n" + mutedStyle.Render("Enter ou Esc para voltar")
}

func wrapPlainText(text string, width int) string {
	if width <= 0 {
		return text
	}
	paragraphs := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		runes := []rune(paragraph)
		for len(runes) > width {
			breakAt := width
			for breakAt > 0 && !unicode.IsSpace(runes[breakAt]) {
				breakAt--
			}
			if breakAt == 0 {
				breakAt = width
			}
			wrapped = append(wrapped, strings.TrimRightFunc(string(runes[:breakAt]), unicode.IsSpace))
			runes = []rune(strings.TrimLeftFunc(string(runes[breakAt:]), unicode.IsSpace))
		}
		wrapped = append(wrapped, string(runes))
	}
	return strings.Join(wrapped, "\n")
}
