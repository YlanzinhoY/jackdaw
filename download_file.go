package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultDownloadLink = "https://0807.st/D7zQ6xo.rar"

	expectedArchiveSize = int64(3_853_243_886)
	extractionBatchSize = 20
	downloadBufferSize  = 1 << 20
)

type installationStage int

const (
	stageDownloading installationStage = iota
	stageListing
	stageExtracting
	stageCopying
	stageCleaning
)

type installationProgress struct {
	Stage          installationStage
	Downloaded     int64
	TotalBytes     int64
	CompletedFiles int
	TotalFiles     int
	CurrentBatch   int
	TotalBatches   int
}

type progressReporter func(installationProgress)

type installerConfig struct {
	downloadURL   string
	expectedSize  int64
	packageName   string
	outputName    string
	subtitle      string
	resultTitle   string
	resultText    string
	downloadHint  string
	cleanupVoices bool
}

func updateInstallerConfig() installerConfig {
	return installerConfig{
		downloadURL:  configuredDownloadLink(),
		expectedSize: expectedArchiveSize,
		packageName:  appText("update.package_name", "update"),
		outputName:   appText("update.output_name", "Update"),
		subtitle:     appText("update.subtitle", "Update 1.0.6 installer"),
		resultTitle:  appText("update.result_title", "UPDATE INSTALLED"),
		resultText:   appText("update.result_text", "All batches were extracted and copied into the game folder."),
		downloadHint: appText("update.download_hint", "The file is approximately 3.6 GiB. Ctrl+C to cancel"),
	}
}

func (config installerConfig) validate() error {
	if strings.TrimSpace(config.downloadURL) == "" {
		return fmt.Errorf(
			"the %s URL is not configured; paste it into voicesDownloadLink in voices.go or set ACBFR_VOICES_URL",
			config.packageName,
		)
	}
	parsedURL, err := url.ParseRequestURI(config.downloadURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return fmt.Errorf("the %s URL is invalid", config.packageName)
	}
	if config.expectedSize < 0 {
		return fmt.Errorf("the expected %s size cannot be negative", config.packageName)
	}
	return nil
}

func downloadFilesWithProgress(gamePath string, report progressReporter) error {
	return downloadPackageWithProgress(updateInstallerConfig(), gamePath, report)
}

func downloadPackageWithProgress(config installerConfig, gamePath string, report progressReporter) error {
	if err := config.validate(); err != nil {
		return err
	}
	if err := downloadAndInstallArchive(
		context.Background(),
		http.DefaultClient,
		config.downloadURL,
		gamePath,
		config.expectedSize,
		extractionBatchSize,
		report,
	); err != nil {
		return err
	}
	if config.cleanupVoices {
		reportProgress(report, installationProgress{Stage: stageCleaning})
		if err := cleanupVoicesArtifacts(gamePath); err != nil {
			return fmt.Errorf("failed to remove unwanted voice-pack files: %w", err)
		}
	}
	return nil
}

func configuredDownloadLink() string {
	if override := strings.TrimSpace(os.Getenv("ACBFR_DOWNLOAD_URL")); override != "" {
		return override
	}
	return defaultDownloadLink
}

