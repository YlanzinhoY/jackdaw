package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const settingsSearchMaxDepth = 3

type settingsUpdatePlan struct {
	path    string
	data    []byte
	changed bool
}

func discoverUbisoftAccountUUID() (string, string, error) {
	roots := make([]string, 0, 3)
	if override := strings.TrimSpace(os.Getenv("ACBFR_SAVEGAMES_ROOT")); override != "" {
		roots = append(roots, override)
	}
	if programFilesX86 := strings.TrimSpace(os.Getenv("ProgramFiles(x86)")); programFilesX86 != "" {
		roots = append(roots, filepath.Join(programFilesX86, "Ubisoft", "Ubisoft Game Launcher", "savegames"))
	}
	roots = append(roots, `C:\Program Files (x86)\Ubisoft\Ubisoft Game Launcher\savegames`)

	var errors []string
	for _, root := range uniquePaths(roots) {
		uuid, accountFolder, err := discoverUUIDInSavegamesRoot(root)
		if err == nil {
			return uuid, accountFolder, nil
		}
		errors = append(errors, err.Error())
	}
	return "", "", fmt.Errorf("automatic UUID detection failed: %s", strings.Join(errors, "; "))
}

func discoverUUIDInSavegamesRoot(root string) (string, string, error) {
	root, err := filepath.Abs(strings.Trim(strings.TrimSpace(root), `"`))
	if err != nil {
		return "", "", fmt.Errorf("invalid savegames path: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", "", fmt.Errorf("could not read %q: %w", root, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		uuid, valid := uuidFromFolderName(entry.Name())
		if valid {
			return uuid, filepath.Join(root, entry.Name()), nil
		}
	}
	return "", "", fmt.Errorf("no account folder containing a UUID was found in %q", root)
}

func uuidFromFolderName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if isValidUUID(name) {
		return name, true
	}
	if len(name) != 32 {
		return "", false
	}
	for _, character := range name {
		if !isASCIIHex(character) {
			return "", false
		}
	}
	uuid := formatUUIDInput(name)
	return uuid, isValidUUID(uuid)
}

func locateBlackFlagGameFolder() (string, error) {
	if override := strings.TrimSpace(os.Getenv("ACBFR_GAME_PATH")); override != "" {
		return validateGamePath(override)
	}
	paths := findSteamGameFolders(gameFolderName)
	if len(paths) == 0 {
		return "", fmt.Errorf(
			"the %q folder was not found automatically; set ACBFR_GAME_PATH to the game path",
			gameFolderName,
		)
	}
	return paths[0], nil
}

func prepareBlackFlagUserIDUpdate(targetUUID string) (settingsUpdatePlan, error) {
	var plan settingsUpdatePlan
	gamePath, err := locateBlackFlagGameFolder()
	if err != nil {
		return plan, err
	}
	settingsPath, err := findBlackFlagSettingsFile(gamePath)
	if err != nil {
		return plan, err
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return plan, fmt.Errorf("could not read settings file %q: %w", settingsPath, err)
	}
	updated, changed, err := replaceSettingsUserID(data, targetUUID)
	if err != nil {
		return plan, fmt.Errorf("could not update %q: %w", settingsPath, err)
	}
	return settingsUpdatePlan{path: settingsPath, data: updated, changed: changed}, nil
}

func applySettingsUpdate(plan settingsUpdatePlan) error {
	if !plan.changed {
		return nil
	}
	backupPath := plan.path + ".acbfr.bak"
	if err := createSaveBackup(plan.path, backupPath); err != nil {
		return fmt.Errorf("could not back up the settings file: %w", err)
	}
	if err := makeDestinationWritable(plan.path); err != nil {
		return fmt.Errorf("could not prepare the settings file: %w", err)
	}
	if err := os.WriteFile(plan.path, plan.data, 0o600); err != nil {
		return fmt.Errorf("could not write the settings file: %w", err)
	}
	return nil
}

func findBlackFlagSettingsFile(gamePath string) (string, error) {
	gamePath, err := filepath.Abs(gamePath)
	if err != nil {
		return "", fmt.Errorf("invalid game path: %w", err)
	}
	gamePath = filepath.Clean(gamePath)
	candidates := make([]string, 0, 1)
	err = filepath.WalkDir(gamePath, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if currentPath == gamePath {
			return nil
		}
		relative, err := filepath.Rel(gamePath, currentPath)
		if err != nil {
			return err
		}
		depth := strings.Count(filepath.Clean(relative), string(filepath.Separator)) + 1
		if entry.IsDir() {
			if entry.Type()&os.ModeSymlink != 0 || depth > settingsSearchMaxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if depth > settingsSearchMaxDepth || !entry.Type().IsRegular() || !isSettingsFileExtension(currentPath) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > 1<<20 {
			return nil
		}
		data, err := os.ReadFile(currentPath)
		if err == nil && looksLikeBlackFlagSettings(data) {
			candidates = append(candidates, currentPath)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("could not search for the settings file inside %q: %w", gamePath, err)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf(
			"no settings file containing [Settings], UserId, and SaveExtension = .save was found in %q",
			gamePath,
		)
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftRelative, _ := filepath.Rel(gamePath, candidates[left])
		rightRelative, _ := filepath.Rel(gamePath, candidates[right])
		leftDepth := strings.Count(leftRelative, string(filepath.Separator))
		rightDepth := strings.Count(rightRelative, string(filepath.Separator))
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return strings.ToLower(leftRelative) < strings.ToLower(rightRelative)
	})
	return candidates[0], nil
}

func isSettingsFileExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ini", ".cfg", ".conf", ".txt":
		return true
	default:
		return false
	}
}

