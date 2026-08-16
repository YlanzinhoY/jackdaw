package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Paste the voice-pack URL between the quotes below.
// Alternatively, set ACBFR_VOICES_URL before running the program.
const voicesDownloadLink = "https://ts.buzzheavier.com/d/ccixxmdz06v1?v=_dkzrGq_0OvcWuoJClObI_sWtbulKlPwAtZ6Kfm33bQaQPx4K7mbH0YqcKqRFBQ5sBd8SGnlKqAkCZCKQCD88iKltVTp8LVIAVV0yoizRt8LJBXGZxIOvJf38zE95JhpZFPhzhBwhTemYqarjfGHMz1iBtCa6S1dxvGBQR-2B0C_1uPnS7aJfBcLljtux8te2EA6J6d-sKYD0aWCyc4LhdVNBl6wOzCCnOXiHdBZRI01Rn-DfV8KTDrPrZ7adXt56mJ3AnhBYeoSdcsWPP_N5mTPfZcSNFlmvl8-yJvaV1Fc"

func newVoicesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "voices",
		Short: "Downloads and installs the voice pack",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runInstaller(command, voicesInstallerConfig())
		},
	}
}

func voicesInstallerConfig() installerConfig {
	return installerConfig{
		downloadURL:   configuredVoicesDownloadLink(),
		packageName:   appText("voices.package_name", "voice pack"),
		outputName:    appText("voices.output_name", "Voice pack"),
		subtitle:      appText("voices.subtitle", "Voice-pack installer"),
		resultTitle:   appText("voices.result_title", "VOICE PACK INSTALLED"),
		resultText:    appText("voices.result_text", "The voice files were installed and unwanted components were removed."),
		downloadHint:  appText("voices.download_hint", "The size will be detected from the server. Ctrl+C to cancel"),
		cleanupVoices: true,
	}
}

func configuredVoicesDownloadLink() string {
	if override := strings.TrimSpace(os.Getenv("ACBFR_VOICES_URL")); override != "" {
		return override
	}
	return strings.TrimSpace(voicesDownloadLink)
}

func cleanupVoicesArtifacts(gamePath string) error {
	gameRoot, err := filepath.Abs(gamePath)
	if err != nil {
		return fmt.Errorf("could not resolve the game folder: %w", err)
	}
	gameRoot = filepath.Clean(gameRoot)
	info, err := os.Stat(gameRoot)
	if err != nil {
		return fmt.Errorf("could not access the game folder: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("the game path is not a folder: %q", gameRoot)
	}

	targets := make([]string, 0)
	err = filepath.WalkDir(gameRoot, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if currentPath == gameRoot {
			return nil
		}

		name := strings.ToLower(entry.Name())
		remove := strings.Contains(name, "denuvowo")
		if entry.IsDir() {
			remove = remove || name == "driver_amd" || name == "driver_intel"
		} else {
			remove = remove || name == "reflex.dll" || name == "reflex.ini" || name == "vbs.cmd"
		}
		if !remove {
			return nil
		}

		if err := ensurePathInside(gameRoot, currentPath); err != nil {
			return err
		}
		targets = append(targets, currentPath)
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("could not search for unwanted files: %w", err)
	}

	for _, target := range targets {
		if err := removeAllWritable(target); err != nil {
			return fmt.Errorf("could not remove %q: %w", target, err)
		}
	}
	return nil
}

func removeAllWritable(target string) error {
	err := filepath.WalkDir(target, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o200 == 0 {
			return os.Chmod(currentPath, info.Mode().Perm()|0o200)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return os.RemoveAll(target)
}

func ensurePathInside(root string, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid cleanup path: %q", target)
	}
	return nil
}
