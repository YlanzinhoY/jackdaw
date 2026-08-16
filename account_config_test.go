package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testBlackFlagSettings = `[Settings]
Username = voices38
Email = voices38@proton.me
UserId = 11111111-2222-3333-4444-555555555555

Language = en-US
SaveType = 0
SavePath =
SaveExtension = .save

[DLC]
5829

[Items]
66077

[Chunks]
0
`

func TestDiscoverUUIDInFirstValidSavegamesFolder(t *testing.T) {
	root := t.TempDir()
	firstUUID := "00112233-4455-6677-8899-aabbccddeeff"
	for _, name := range []string{
		"not-an-account",
		firstUUID,
		"ffeeddcc-bbaa-9988-7766-554433221100",
	} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	uuid, accountPath, err := discoverUUIDInSavegamesRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if uuid != firstUUID {
		t.Fatalf("discovered UUID = %q, want %q", uuid, firstUUID)
	}
	if accountPath != filepath.Join(root, firstUUID) {
		t.Fatalf("account path = %q", accountPath)
	}
}

func TestFindAndUpdateBlackFlagSettingsUserID(t *testing.T) {
	const targetUUID = "d6e2f1b1-3543-4ee3-ae3f-dbe3c633c5fa"
	gamePath := filepath.Join(t.TempDir(), gameFolderName)
	settingsPath := filepath.Join(gamePath, "config", "settings.ini")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte(strings.ReplaceAll(testBlackFlagSettings, "\n", "\r\n"))
	if err := os.WriteFile(settingsPath, original, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gamePath, "unrelated.ini"), []byte("[Settings]\nUserId = keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	found, err := findBlackFlagSettingsFile(gamePath)
	if err != nil {
		t.Fatal(err)
	}
	if found != settingsPath {
		t.Fatalf("settings path = %q, want %q", found, settingsPath)
	}
	t.Setenv("ACBFR_GAME_PATH", gamePath)
	plan, err := prepareBlackFlagUserIDUpdate(targetUUID)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.changed {
		t.Fatal("settings update was not marked as changed")
	}
	if err := applySettingsUpdate(plan); err != nil {
		t.Fatal(err)
	}

	updated, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "UserId = "+targetUUID) {
		t.Fatalf("UserId was not updated: %q", updated)
	}
	for _, unchanged := range []string{
		"Username = voices38",
		"Email = voices38@proton.me",
		"SaveExtension = .save",
		"\r\n[DLC]\r\n5829",
	} {
		if !strings.Contains(string(updated), unchanged) {
			t.Errorf("settings content %q was not preserved", unchanged)
		}
	}
	backup, err := os.ReadFile(settingsPath + ".acbfr.bak")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatal("settings backup differs from the original")
	}
}

func TestSaveWorkflowSynchronizesSettingsWithTargetUUID(t *testing.T) {
	const sourceUUID = "00112233-4455-6677-8899-aabbccddeeff"
	const targetUUID = "d6e2f1b1-3543-4ee3-ae3f-dbe3c633c5fa"
	root := t.TempDir()
	gamePath := filepath.Join(root, gameFolderName)
	if err := os.Mkdir(gamePath, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(gamePath, "settings.ini")
	if err := os.WriteFile(settingsPath, []byte(testBlackFlagSettings), 0o600); err != nil {
		t.Fatal(err)
	}
	saveFolder := filepath.Join(root, "account", "save-slot")
	if err := os.MkdirAll(saveFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	savePath := filepath.Join(saveFolder, "ACBlackFlag[ManualSave02].save")
	if err := os.WriteFile(savePath, encryptedSaveData(sourceUUID, validPlainSavePayload(83)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACBFR_GAME_PATH", gamePath)

	result, err := resignSavesAndSynchronizeSettings(saveFolder, targetUUID)
	if err != nil {
		t.Fatal(err)
	}
	if result.SettingsPath != settingsPath || result.FileCount != 1 {
		t.Fatalf("result = %#v", result)
	}
	settings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(settings), "UserId = "+targetUUID) {
		t.Fatalf("settings UserId was not synchronized: %q", settings)
	}
}
