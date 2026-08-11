package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const gameFolderName = "Assassin's Creed Black Flag Resynced"

var steamLibraryPathPattern = regexp.MustCompile(`(?m)"path"\s*"([^"]+)"`)

func findSteamGameFolders(gameName string) []string {
	return findSteamGameFoldersInRoots(gameName, steamInstallRoots())
}

func findSteamGameFoldersInRoots(gameName string, installRoots []string) []string {
	libraryRoots := append([]string(nil), installRoots...)
	for _, installRoot := range installRoots {
		libraryRoots = append(libraryRoots, readSteamLibraries(installRoot)...)
	}

	found := make([]string, 0, 1)
	for _, libraryRoot := range uniquePaths(libraryRoots) {
		candidate := filepath.Join(libraryRoot, "steamapps", "common", gameName)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			found = append(found, filepath.Clean(candidate))
		}
	}

	found = uniquePaths(found)
	sort.Strings(found)
	return found
}

func steamInstallRoots() []string {
	candidates := make([]string, 0, 5)
	for _, environmentName := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if base := strings.TrimSpace(os.Getenv(environmentName)); base != "" {
			candidates = append(candidates, filepath.Join(base, "Steam"))
		}
	}

	registryValues := []struct {
		key  string
		name string
	}{
		{key: `HKCU\Software\Valve\Steam`, name: "SteamPath"},
		{key: `HKLM\SOFTWARE\WOW6432Node\Valve\Steam`, name: "InstallPath"},
		{key: `HKLM\SOFTWARE\Valve\Steam`, name: "InstallPath"},
	}
	for _, value := range registryValues {
		if path := readRegistryString(value.key, value.name); path != "" {
			candidates = append(candidates, path)
		}
	}

	return uniquePaths(candidates)
}

func readRegistryString(key string, name string) string {
	output, err := exec.Command("reg.exe", "query", key, "/v", name).Output()
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		for index, field := range fields {
			if strings.HasPrefix(field, "REG_") && index+1 < len(fields) {
				return strings.Join(fields[index+1:], " ")
			}
		}
	}
	return ""
}

func readSteamLibraries(installRoot string) []string {
	data, err := os.ReadFile(filepath.Join(installRoot, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return nil
	}

	matches := steamLibraryPathPattern.FindAllStringSubmatch(string(data), -1)
	libraries := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 2 {
			libraries = append(libraries, strings.ReplaceAll(match[1], `\\`, `\`))
		}
	}
	return libraries
}

func validateGamePath(value string) (string, error) {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	value = os.ExpandEnv(value)
	if value == "" {
		return "", fmt.Errorf("informe o caminho completo da pasta %s", gameFolderName)
	}

	absolutePath, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("caminho inválido %q: %w", value, err)
	}
	absolutePath = filepath.Clean(absolutePath)

	info, err := os.Stat(absolutePath)
	if err != nil {
		return "", fmt.Errorf("não foi possível acessar %q: %w", absolutePath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q não é uma pasta", absolutePath)
	}
	if !strings.EqualFold(filepath.Base(absolutePath), gameFolderName) {
		return "", fmt.Errorf("a pasta selecionada deve se chamar %q", gameFolderName)
	}

	return absolutePath, nil
}

func uniquePaths(paths []string) []string {
	unique := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		key := strings.ToLower(path)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, path)
	}
	return unique
}
