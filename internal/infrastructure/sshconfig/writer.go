// ==============================================================================
// File: internal/infrastructure/sshconfig/writer.go
// Project: lazyssh
// Description: Atomic, crash-safe persistence engine for SSH configuration files
//              enforcing strict POSIX 0600 permissions and hardware fsync.
// ==============================================================================

package sshconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Writer provides crash-safe, atomic file persistence for SSH configuration documents.
type Writer struct{}

// NewWriter returns a new Writer instance.
func NewWriter() *Writer {
	return &Writer{}
}

// WriteAtomic writes the Document to the target path using atomic replacement with fsync.
// Invariant: Permissions are strictly enforced to 0600 (POSIX rw-------).
func (w *Writer) WriteAtomic(filePath string, doc *Document) error {
	expandedPath, err := expandHome(filePath)
	if err != nil {
		return fmt.Errorf("expanding target path '%s': %w", filePath, err)
	}

	dir := filepath.Dir(expandedPath)
	base := filepath.Base(expandedPath)

	// Ensure parent directory exists
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating directory '%s': %w", dir, err)
	}

	// 1. Create temporary file in the same directory (required for atomic os.Rename across filesystem boundaries)
	tempName := fmt.Sprintf(".%s.tmp.%d", base, time.Now().UnixNano())
	tempPath := filepath.Join(dir, tempName)

	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("creating temporary file '%s': %w", tempPath, err)
	}

	// Guarantee cleanup on failure
	var writeErr error
	defer func() {
		if writeErr != nil {
			_ = os.Remove(tempPath)
		}
	}()

	// 2. Serialize and write AST content
	content := doc.Serialize()
	if _, err := tempFile.WriteString(content); err != nil {
		writeErr = fmt.Errorf("writing content to temp file: %w", err)
		_ = tempFile.Close()
		return writeErr
	}

	// 3. Flush OS page cache to physical disk (POSIX fsync)
	if err := tempFile.Sync(); err != nil {
		writeErr = fmt.Errorf("fsync temporary file '%s': %w", tempPath, err)
		_ = tempFile.Close()
		return writeErr
	}

	if err := tempFile.Close(); err != nil {
		writeErr = fmt.Errorf("closing temporary file '%s': %w", tempPath, err)
		return writeErr
	}

	// 4. Atomic inode replacement
	if err := os.Rename(tempPath, expandedPath); err != nil {
		writeErr = fmt.Errorf("atomic rename '%s' to '%s': %w", tempPath, expandedPath, err)
		return writeErr
	}

	return nil
}