func downloadAndInstallArchive(
	ctx context.Context,
	client *http.Client,
	sourceURL string,
	gamePath string,
	expectedSize int64,
	batchSize int,
	report progressReporter,
) error {
	if batchSize <= 0 {
		return fmt.Errorf("the batch size must be greater than zero")
	}
	info, err := os.Stat(gamePath)
	if err != nil {
		return fmt.Errorf("could not access the game folder: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("the game path is not a folder: %q", gamePath)
	}

	workRoot, err := os.MkdirTemp(gamePath, ".acbfr-work-*")
	if err != nil {
		return fmt.Errorf("could not create the temporary workspace in the game folder: %w", err)
	}
	defer os.RemoveAll(workRoot)

	archivePath := filepath.Join(workRoot, "update.rar")
	if err := downloadArchive(ctx, client, sourceURL, archivePath, expectedSize, report); err != nil {
		return fmt.Errorf("failed to download the package: %w", err)
	}

	tarPath, err := exec.LookPath("tar.exe")
	if err != nil {
		return fmt.Errorf("the native Windows extractor (tar.exe) was not found: %w", err)
	}
	if err := installArchiveInBatches(
		ctx,
		tarPath,
		archivePath,
		gamePath,
		workRoot,
		batchSize,
		report,
	); err != nil {
		return fmt.Errorf("failed to install the package: %w", err)
	}
	return nil
}

func downloadArchive(
	ctx context.Context,
	client *http.Client,
	sourceURL string,
	destinationPath string,
	expectedSize int64,
	report progressReporter,
) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("could not prepare the download: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "acbfr/0.0.1")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the server returned %s", response.Status)
	}
	if expectedSize > 0 && response.ContentLength > 0 && response.ContentLength != expectedSize {
		return fmt.Errorf(
			"unexpected size reported by the server: received %d bytes, expected %d",
			response.ContentLength,
			expectedSize,
		)
	}

	output, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("could not create the temporary file: %w", err)
	}

	totalBytes := response.ContentLength
	if totalBytes <= 0 {
		totalBytes = expectedSize
	}
	reportProgress(report, installationProgress{Stage: stageDownloading, TotalBytes: totalBytes})
	progress := &downloadProgressWriter{
		writer:     output,
		totalBytes: totalBytes,
		report:     report,
		lastReport: time.Now(),
	}
	buffer := make([]byte, downloadBufferSize)
	written, copyErr := io.CopyBuffer(progress, response.Body, buffer)
	progress.finish()
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("could not save the download: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("could not finalize the downloaded file: %w", closeErr)
	}
	if written == 0 {
		return fmt.Errorf("the server returned an empty file")
	}
	if response.ContentLength > 0 && written != response.ContentLength {
		return fmt.Errorf("incomplete download: received %d of %d bytes", written, response.ContentLength)
	}
	if expectedSize > 0 && written != expectedSize {
		return fmt.Errorf("incomplete download: received %d of %d bytes", written, expectedSize)
	}
	return nil
}

func installArchiveInBatches(
	ctx context.Context,
	tarPath string,
	archivePath string,
	gamePath string,
	workRoot string,
	batchSize int,
	report progressReporter,
) error {
	reportProgress(report, installationProgress{Stage: stageListing})
	files, err := listArchiveFiles(ctx, tarPath, archivePath, workRoot)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("the RAR archive contains no files")
	}

	batches := splitIntoBatches(files, batchSize)
	completedFiles := 0
	for batchIndex, batch := range batches {
		batchNumber := batchIndex + 1
		batchRoot := filepath.Join(workRoot, fmt.Sprintf("batch-%03d", batchNumber))
		if err := os.Mkdir(batchRoot, 0o755); err != nil {
			return fmt.Errorf("could not create batch %d: %w", batchNumber, err)
		}

		reportProgress(report, installationProgress{
			Stage:          stageExtracting,
			CompletedFiles: completedFiles,
			TotalFiles:     len(files),
			CurrentBatch:   batchNumber,
			TotalBatches:   len(batches),
		})
		arguments := []string{"-xf", archivePath, "-C", batchRoot}
		for _, fileName := range batch {
			arguments = append(arguments, "--include="+fileName)
		}
		output, extractErr := exec.CommandContext(ctx, tarPath, arguments...).CombinedOutput()
		if extractErr != nil {
			_ = os.RemoveAll(batchRoot)
			return fmt.Errorf(
				"could not extract batch %d/%d: %w: %s",
				batchNumber,
				len(batches),
				extractErr,
				strings.TrimSpace(string(output)),
			)
		}
		if err := verifyExtractedBatch(batchRoot, batch); err != nil {
			_ = os.RemoveAll(batchRoot)
			return fmt.Errorf("batch %d/%d is invalid: %w", batchNumber, len(batches), err)
		}

		copied, err := copyBatchWithProgress(
			batchRoot,
			gamePath,
			completedFiles,
			len(files),
			batchNumber,
			len(batches),
			report,
		)
		if err != nil {
			_ = os.RemoveAll(batchRoot)
			return fmt.Errorf("could not copy batch %d/%d: %w", batchNumber, len(batches), err)
		}
		if copied != len(batch) {
			_ = os.RemoveAll(batchRoot)
			return fmt.Errorf("batch %d copied %d of %d files", batchNumber, copied, len(batch))
		}
		completedFiles += copied
		if err := os.RemoveAll(batchRoot); err != nil {
			return fmt.Errorf("could not clean up batch %d: %w", batchNumber, err)
		}
	}
	return nil
}

