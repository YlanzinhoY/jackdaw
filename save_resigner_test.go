package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatAndValidateUUID(t *testing.T) {
	raw := "{00112233445566778899AABBCCDDEEFF}extra"
	want := "00112233-4455-6677-8899-AABBCCDDEEFF"
	if got := formatUUIDInput(raw); got != want {
		t.Fatalf("formatUUIDInput() = %q, want %q", got, want)
	}
	if !isValidUUID(want) {
		t.Fatalf("isValidUUID(%q) = false", want)
	}
	for _, invalid := range []string{"", "00112233-4455-6677-8899", "not-a-uuid-0000-0000-000000000000"} {
		if isValidUUID(invalid) {
			t.Errorf("isValidUUID(%q) = true", invalid)
		}
	}
}

func TestResignSaveFolderDetectsOldKeyAndCreatesBackup(t *testing.T) {
	const sourceUUID = "00112233-4455-6677-8899-aabbccddeeff"
	const targetUUID = "ffeeddcc-bbaa-9988-7766-554433221100"

	root := t.TempDir()
	saveFolder := filepath.Join(root, "save-slot")
	if err := os.Mkdir(saveFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	plainPayload := validPlainSavePayload(83)
	originals := make(map[string][]byte)
	for _, name := range []string{
		"ACBlackFlag[ManualSave02].save",
		"ACBF[AutoSave01].SAVE",
	} {
		data := encryptedSaveData(sourceUUID, plainPayload)
		filePath := filepath.Join(saveFolder, name)
		if err := os.WriteFile(filePath, data, 0o600); err != nil {
			t.Fatal(err)
		}
		originals[name] = data
	}
	if err := os.WriteFile(filepath.Join(saveFolder, "ignore.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	validatedFolder, files, err := validateSaveFolder("\x00\x00\x00\x00\x00\x00" + saveFolder + "\x00")
	if err != nil {
		t.Fatal(err)
	}
	if validatedFolder != saveFolder || len(files) != len(originals) {
		t.Fatalf("validated pasted path = (%q, %d files)", validatedFolder, len(files))
	}

	result, err := resignSaveFolder(saveFolder, targetUUID)
	if err != nil {
		t.Fatal(err)
	}
	if result.FileCount != len(originals) {
		t.Fatalf("converted count = %d, want %d", result.FileCount, len(originals))
	}
	if result.BackupDir != filepath.Join(root, "Backup") {
		t.Fatalf("backup dir = %q", result.BackupDir)
	}

	for name, original := range originals {
		backup, err := os.ReadFile(filepath.Join(result.BackupDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(backup, original) {
			t.Fatalf("backup %q differs from original", name)
		}

		converted, err := os.ReadFile(filepath.Join(saveFolder, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(converted[:8], original[:8]) {
			t.Fatalf("first eight header bytes changed in %q", name)
		}
		if !bytes.Equal(converted[8:saveHeaderSize], make([]byte, saveHeaderSize-8)) {
			t.Fatalf("header bytes 8..39 were not cleared in %q", name)
		}
		_, targetKey := buildSaveKey(targetUUID, len(plainPayload))
		decrypted := xorSavePayload(converted[saveHeaderSize:], targetKey)
		if !bytes.Equal(decrypted, plainPayload) {
			t.Fatalf("converted payload %q does not decrypt with target UUID", name)
		}
	}

	info, err := os.ReadFile(filepath.Join(result.BackupDir, "info.txt"))
	if err != nil {
		t.Fatal(err)
	}
	sourceMD5 := md5.Sum([]byte(sourceUUID))
	if !strings.Contains(string(info), targetUUID) || !strings.Contains(string(info), hex.EncodeToString(sourceMD5[:])) {
		t.Fatalf("backup info is incomplete: %q", info)
	}
}

func TestValidateSaveFolderRejectsFolderWithoutRecognizedSaves(t *testing.T) {
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "unrelated.save"), []byte("not a save"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := validateSaveFolder(folder); err == nil || !strings.Contains(err.Error(), "no valid") {
		t.Fatalf("error = %v, want no valid save error", err)
	}
}

func validPlainSavePayload(size int) []byte {
	if size < 48 {
		size = 48
	}
	payload := make([]byte, size)
	for index := range payload {
		payload[index] = byte(index*17 + 3)
	}
	copy(payload[:16], knownSaveBlock[:])
	copy(payload[36:48], knownSaveTail[:])
	return payload
}

func encryptedSaveData(accountUUID string, plainPayload []byte) []byte {
	header := bytes.Repeat([]byte{0xA5}, saveHeaderSize)
	_, key := buildSaveKey(accountUUID, len(plainPayload))
	return append(header, xorSavePayload(plainPayload, key)...)
}
