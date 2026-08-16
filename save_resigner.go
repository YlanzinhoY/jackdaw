package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const saveHeaderSize = 40

var (
	saveMagic      = [4]byte{0xAC, 0xDB, 0xFE, 0x00}
	knownSaveBlock = [16]byte{
		0xAC, 0xDB, 0xFE, 0x00, 0x36, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	knownSaveTail = [12]byte{
		0x33, 0xAA, 0xFB, 0x57, 0x99, 0xFA,
		0x04, 0x10, 0x03, 0x00, 0x05, 0x00,
	}
)

type saveResignResult struct {
	FileCount    int
	BackupDir    string
	SettingsPath string
}

type preparedSave struct {
	path        string
	data        []byte
	previousMD5 string
}

func recommendedVoicesSaveFolder() string {
	appData := strings.TrimSpace(os.Getenv("APPDATA"))
	if appData == "" {
		return ""
	}
	return filepath.Join(appData, "Goldberg UplayEmu Saves", "66088")
}

func formatUUIDInput(raw string) string {
	hexCharacters := make([]rune, 0, 32)
	for _, character := range raw {
		if isASCIIHex(character) {
			hexCharacters = append(hexCharacters, character)
			if len(hexCharacters) == 32 {
				break
			}
		}
	}

	var formatted strings.Builder
	formatted.Grow(36)
	for index, character := range hexCharacters {
		if index == 8 || index == 12 || index == 16 || index == 20 {
			formatted.WriteByte('-')
		}
		formatted.WriteRune(character)
	}
	return formatted.String()
}

func isASCIIHex(character rune) bool {
	return character >= '0' && character <= '9' ||
		character >= 'a' && character <= 'f' ||
		character >= 'A' && character <= 'F'
}

func isValidUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
		default:
			if !isASCIIHex(character) {
				return false
			}
		}
	}
	return true
}

func buildSaveKey(accountUUID string, payloadSize int) ([16]byte, [16]byte) {
	digest := md5.Sum([]byte(strings.ToLower(accountUUID)))
	var pre [16]byte
	pre[0] = digest[0]
	pre[1] = digest[15]
	for index := 0; index < 14; index++ {
		pre[index+2] = digest[14-index]
	}

	rotation := payloadSize % len(pre)
	var post [16]byte
	if rotation == 0 {
		post = pre
	} else {
		copy(post[:rotation], pre[len(pre)-rotation:])
		copy(post[rotation:], pre[:len(pre)-rotation])
	}
	return pre, post
}

func xorSavePayload(data []byte, key [16]byte) []byte {
	result := make([]byte, len(data))
	for index, value := range data {
		result[index] = value ^ key[index%len(key)]
	}
	return result
}

func recoverSaveKey(payload []byte) ([16]byte, [16]byte, bool) {
	var pre, post [16]byte
	if len(payload) < 48 {
		return pre, post, false
	}

	for index := range post {
		post[index] = payload[index] ^ knownSaveBlock[index]
	}
	post[8] = payload[40] ^ 0x99
	post[12] = payload[44] ^ 0x03

	decrypted := xorSavePayload(payload[:48], post)
	if !bytes.Equal(decrypted[:4], saveMagic[:]) || !bytes.Equal(decrypted[36:48], knownSaveTail[:]) {
		return pre, post, false
	}

	rotation := len(payload) % len(pre)
	if rotation == 0 {
		pre = post
	} else {
		copy(pre[:len(pre)-rotation], post[rotation:])
		copy(pre[len(pre)-rotation:], post[:rotation])
	}
	return pre, post, true
}

func isBlackFlagSave(path string, data []byte) bool {
	if len(data) < saveHeaderSize+48 || !strings.EqualFold(filepath.Ext(path), ".save") {
		return false
	}
	name := strings.ToLower(filepath.Base(path))
	if strings.Contains(name, "acblackflag") || strings.Contains(name, "acbf") {
		return true
	}
	_, _, detected := recoverSaveKey(data[saveHeaderSize:])
	return detected
}

func validateSaveFolder(value string) (string, []string, error) {
	value = sanitizePathInput(value)
	value = strings.Trim(strings.TrimSpace(value), `"`)
	value = os.ExpandEnv(value)
	if value == "" {
		return "", nil, fmt.Errorf("enter the folder containing the .save files")
	}

	folder, err := filepath.Abs(value)
	if err != nil {
		return "", nil, fmt.Errorf("invalid path %q: %w", value, err)
	}
	folder = filepath.Clean(folder)
	info, err := os.Stat(folder)
	if err != nil {
		return "", nil, fmt.Errorf("could not access %q: %w", folder, err)
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("%q is not a folder", folder)
	}

	files, err := collectSaveFiles(folder)
	if err != nil {
		return "", nil, err
	}
	return folder, files, nil
}