func listArchiveFiles(ctx context.Context, tarPath string, archivePath string, validationRoot string) ([]string, error) {
	namesOutput, err := exec.CommandContext(ctx, tarPath, "-tf", archivePath).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("could not list the RAR archive: %w: %s", err, strings.TrimSpace(string(namesOutput)))
	}
	verboseOutput, err := exec.CommandContext(ctx, tarPath, "-tvf", archivePath).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("could not inspect the RAR archive: %w: %s", err, strings.TrimSpace(string(verboseOutput)))
	}

	names := outputLines(namesOutput)
	verboseLines := outputLines(verboseOutput)
	if len(names) != len(verboseLines) {
		return nil, fmt.Errorf("the RAR listing is inconsistent: %d names and %d details", len(names), len(verboseLines))
	}

	files := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for index, archiveName := range names {
		verboseLine := verboseLines[index]
		if verboseLine == "" {
			return nil, fmt.Errorf("entry %q has no metadata", archiveName)
		}
		entryType := verboseLine[0]
		if entryType == 'd' {
			continue
		}
		if entryType != '-' {
			return nil, fmt.Errorf("unsupported RAR entry type: %q", archiveName)
		}
		if _, err := archiveDestination(validationRoot, archiveName); err != nil {
			return nil, err
		}
		key := strings.ToLower(strings.ReplaceAll(archiveName, `\`, "/"))
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate file in RAR archive: %q", archiveName)
		}
		seen[key] = struct{}{}
		files = append(files, archiveName)
	}
	return files, nil
}

func outputLines(output []byte) []string {
	normalized := strings.ReplaceAll(string(output), "\r\n", "\n")
	rawLines := strings.Split(normalized, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSuffix(line, "\r")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitIntoBatches(files []string, batchSize int) [][]string {
	if batchSize <= 0 || len(files) == 0 {
		return nil
	}
	batches := make([][]string, 0, (len(files)+batchSize-1)/batchSize)
	for start := 0; start < len(files); start += batchSize {
		end := start + batchSize
		if end > len(files) {
			end = len(files)
		}
		batches = append(batches, files[start:end])
	}
	return batches
}

func verifyExtractedBatch(batchRoot string, expectedFiles []string) error {
	expected := make(map[string]struct{}, len(expectedFiles))
	for _, fileName := range expectedFiles {
		expected[strings.ToLower(path.Clean(strings.ReplaceAll(fileName, `\`, "/")))] = struct{}{}
	}

	found := 0
	err := filepath.WalkDir(batchRoot, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported extracted file type: %q", entry.Name())
		}
		relative, err := filepath.Rel(batchRoot, filePath)
		if err != nil {
			return err
		}
		key := strings.ToLower(filepath.ToSlash(relative))
		if _, exists := expected[key]; !exists {
			return fmt.Errorf("unexpected extracted file: %q", relative)
		}
		delete(expected, key)
		found++
		return nil
	})
	if err != nil {
		return err
	}
	if found != len(expectedFiles) || len(expected) != 0 {
		return fmt.Errorf("extracted %d of %d files", found, len(expectedFiles))
	}
	return nil
}

func copyBatchWithProgress(
	sourceRoot string,
	destinationRoot string,
	completedBefore int,
	totalFiles int,
	currentBatch int,
	totalBatches int,
	report progressReporter,
) (int, error) {
	copiedFiles := 0
	err := filepath.WalkDir(sourceRoot, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceRoot, sourcePath)
		if err != nil || relative == "." {
			return err
		}
		destinationPath := filepath.Join(destinationRoot, relative)
		if entry.IsDir() {
			return os.MkdirAll(destinationPath, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported file type: %q", relative)
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("could not read metadata for %q: %w", relative, err)
		}
		input, err := os.Open(sourcePath)
		if err != nil {
			return fmt.Errorf("could not open source %q: %w", relative, err)
		}
		if err := makeDestinationWritable(destinationPath); err != nil {
			input.Close()
			return fmt.Errorf("could not prepare destination %q: %w", relative, err)
		}
		output, err := os.OpenFile(
			destinationPath,
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
			info.Mode().Perm()|0o200,
		)
		if err != nil {
			input.Close()
			return fmt.Errorf("could not open destination %q: %w", relative, err)
		}

		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		outputCloseErr := output.Close()
		if copyErr != nil {
			return fmt.Errorf("could not copy %q: %w", relative, copyErr)
		}
		if inputCloseErr != nil {
			return fmt.Errorf("could not close source %q: %w", relative, inputCloseErr)
		}
		if outputCloseErr != nil {
			return fmt.Errorf("could not close destination %q: %w", relative, outputCloseErr)
		}
		if err := os.Chtimes(destinationPath, info.ModTime(), info.ModTime()); err != nil {
			return fmt.Errorf("could not update the timestamp of %q: %w", relative, err)
		}

		copiedFiles++
		reportProgress(report, installationProgress{
			Stage:          stageCopying,
			CompletedFiles: completedBefore + copiedFiles,
			TotalFiles:     totalFiles,
			CurrentBatch:   currentBatch,
			TotalBatches:   totalBatches,
		})
		return nil
	})
	return copiedFiles, err
}

func makeDestinationWritable(destinationPath string) error {
	info, err := os.Lstat(destinationPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("the existing destination is not a regular file")
	}
	if info.Mode().Perm()&0o200 != 0 {
		return nil
	}
	if err := os.Chmod(destinationPath, info.Mode().Perm()|0o200); err != nil {
		return fmt.Errorf("could not remove the read-only attribute: %w", err)
	}
	return nil
}

func archiveDestination(root string, archiveName string) (string, error) {
	normalized := strings.ReplaceAll(archiveName, `\`, "/")
	cleanName := path.Clean(normalized)
	localName := filepath.FromSlash(cleanName)
	if cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, "../") ||
		path.IsAbs(cleanName) || filepath.IsAbs(localName) || filepath.VolumeName(localName) != "" {
		return "", fmt.Errorf("invalid path inside the RAR archive: %q", archiveName)
	}

	target := filepath.Join(root, localName)
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path inside the RAR archive: %q", archiveName)
	}
	return target, nil
}

type downloadProgressWriter struct {
	writer     io.Writer
	totalBytes int64
	written    int64
	lastReport time.Time
	report     progressReporter
}

func (writer *downloadProgressWriter) Write(data []byte) (int, error) {
	written, err := writer.writer.Write(data)
	writer.written += int64(written)
	now := time.Now()
	if now.Sub(writer.lastReport) >= 100*time.Millisecond ||
		(writer.totalBytes > 0 && writer.written >= writer.totalBytes) {
		writer.sendProgress()
		writer.lastReport = now
	}
	return written, err
}

func (writer *downloadProgressWriter) finish() {
	writer.sendProgress()
}

func (writer *downloadProgressWriter) sendProgress() {
	reportProgress(writer.report, installationProgress{
		Stage:      stageDownloading,
		Downloaded: writer.written,
		TotalBytes: writer.totalBytes,
	})
}

func reportProgress(report progressReporter, progress installationProgress) {
	if report != nil {
		report(progress)
	}
}
