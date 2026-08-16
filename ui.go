package main

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	packageScreen screen = iota
	sourceScreen
	manualPathScreen
	steamInstallationsScreen
	installScreen
	resultScreen
	saveFolderScreen
	saveUUIDScreen
	saveProcessingScreen
	saveResultScreen
	errorScreen
)

type installationFinishedMsg struct {
	err error
}

type installationProgressMsg struct {
	progress installationProgress
}

type saveResignFinishedMsg struct {
	result saveResignResult
	err    error
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
	config          installerConfig
	showPackageMenu bool
	screen          screen
	previousScreen  screen
	cursor          int
	packageCursor   int
	pathInput       string
	steamPaths      []string
	selectedPath    string
	errorMessage    string
	installEvents   chan tea.Msg
	progress        installationProgress
	saveFolderInput string
	saveFolder      string
	saveFiles       []string
	saveUUID        string
	saveAccountPath string
	saveResult      saveResignResult
	width           int
}

func newAppModel() appModel {
	return appModel{
		config:          updateInstallerConfig(),
		showPackageMenu: true,
		screen:          packageScreen,
	}
}

func newAppModelWithConfig(config installerConfig) appModel {
	return appModel{config: config, screen: sourceScreen}
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
	case saveResignFinishedMsg:
		if message.err != nil {
			model.showError(saveUUIDScreen, message.err.Error())
			return model, nil
		}
		model.saveResult = message.result
		model.screen = saveResultScreen
		return model, nil
	case tea.KeyMsg:
		if message.Type == tea.KeyCtrlC {
			return model, tea.Quit
		}

		switch model.screen {
		case packageScreen:
			return model.updatePackageScreen(message)
		case sourceScreen:
			return model.updateSourceScreen(message)
		case manualPathScreen:
			return model.updateManualPathScreen(message)
		case steamInstallationsScreen:
			return model.updateSteamInstallationsScreen(message)
		case saveFolderScreen:
			return model.updateSaveFolderScreen(message)
		case saveUUIDScreen:
			return model.updateSaveUUIDScreen(message)
		case resultScreen:
			if message.Type == tea.KeyEnter || message.Type == tea.KeyEsc || message.String() == "q" {
				return model, tea.Quit
			}
		case saveResultScreen:
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

func (model appModel) updatePackageScreen(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyUp:
		if model.packageCursor > 0 {
			model.packageCursor--
		}
	case tea.KeyDown:
		if model.packageCursor < 2 {
			model.packageCursor++
		}
	case tea.KeyEnter:
		switch model.packageCursor {
		case 0:
			model.config = updateInstallerConfig()
		case 1:
			model.config = voicesInstallerConfig()
		case 2:
			model.saveFolderInput = ""
			model.saveFolder = ""
			model.saveFiles = nil
			model.saveUUID = ""
			model.saveAccountPath = ""
			model.saveResult = saveResignResult{}
			if uuid, accountPath, err := discoverUbisoftAccountUUID(); err == nil {
				model.saveUUID = uuid
				model.saveAccountPath = accountPath
			}
			if recommended := recommendedVoicesSaveFolder(); recommended != "" {
				if info, err := os.Stat(recommended); err == nil && info.IsDir() {
					model.saveFolderInput = recommended
				}
			}
			model.screen = saveFolderScreen
			return model, nil
		}
		if err := model.config.validate(); err != nil {
			model.showError(packageScreen, err.Error())
			return model, nil
		}
		model.cursor = 0
		model.screen = sourceScreen
	case tea.KeyEsc:
		return model, tea.Quit
	}
	return model, nil
}

func (model appModel) updateSaveFolderScreen(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		model.screen = packageScreen
		return model, nil
	case tea.KeyEnter:
		folder, files, err := validateSaveFolder(model.saveFolderInput)
		if err != nil {
			model.showError(saveFolderScreen, err.Error())
			return model, nil
		}
		model.saveFolder = folder
		model.saveFiles = files
		model.screen = saveUUIDScreen
		return model, nil
	case tea.KeyBackspace, tea.KeyDelete:
		runes := []rune(model.saveFolderInput)
		if len(runes) > 0 {
			model.saveFolderInput = string(runes[:len(runes)-1])
		}
		return model, nil
	case tea.KeyCtrlU:
		model.saveFolderInput = ""
		return model, nil
	case tea.KeyRunes:
		model.saveFolderInput = sanitizePathInput(model.saveFolderInput + string(key.Runes))
		return model, nil
	}
	return model, nil
}