func collectSaveFiles(folder string) ([]string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("could not read the save folder: %w", err)
	}

	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !entry.Type().IsRegular() {
			continue
		}
		filePath := filepath.Join(folder, entry.Name())
		data, err := os.ReadFile(filePath)
		if err == nil && isBlackFlagSave(filePath, data) {
			files = append(files, filePath)
		}
	}
	sort.Slice(files, func(left, right int) bool {
		return strings.ToLower(files[left]) < strings.ToLower(files[right])
	})
	if len(files) == 0 {
		return nil, fmt.Errorf("no valid Assassin's Creed Black Flag save files were found in this folder")
	}
	return files, nil
}

func resignSaveFolder(folder string, targetUUID string) (saveResignResult, error) {
	var result saveResignResult
	if !isValidUUID(targetUUID) {
		return result, fmt.Errorf("the account UUID is invalid")
	}

	folder, files, err := validateSaveFolder(folder)
	if err != nil {
		return result, err
	}
	backupDir := filepath.Join(filepath.Dir(folder), "Backup")
	prepared := make([]preparedSave, 0, len(files))
	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return result, fmt.Errorf("could not read %q: %w", filePath, err)
		}
		resigned, previousMD5, err := resignSaveData(data, targetUUID)
		if err != nil {
			return result, fmt.Errorf("%s: %w", filepath.Base(filePath), err)
		}
		prepared = append(prepared, preparedSave{
			path:        filePath,
			data:        resigned,
			previousMD5: previousMD5,
		})
	}

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return result, fmt.Errorf("could not create the backup folder: %w", err)
	}
	for _, save := range prepared {
		backupPath := filepath.Join(backupDir, filepath.Base(save.path))
		if err := createSaveBackup(save.path, backupPath); err != nil {
			return result, err
		}
	}
	for _, save := range prepared {
		if err := makeDestinationWritable(save.path); err != nil {
			return result, fmt.Errorf("could not prepare %q: %w", save.path, err)
		}
		if err := os.WriteFile(save.path, save.data, 0o600); err != nil {
			return result, fmt.Errorf("could not update %q: %w", save.path, err)
		}
	}

	var info strings.Builder
	fmt.Fprintf(&info, "Backup of the original Assassin's Creed Black Flag Resynced save files.\n\n")
	fmt.Fprintf(&info, "Source folder: %s\n", folder)
	fmt.Fprintf(&info, "Target UUID: %s\n\n", targetUUID)
	info.WriteString("Resigned files:\n")
	for _, save := range prepared {
		fmt.Fprintf(&info, " - %s: Previous MD5 = %s\n", filepath.Base(save.path), save.previousMD5)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "info.txt"), []byte(info.String()), 0o600); err != nil {
		return result, fmt.Errorf("the saves were resigned, but info.txt could not be written: %w", err)
	}

	result.FileCount = len(prepared)
	result.BackupDir = backupDir
	return result, nil
}

func resignSavesAndSynchronizeSettings(folder string, targetUUID string) (saveResignResult, error) {
	plan, err := prepareBlackFlagUserIDUpdate(targetUUID)
	if err != nil {
		return saveResignResult{}, err
	}
	result, err := resignSaveFolder(folder, targetUUID)
	if err != nil {
		return result, err
	}
	if err := applySettingsUpdate(plan); err != nil {
		return result, fmt.Errorf("the saves were resigned, but UserId synchronization failed: %w", err)
	}
	result.SettingsPath = plan.path
	return result, nil
}

func resignSaveData(data []byte, targetUUID string) ([]byte, string, error) {
	if len(data) < saveHeaderSize+48 {
		return nil, "", fmt.Errorf("the file is too small to be a valid save")
	}

	header := append([]byte(nil), data[:saveHeaderSize]...)
	payload := data[saveHeaderSize:]
	pre, sourceKey, detected := recoverSaveKey(payload)
	if !detected {
		return nil, "", fmt.Errorf("the original key could not be detected automatically")
	}
	decrypted := xorSavePayload(payload, sourceKey)
	if !bytes.Equal(decrypted[:4], saveMagic[:]) {
		return nil, "", fmt.Errorf("decryption failed; the save may be corrupted")
	}

	var previousMD5 [16]byte
	previousMD5[0] = pre[0]
	previousMD5[15] = pre[1]
	for index := 1; index < 15; index++ {
		previousMD5[index] = pre[16-index]
	}
	_, targetKey := buildSaveKey(targetUUID, len(payload))
	resignedPayload := xorSavePayload(decrypted, targetKey)
	for index := 8; index < saveHeaderSize; index++ {
		header[index] = 0
	}

	output := make([]byte, 0, len(data))
	output = append(output, header...)
	output = append(output, resignedPayload...)
	return output, hex.EncodeToString(previousMD5[:]), nil
}

func createSaveBackup(sourcePath string, backupPath string) error {
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not access backup %q: %w", backupPath, err)
	}

	input, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("could not open %q for backup: %w", sourcePath, err)
	}
	defer input.Close()
	output, err := os.OpenFile(backupPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("could not create backup %q: %w", backupPath, err)
	}
	completed := false
	defer func() {
		if !completed {
			_ = os.Remove(backupPath)
		}
	}()
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("could not copy backup %q: %w", backupPath, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("could not finalize backup %q: %w", backupPath, closeErr)
	}
	completed = true
	return nil
}
