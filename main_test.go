package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func TestExplorerMousetrapIsDisabled(t *testing.T) {
	previousHelpText := cobra.MousetrapHelpText
	defer func() { cobra.MousetrapHelpText = previousHelpText }()
	cobra.MousetrapHelpText = "command-line warning"
	disableExplorerMousetrap()
	if cobra.MousetrapHelpText != "" {
		t.Fatalf("MousetrapHelpText = %q, want empty", cobra.MousetrapHelpText)
	}
}

func TestApplicationTextsCanBeLoadedFromEditableJSON(t *testing.T) {
	previousTexts := applicationTexts
	defer func() { applicationTexts = previousTexts }()
	textPath := filepath.Join(t.TempDir(), "custom-texts.json")
	if err := os.WriteFile(textPath, []byte(`{"menu.subtitle":"Custom menu","watermark":"Made by YlanzinhoY"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACBFR_TEXTS_FILE", textPath)
	if err := loadApplicationTexts(); err != nil {
		t.Fatal(err)
	}
	if got := appText("menu.subtitle", "fallback"); got != "Custom menu" {
		t.Fatalf("custom text = %q", got)
	}
	if got := appText("missing", "fallback"); got != "fallback" {
		t.Fatalf("fallback text = %q", got)
	}
}

func TestFindSteamGameFolderInAdditionalLibrary(t *testing.T) {
	installRoot := t.TempDir()
	libraryRoot := filepath.Join(t.TempDir(), "Steam Library")
	gamePath := filepath.Join(libraryRoot, "steamapps", "common", gameFolderName)
	if err := os.MkdirAll(gamePath, 0o755); err != nil {
		t.Fatal(err)
	}
	steamApps := filepath.Join(installRoot, "steamapps")
	if err := os.MkdirAll(steamApps, 0o755); err != nil {
		t.Fatal(err)
	}
	escapedLibraryPath := strings.ReplaceAll(libraryRoot, `\`, `\\`)
	vdf := `"libraryfolders"` + "\n{\n" +
		`  "1" { "path" "` + escapedLibraryPath + `" }` + "\n}\n"
	if err := os.WriteFile(filepath.Join(steamApps, "libraryfolders.vdf"), []byte(vdf), 0o600); err != nil {
		t.Fatal(err)
	}

	found := findSteamGameFoldersInRoots(gameFolderName, []string{installRoot})
	if len(found) != 1 || found[0] != filepath.Clean(gamePath) {
		t.Fatalf("found = %v, want [%q]", found, gamePath)
	}
}

func TestValidateGamePathAcceptsExpectedFolder(t *testing.T) {
	gamePath := filepath.Join(t.TempDir(), gameFolderName)
	if err := os.Mkdir(gamePath, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := validateGamePath("\x00\x00\x00\x00\x00\x00\"" + gamePath + "\"\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != gamePath {
		t.Fatalf("validateGamePath() = %q, want %q", got, gamePath)
	}
}

func TestSanitizePathInputRemovesInvisiblePasteCharacters(t *testing.T) {
	want := `C:\Users\enzom\Saves`
	input := "\x00\x00\ufeff" + want + "\r\n"
	if got := sanitizePathInput(input); got != want {
		t.Fatalf("sanitizePathInput() = %q, want %q", got, want)
	}
}

func TestArchiveDestinationRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../outside.dll", `..\outside.dll`, "/outside.dll", `C:\outside.dll`} {
		if _, err := archiveDestination(root, name); err == nil {
			t.Errorf("archiveDestination(%q) accepted an unsafe path", name)
		}
	}
}

func TestSplitIntoBatchesUsesTwentyFiles(t *testing.T) {
	files := make([]string, 109)
	for index := range files {
		files[index] = fmt.Sprintf("file-%03d.bin", index)
	}
	batches := splitIntoBatches(files, extractionBatchSize)
	wantSizes := []int{20, 20, 20, 20, 20, 9}
	if len(batches) != len(wantSizes) {
		t.Fatalf("batch count = %d, want %d", len(batches), len(wantSizes))
	}
	for index, want := range wantSizes {
		if len(batches[index]) != want {
			t.Fatalf("batch %d size = %d, want %d", index+1, len(batches[index]), want)
		}
	}
}

func TestConfiguredDownloadLinkCanBeOverridden(t *testing.T) {
	override := "https://example.test/update.rar"
	t.Setenv("ACBFR_DOWNLOAD_URL", "  "+override+"  ")
	if got := configuredDownloadLink(); got != override {
		t.Fatalf("configuredDownloadLink() = %q, want %q", got, override)
	}
}

func TestRootCommandIncludesVoices(t *testing.T) {
	command, _, err := newRootCommand().Find([]string{"voices"})
	if err != nil {
		t.Fatal(err)
	}
	if command.Name() != "voices" {
		t.Fatalf("command name = %q, want voices", command.Name())
	}
}

func TestMainScreenOffersUpdateVoicesAndSaves(t *testing.T) {
	model := newAppModel()
	view := model.View()
	if model.screen != packageScreen {
		t.Fatalf("initial screen = %v, want package screen", model.screen)
	}
	for _, text := range []string{
		"Install update 1.0.6 — Hypervisor",
		"Install Voices38 pack — HV to Voices",
		"Resign save files",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("main screen does not contain %q: %q", text, view)
		}
	}
	if !strings.Contains(view, "Made by YlanzinhoY") {
		t.Fatalf("main screen does not contain watermark: %q", view)
	}
}

func TestSelectingSaveResignerFromMainScreen(t *testing.T) {
	savegamesRoot := t.TempDir()
	appData := t.TempDir()
	uuid := "d6e2f1b1-3543-4ee3-ae3f-dbe3c633c5fa"
	accountPath := filepath.Join(savegamesRoot, uuid)
	if err := os.Mkdir(accountPath, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACBFR_SAVEGAMES_ROOT", savegamesRoot)
	t.Setenv("APPDATA", appData)
	recommendedFolder := filepath.Join(appData, "Goldberg UplayEmu Saves", "66088")
	if err := os.MkdirAll(recommendedFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	model := newAppModel()
	for range 2 {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(appModel)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(appModel)

	if model.screen != saveFolderScreen {
		t.Fatalf("screen after selecting save resigner = %v, want save folder screen", model.screen)
	}
	if !strings.Contains(model.View(), "SAVE FOLDER") {
		t.Fatalf("save folder screen was not rendered: %q", model.View())
	}
	if model.saveUUID != uuid || model.saveAccountPath != accountPath {
		t.Fatalf("automatic account detection = (%q, %q)", model.saveUUID, model.saveAccountPath)
	}
	if model.saveFolderInput != recommendedFolder {
		t.Fatalf("recommended save folder = %q, want %q", model.saveFolderInput, recommendedFolder)
	}
}

func TestSelectingVoicesFromMainScreen(t *testing.T) {
	t.Setenv("ACBFR_VOICES_URL", "https://example.test/voices.rar")
	model := newAppModel()

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(appModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(appModel)

	if model.screen != sourceScreen {
		t.Fatalf("screen after selecting voices = %v, want source screen", model.screen)
	}
	if model.config.packageName != "voice pack" {
		t.Fatalf("selected package = %q, want voice pack", model.config.packageName)
	}
	if !strings.Contains(model.View(), "Voice-pack installer") {
		t.Fatalf("voices source screen was not rendered: %q", model.View())
	}
}

func TestConfiguredVoicesDownloadLinkCanBeOverridden(t *testing.T) {
	override := "https://example.test/voices.rar"
	t.Setenv("ACBFR_VOICES_URL", "  "+override+"  ")
	if got := configuredVoicesDownloadLink(); got != override {
		t.Fatalf("configuredVoicesDownloadLink() = %q, want %q", got, override)
	}

	config := voicesInstallerConfig()
	if config.downloadURL != override {
		t.Fatalf("voices download URL = %q, want %q", config.downloadURL, override)
	}
	if config.expectedSize != 0 {
		t.Fatalf("voices expected size = %d, want automatic size detection", config.expectedSize)
	}
	if !config.cleanupVoices {
		t.Fatal("voices cleanup is disabled")
	}
	if err := config.validate(); err != nil {
		t.Fatalf("voices config is invalid: %v", err)
	}
}

func TestCleanupVoicesArtifacts(t *testing.T) {
	gamePath := filepath.Join(t.TempDir(), gameFolderName)
	outsidePath := filepath.Join(filepath.Dir(gamePath), "outside-denuvOwO.txt")

	remove := []string{
		"driver_amd/driver.dll",
		"mods/DRIVER_INTEL/driver.dll",
		"reflex.dll",
		"config/reflex.ini",
		"scripts/vbs.cmd",
		"denuvOwO.log",
		"cache/prefix-denuvOwO-backup.bin",
		"mods/denuvOwO-data/content.bin",
	}
	keep := []string{
		"game.exe",
		"config/settings.ini",
		"mods/voices/audio.pck",
	}
	for _, relative := range append(remove, keep...) {
		filePath := filepath.Join(gamePath, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(relative), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(outsidePath, []byte("keep outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(gamePath, "reflex.dll"), 0o400); err != nil {
		t.Fatal(err)
	}

	if err := cleanupVoicesArtifacts(gamePath); err != nil {
		t.Fatal(err)
	}
	for _, relative := range remove {
		if _, err := os.Stat(filepath.Join(gamePath, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Errorf("artifact %q still exists (error: %v)", relative, err)
		}
	}
	for _, relative := range keep {
		if _, err := os.Stat(filepath.Join(gamePath, filepath.FromSlash(relative))); err != nil {
			t.Errorf("wanted file %q was affected: %v", relative, err)
		}
	}
	if _, err := os.Stat(outsidePath); err != nil {
		t.Errorf("file outside game folder was affected: %v", err)
	}
}

func TestInstallerConfigRejectsMissingAndInvalidURLs(t *testing.T) {
	config := voicesInstallerConfig()
	config.downloadURL = ""
	if err := config.validate(); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing URL error = %v", err)
	}

	config.downloadURL = "local-file.rar"
	if err := config.validate(); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalid URL error = %v", err)
	}
}

func TestListArchiveFilesIgnoresDirectories(t *testing.T) {
	tarPath := requireTar(t)
	archivePath := filepath.Join("testdata", "batch.rar")
	files, err := listArchiveFiles(context.Background(), tarPath, archivePath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Fatalf("file count = %d, want 5: %v", len(files), files)
	}
	for _, name := range files {
		if name == "config" || name == "data" {
			t.Fatalf("directory was returned as a file: %q", name)
		}
	}
}

func TestEndToEndDownloadsAndInstallsThreeBatches(t *testing.T) {
	requireTar(t)
	archive, err := os.ReadFile(filepath.Join("testdata", "batch.rar"))
	if err != nil {
		t.Fatal(err)
	}
	acceptHeader := ""
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		acceptHeader = request.Header.Get("Accept")
		response.Header().Set("Content-Type", "application/octet-stream")
		response.Header().Set("Content-Length", fmt.Sprintf("%d", len(archive)))
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write(archive)
	}))
	defer server.Close()

	gamePath := filepath.Join(t.TempDir(), gameFolderName)
	if err := os.Mkdir(gamePath, 0o755); err != nil {
		t.Fatal(err)
	}
	rootFile := filepath.Join(gamePath, "root.txt")
	if err := os.WriteFile(rootFile, []byte("old root content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(rootFile, 0o400); err != nil {
		t.Fatal(err)
	}

	var extractionBatches []int
	lastCopied := 0
	err = downloadAndInstallArchive(
		context.Background(),
		server.Client(),
		server.URL,
		gamePath,
		int64(len(archive)),
		2,
		func(progress installationProgress) {
			if progress.Stage == stageExtracting {
				extractionBatches = append(extractionBatches, progress.CurrentBatch)
			}
			if progress.Stage == stageCopying {
				lastCopied = progress.CompletedFiles
			}
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if acceptHeader != "application/octet-stream" {
		t.Fatalf("Accept = %q, want application/octet-stream", acceptHeader)
	}
	if fmt.Sprint(extractionBatches) != "[1 2 3]" {
		t.Fatalf("extraction batches = %v, want [1 2 3]", extractionBatches)
	}
	if lastCopied != 5 {
		t.Fatalf("last copied count = %d, want 5", lastCopied)
	}

	wantContents := map[string]string{
		"root.txt":            "new root content\n",
		"config/settings.ini": "setting=true\n",
		"data/one.bin":        "one\n",
		"data/two.bin":        "two\n",
		"data/space name.txt": "space\n",
	}
	for relative, want := range wantContents {
		content, err := os.ReadFile(filepath.Join(gamePath, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		if string(content) != want {
			t.Fatalf("%s content = %q, want %q", relative, content, want)
		}
	}
	entries, err := os.ReadDir(gamePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".acbfr-work-") {
			t.Fatalf("temporary work directory was not removed: %q", entry.Name())
		}
	}
}

func TestDownloadRejectsUnexpectedArchiveSize(t *testing.T) {
	archive := []byte("not the expected archive")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Length", fmt.Sprintf("%d", len(archive)))
		_, _ = response.Write(archive)
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "update.rar")
	err := downloadArchive(
		context.Background(),
		server.Client(),
		server.URL,
		destination,
		int64(len(archive)+1),
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "unexpected size") {
		t.Fatalf("error = %v, want unexpected size error", err)
	}
}

func TestBatchProgressIsRendered(t *testing.T) {
	model := newAppModel()
	model.screen = installScreen
	model.selectedPath = `D:\SteamLibrary\steamapps\common\Assassin's Creed Black Flag Resynced`
	model.progress = installationProgress{
		Stage:          stageCopying,
		CompletedFiles: 40,
		TotalFiles:     109,
		CurrentBatch:   2,
		TotalBatches:   6,
	}
	view := model.View()
	if !strings.Contains(view, "batch 2/6") || !strings.Contains(view, "40/109 files") {
		t.Fatalf("batch progress was not rendered: %q", view)
	}
}

func TestInstallationProgressMessageUpdatesModel(t *testing.T) {
	model := newAppModel()
	model.screen = installScreen
	model.installEvents = make(chan tea.Msg)
	progress := installationProgress{Stage: stageListing}
	updated, command := model.Update(installationProgressMsg{progress: progress})
	model = updated.(appModel)
	if model.progress != progress {
		t.Fatalf("progress = %#v, want %#v", model.progress, progress)
	}
	if command == nil {
		t.Fatal("expected command waiting for the next progress event")
	}
	close(model.installEvents)
}

func TestErrorViewWrapsLongMessages(t *testing.T) {
	model := newAppModel()
	model.screen = errorScreen
	model.width = 40
	model.errorMessage = "could not copy D:\\SteamLibrary\\steamapps\\common\\Assassin's Creed Black Flag Resynced\\very-long-file-name.bin"
	view := model.View()
	if !strings.Contains(view, "\n") || !strings.Contains(view, "very-long") {
		t.Fatalf("error message was not wrapped: %q", view)
	}
}

func requireTar(t *testing.T) string {
	t.Helper()
	tarPath, err := exec.LookPath("tar.exe")
	if err != nil {
		t.Skip("tar.exe is required for the Windows RAR integration test")
	}
	return tarPath
}
