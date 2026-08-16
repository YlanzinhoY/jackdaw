package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const applicationTextsFileName = "texts.json"

var applicationTexts map[string]string

func appText(key string, fallback string) string {
	if value, exists := applicationTexts[key]; exists {
		return value
	}
	return fallback
}

func appTextf(key string, fallback string, arguments ...any) string {
	return fmt.Sprintf(appText(key, fallback), arguments...)
}

func loadApplicationTexts() error {
	for _, candidate := range applicationTextCandidates() {
		data, err := os.ReadFile(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("could not read text catalog %q: %w", candidate, err)
		}
		catalog := make(map[string]string)
		if err := json.Unmarshal(data, &catalog); err != nil {
			return fmt.Errorf("invalid text catalog %q: %w", candidate, err)
		}
		applicationTexts = catalog
		return nil
	}
	applicationTexts = nil
	return nil
}

func applicationTextCandidates() []string {
	candidates := make([]string, 0, 4)
	if override := strings.TrimSpace(os.Getenv("ACBFR_TEXTS_FILE")); override != "" {
		candidates = append(candidates, override)
	}
	if executablePath, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executablePath)
		candidates = append(candidates,
			filepath.Join(executableDir, applicationTextsFileName),
			filepath.Join(filepath.Dir(executableDir), applicationTextsFileName),
		)
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(workingDirectory, applicationTextsFileName))
	}
	return uniquePaths(candidates)
}