func looksLikeBlackFlagSettings(data []byte) bool {
	section := ""
	hasSettings := false
	hasUserID := false
	hasSaveExtension := false
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			if strings.EqualFold(section, "Settings") {
				hasSettings = true
			}
			continue
		}
		if !strings.EqualFold(section, "Settings") {
			continue
		}
		key, value, found := strings.Cut(trimmed, "=")
		if !found {
			continue
		}
		switch {
		case strings.EqualFold(strings.TrimSpace(key), "UserId"):
			hasUserID = true
		case strings.EqualFold(strings.TrimSpace(key), "SaveExtension") && strings.EqualFold(strings.TrimSpace(value), ".save"):
			hasSaveExtension = true
		}
	}
	return hasSettings && hasUserID && hasSaveExtension
}

func replaceSettingsUserID(data []byte, targetUUID string) ([]byte, bool, error) {
	if !isValidUUID(targetUUID) {
		return nil, false, fmt.Errorf("invalid target UUID")
	}
	lines := strings.Split(string(data), "\n")
	section := ""
	found := 0
	changed := false
	for index, line := range lines {
		hasCarriageReturn := strings.HasSuffix(line, "\r")
		content := strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimSpace(strings.TrimPrefix(content, "\ufeff"))
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			continue
		}
		if !strings.EqualFold(section, "Settings") {
			continue
		}
		equals := strings.IndexByte(content, '=')
		if equals < 0 || !strings.EqualFold(strings.TrimSpace(content[:equals]), "UserId") {
			continue
		}
		found++
		if found > 1 {
			return nil, false, fmt.Errorf("more than one UserId key was found in [Settings]")
		}
		value := content[equals+1:]
		leadingWhitespaceLength := len(value) - len(strings.TrimLeft(value, " \t"))
		currentUUID := strings.TrimSpace(value)
		if !strings.EqualFold(currentUUID, targetUUID) {
			content = content[:equals+1] + value[:leadingWhitespaceLength] + targetUUID
			changed = true
		}
		if hasCarriageReturn {
			content += "\r"
		}
		lines[index] = content
	}
	if found == 0 {
		return nil, false, fmt.Errorf("the UserId key was not found inside [Settings]")
	}
	return []byte(strings.Join(lines, "\n")), changed, nil
}