func (model appModel) updateSaveUUIDScreen(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		model.screen = saveFolderScreen
		return model, nil
	case tea.KeyEnter:
		if !isValidUUID(model.saveUUID) {
			model.showError(saveUUIDScreen, appText("save_uuid.invalid", "enter a valid UUID containing 32 hexadecimal characters"))
			return model, nil
		}
		model.screen = saveProcessingScreen
		return model, resignSaves(model.saveFolder, model.saveUUID)
	case tea.KeyBackspace, tea.KeyDelete:
		runes := []rune(model.saveUUID)
		if len(runes) > 0 {
			model.saveUUID = formatUUIDInput(string(runes[:len(runes)-1]))
		}
		return model, nil
	case tea.KeyCtrlU:
		model.saveUUID = ""
		return model, nil
	case tea.KeyRunes:
		model.saveUUID = formatUUIDInput(model.saveUUID + string(key.Runes))
		return model, nil
	}
	return model, nil
}

func resignSaves(folder string, targetUUID string) tea.Cmd {
	return func() tea.Msg {
		result, err := resignSavesAndSynchronizeSettings(folder, targetUUID)
		return saveResignFinishedMsg{result: result, err: err}
	}
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
				model.showError(sourceScreen, appTextf(
					"location.not_found",
					"%s was not found in your Steam libraries.",
					gameFolderName,
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
		if model.showPackageMenu {
			model.screen = packageScreen
			return model, nil
		}
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
		model.pathInput = sanitizePathInput(model.pathInput + string(key.Runes))
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
	return model, installFiles(model.config, model.selectedPath, model.installEvents)
}

func installFiles(config installerConfig, gamePath string, events chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			err := downloadPackageWithProgress(config, gamePath, func(progress installationProgress) {
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
	case packageScreen:
		content = model.packageView()
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
	case saveFolderScreen:
		content = model.saveFolderView()
	case saveUUIDScreen:
		content = model.saveUUIDView()
	case saveProcessingScreen:
		content = model.saveProcessingView()
	case saveResultScreen:
		content = model.saveResultView()
	case errorScreen:
		content = model.errorView()
	}

	container := lipgloss.NewStyle().Padding(1, 2)
	if model.width > 0 {
		container = container.MaxWidth(model.width)
	}
	if watermark := appText("watermark", "Made by YlanzinhoY"); watermark != "" {
		content += "\n\n" + mutedStyle.Render(watermark)
	}
	return container.Render(content)
}

func (model appModel) packageView() string {
	options := []string{
		appText("menu.update", "Install update 1.0.6 — Hypervisor"),
		appText("menu.voices", "Install Voices38 pack — HV to Voices"),
		appText("menu.resign", "Resign save files"),
	}

	var builder strings.Builder
	builder.WriteString(titleStyle.Render(appText("brand.title", "ASSASSIN'S CREED BLACK FLAG RESYNCED")))
	builder.WriteString("\n")
	builder.WriteString(mutedStyle.Render(appText("menu.subtitle", "Choose an action")))
	builder.WriteString("\n\n")
	for index, option := range options {
		prefix := "  "
		style := lipgloss.NewStyle()
		if index == model.packageCursor {
			prefix = "› "
			style = selectedStyle
		}
		builder.WriteString(style.Render(prefix + option))
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	builder.WriteString(mutedStyle.Render(appText("controls.menu", "↑/↓ navigate • Enter select • Esc quit")))
	return builder.String()
}

func (model appModel) saveFolderView() string {
	input := model.saveFolderInput
	if input == "" {
		input = mutedStyle.Render(appText(
			"save_folder.placeholder",
			`C:\Users\<WindowsUser>\AppData\Roaming\Goldberg UplayEmu Saves\66088`,
		))
	} else {
		input = pathStyle.Render(input)
	}
	uuidStatus := mutedStyle.Render(appText(
		"save_folder.uuid_missing",
		"Account UUID was not detected automatically; you can enter it on the next screen.",
	))
	if model.saveAccountPath != "" {
		uuidStatus = successStyle.Render(appTextf(
			"save_folder.uuid_found",
			"UUID detected automatically: %s",
			model.saveUUID,
		)) +
			"\n" + mutedStyle.Render(model.saveAccountPath)
	}
	return titleStyle.Render(appText("save_folder.title", "SAVE FOLDER")) +
		"\n\n" + appText("save_folder.prompt", "Paste the path to the folder containing the .save files:") +
		"\n\n" + selectedStyle.Render("> ") + input + selectedStyle.Render("█") +
		"\n\n" + mutedStyle.Render(appText("save_folder.detect_hint", "ACBlackFlag[...].save files are detected automatically.")) +
		"\n\n" + uuidStatus +
		"\n\n" + mutedStyle.Render(appText("save_folder.controls", "Enter continue • Ctrl+U clear • Esc back"))
}

func (model appModel) saveUUIDView() string {
	input := model.saveUUID
	if input == "" {
		input = mutedStyle.Render(appText("save_uuid.placeholder", "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"))
	} else {
		input = pathStyle.Render(input)
	}
	uuidExplanation := appText("save_uuid.prompt", "Confirm or enter the UUID of the account that will use these saves:")
	if model.saveAccountPath != "" {
		uuidExplanation = appText("save_uuid.detected_prompt", "Confirm the detected UUID or edit it if necessary:")
	}
	return titleStyle.Render(appText("save_uuid.title", "NEW SAVE KEY")) +
		"\n\n" + appTextf("save_uuid.files_found", "%d .save file(s) found in:", len(model.saveFiles)) + "\n" +
		pathStyle.Render(model.saveFolder) +
		"\n\n" + uuidExplanation +
		"\n\n" + selectedStyle.Render("> ") + input + selectedStyle.Render("█") +
		"\n\n" + mutedStyle.Render(appText("save_uuid.hint", "The previous key is detected automatically from the save data.")) +
		"\n\n" + mutedStyle.Render(appText("save_uuid.controls", "Enter resign • Ctrl+U clear • Esc back"))
}

func (model appModel) saveProcessingView() string {
	return titleStyle.Render(appText("save_processing.title", "RESIGNING SAVE FILES")) +
		"\n\n" + pathStyle.Render(model.saveFolder) +
		"\n\n" + appText("save_processing.status", "Creating backups and resigning files...") +
		"\n\n" + mutedStyle.Render(appText("save_processing.controls", "Do not close the program. Ctrl+C to cancel"))
}

func (model appModel) saveResultView() string {
	return successStyle.Render(appText("save_result.title", "SAVE FILES RESIGNED")) +
		"\n\n" + appTextf("save_result.count", "%d file(s) resigned.", model.saveResult.FileCount) +
		"\n\n" + appText("save_result.backup", "Original-file backup:") + "\n" + pathStyle.Render(model.saveResult.BackupDir) +
		"\n\n" + appText("save_result.settings", "UserId synchronized in:") + "\n" + pathStyle.Render(model.saveResult.SettingsPath) +
		"\n\n" + mutedStyle.Render(appText("controls.quit", "Enter or q to quit"))
}

func (model appModel) sourceView() string {
	options := []string{
		appText("location.auto", "Find automatically through Steam"),
		appText("location.manual", "Enter the full path"),
	}

	var builder strings.Builder
	builder.WriteString(titleStyle.Render(appText("brand.title", "ASSASSIN'S CREED BLACK FLAG RESYNCED")))
	builder.WriteString("\n")
	builder.WriteString(mutedStyle.Render(model.config.subtitle))
	builder.WriteString("\n\n" + appText("location.prompt", "Where is the game installed?") + "\n\n")
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
	builder.WriteString(mutedStyle.Render(appText("controls.menu", "↑/↓ navigate • Enter select • Esc quit")))
	return builder.String()
}

func (model appModel) manualPathView() string {
	input := model.pathInput
	if input == "" {
		input = mutedStyle.Render(appText(
			"manual_path.placeholder",
			`D:\SteamLibrary\steamapps\common\Assassin's Creed Black Flag Resynced`,
		))
	} else {
		input = pathStyle.Render(input)
	}
	return titleStyle.Render(appText("manual_path.title", "GAME FOLDER")) +
		"\n\n" + appText("manual_path.prompt", "Paste or enter the full path:") + "\n\n" +
		selectedStyle.Render("> ") + input + selectedStyle.Render("█") +
		"\n\n" + mutedStyle.Render(appText("manual_path.controls", "Enter confirm • Ctrl+U clear • Esc back"))
}

func (model appModel) steamInstallationsView() string {
	var builder strings.Builder
	builder.WriteString(titleStyle.Render(appText("steam.title", "INSTALLATIONS FOUND")))
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
	builder.WriteString(mutedStyle.Render(appText("steam.controls", "↑/↓ navigate • Enter select • Esc back")))
	return builder.String()
}

func (model appModel) resultView() string {
	return successStyle.Render(model.config.resultTitle) +
		"\n\n" + pathStyle.Render(model.selectedPath) +
		"\n\n" + mutedStyle.Render(model.config.resultText) +
		"\n\n" + mutedStyle.Render(appText("controls.quit", "Enter or q to quit"))
}

func (model appModel) installView() string {
	var status string
	switch model.progress.Stage {
	case stageDownloading:
		status = model.downloadProgressView()
	case stageListing:
		status = appText("install.listing", "Reading the RAR file list...")
	case stageExtracting:
		status = appTextf(
			"install.extracting",
			"Extracting batch %d/%d (up to %d files per batch)...",
			model.progress.CurrentBatch,
			model.progress.TotalBatches,
			extractionBatchSize,
		)
	case stageCopying:
		status = appTextf(
			"install.copying",
			"Copying batch %d/%d... %d/%d files",
			model.progress.CurrentBatch,
			model.progress.TotalBatches,
			model.progress.CompletedFiles,
			model.progress.TotalFiles,
		)
	case stageCleaning:
		status = appText("install.cleaning", "Removing unwanted voice-pack files...")
	}

	return titleStyle.Render(appText("install.title", "DOWNLOADING AND INSTALLING")) +
		"\n\n" + pathStyle.Render(model.selectedPath) +
		"\n\n" + status +
		"\n\n" + mutedStyle.Render(model.config.downloadHint)
}

func (model appModel) downloadProgressView() string {
	if model.progress.TotalBytes == 0 && model.progress.Downloaded == 0 {
		return mutedStyle.Render(appText("download.connecting", "Connecting to the server..."))
	}
	if model.progress.TotalBytes <= 0 {
		return mutedStyle.Render(appTextf("download.downloaded", "Downloaded: %s", formatBytes(model.progress.Downloaded)))
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
	return errorStyle.Render(appText("error.title", "UNABLE TO CONTINUE")) +
		"\n\n" + wrapPlainText(model.errorMessage, messageWidth) +
		"\n\n" + mutedStyle.Render(appText("error.controls", "Enter or Esc to go back"))
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
