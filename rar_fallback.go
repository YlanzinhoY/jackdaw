package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwaples/rardecode/v2"
)

// installRarArchiveInBatches extracts RAR files without depending on the
// tar.exe version installed on the user's PC. It keeps only one batch in the
// temporary workspace, matching the disk-space behaviour of the tar path.
func installRarArchiveInBatches(
	ctx context.Context,
	archivePath string,
	gamePath string,
	workRoot string,
	batchSize int,
	archivePassword string,
	report progressReporter,
) error {
	if batchSize <= 0 {
		return fmt.Errorf("the batch size must be greater than zero")
	}

	reportProgress(report, installationProgress{Stage: stageListing})
	files, err := listRarArchiveFiles(archivePath, gamePath, archivePassword)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("the RAR archive contains no files")
	}

	reader, err := openRarArchive(archivePath, archivePassword)
	if err != nil {
		return fmt.Errorf("could not open the RAR archive: %w", err)
	}
	defer reader.Close()

	expected := make(map[string]struct{}, len(files))
	for _, name := range files {
		expected[rarEntryKey(name)] = struct{}{}
	}

	batches := splitIntoBatches(files, batchSize)
	batchIndex := 0
	batchFiles := make([]string, 0, batchSize)
	batchRoot := ""
	completedFiles := 0
	seenFiles := 0

	startBatch := func() error {
		batchNumber := batchIndex + 1
		batchRoot = filepath.Join(workRoot, fmt.Sprintf("rar-fallback-%03d", batchNumber))
		if err := os.Mkdir(batchRoot, 0o755); err != nil {
			return fmt.Errorf("could not create fallback batch %d: %w", batchNumber, err)
		}
		batchFiles = batchFiles[:0]
		reportProgress(report, installationProgress{
			Stage:          stageExtracting,
			CompletedFiles: completedFiles,
			TotalFiles:     len(files),
			CurrentBatch:   batchNumber,
			TotalBatches:   len(batches),
		})
		return nil
	}
	copyBatch := func() error {
		if len(batchFiles) == 0 {
			return nil
		}
		batchNumber := batchIndex + 1
		if err := verifyExtractedBatch(batchRoot, batchFiles); err != nil {
			return fmt.Errorf("fallback batch %d/%d is invalid: %w", batchNumber, len(batches), err)
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
			return fmt.Errorf("could not copy fallback batch %d/%d: %w", batchNumber, len(batches), err)
		}
		if copied != len(batchFiles) {
			return fmt.Errorf("fallback batch %d copied %d of %d files", batchNumber, copied, len(batchFiles))
		}
		completedFiles += copied
		if err := os.RemoveAll(batchRoot); err != nil {
			return fmt.Errorf("could not clean up fallback batch %d: %w", batchNumber, err)
		}
		batchIndex++
		batchRoot = ""
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("could not read the RAR archive: %w", nextErr)
		}
		if header.IsDir {
			continue
		}
		if err := validateRarFileHeader(gamePath, header, archivePassword); err != nil {
			return err
		}
		key := rarEntryKey(header.Name)
		if _, exists := expected[key]; !exists {
			return fmt.Errorf("unexpected file in RAR archive: %q", header.Name)
		}
		delete(expected, key)

		if batchRoot == "" {
			if err := startBatch(); err != nil {
				return err
			}
		}
		if err := extractRarEntry(reader, header, batchRoot); err != nil {
			return err
		}
		batchFiles = append(batchFiles, header.Name)
		seenFiles++
		if len(batchFiles) == batchSize {
			if err := copyBatch(); err != nil {
				return err
			}
		}
	}

	if len(batchFiles) > 0 {
		if err := copyBatch(); err != nil {
			return err
		}
	}
	if seenFiles != len(files) || len(expected) != 0 || completedFiles != len(files) {
		return fmt.Errorf("the RAR archive changed while it was being extracted")
	}
	return nil
}

func listRarArchiveFiles(archivePath string, validationRoot string, archivePassword string) ([]string, error) {
	reader, err := openRarArchive(archivePath, archivePassword)
	if err != nil {
		return nil, fmt.Errorf("could not open the RAR archive: %w", err)
	}
	defer reader.Close()

	files := make([]string, 0)
	seen := make(map[string]struct{})
	for {
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("could not list the RAR archive: %w", nextErr)
		}
		if header.IsDir {
			continue
		}
		if err := validateRarFileHeader(validationRoot, header, archivePassword); err != nil {
			return nil, err
		}
		key := rarEntryKey(header.Name)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate file in RAR archive: %q", header.Name)
		}
		seen[key] = struct{}{}
		files = append(files, header.Name)
	}
	return files, nil
}

func openRarArchive(archivePath string, archivePassword string) (*rardecode.ReadCloser, error) {
	if archivePassword == "" {
		return rardecode.OpenReader(archivePath)
	}
	return rardecode.OpenReader(archivePath, rardecode.Password(archivePassword))
}

func validateRarFileHeader(validationRoot string, header *rardecode.FileHeader, archivePassword string) error {
	if (header.HeaderEncrypted || header.Encrypted) && archivePassword == "" {
		return fmt.Errorf("encrypted RAR entry requires a password: %q", header.Name)
	}
	if !header.Mode().IsRegular() {
		return fmt.Errorf("unsupported RAR entry type: %q", header.Name)
	}
	if _, err := archiveDestination(validationRoot, header.Name); err != nil {
		return err
	}
	return nil
}

func extractRarEntry(reader io.Reader, header *rardecode.FileHeader, batchRoot string) error {
	destinationPath, err := archiveDestination(batchRoot, header.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf("could not create destination for %q: %w", header.Name, err)
	}
	output, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.Mode().Perm()|0o200)
	if err != nil {
		return fmt.Errorf("could not create extracted file %q: %w", header.Name, err)
	}
	buffer := make([]byte, downloadBufferSize)
	_, copyErr := io.CopyBuffer(output, reader, buffer)
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("could not extract %q: %w", header.Name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("could not finalize extracted file %q: %w", header.Name, closeErr)
	}
	if !header.ModificationTime.IsZero() {
		if err := os.Chtimes(destinationPath, header.ModificationTime, header.ModificationTime); err != nil {
			return fmt.Errorf("could not update the timestamp of %q: %w", header.Name, err)
		}
	}
	return nil
}

func rarEntryKey(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, `\\`, "/"))
}
