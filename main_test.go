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
	got, err := validateGamePath(`"` + gamePath + `"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != gamePath {
		t.Fatalf("validateGamePath() = %q, want %q", got, gamePath)
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
	if err == nil || !strings.Contains(err.Error(), "tamanho inesperado") {
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
	if !strings.Contains(view, "lote 2/6") || !strings.Contains(view, "40/109 arquivos") {
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
	model.errorMessage = "não foi possível copiar D:\\SteamLibrary\\steamapps\\common\\Assassin's Creed Black Flag Resynced\\very-long-file-name.bin"
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
