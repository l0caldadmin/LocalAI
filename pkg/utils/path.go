package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ExistsInPath(path string, s string) bool {
	_, err := os.Stat(filepath.Join(path, s))
	return err == nil
}

func InTrustedRoot(path string, trustedRoot string) error {
	absBase, err := filepath.Abs(trustedRoot)
	if err != nil {
		return fmt.Errorf("resolving base dir: %w", err)
	}
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return fmt.Errorf("resolving base dir symlinks: %w", err)
	}

	absTarget, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving target path: %w", err)
	}
	realTarget, err := filepath.EvalSymlinks(absTarget)
	if err != nil {
		// File may not exist yet, walk up to first existing ancestor
		remaining := filepath.Base(absTarget)
		dir := filepath.Dir(absTarget)
		for {
			resolved, resolveErr := filepath.EvalSymlinks(dir)
			if resolveErr == nil {
				realTarget = filepath.Join(resolved, remaining)
				break
			}
			remaining = filepath.Join(filepath.Base(dir), remaining)
			parent := filepath.Dir(dir)
			if parent == dir {
				realTarget = filepath.Clean(absTarget)
				break
			}
			dir = parent
		}
	}

	if !strings.HasPrefix(realTarget, realBase+string(filepath.Separator)) && realTarget != realBase {
		return fmt.Errorf("path %q is outside allowed directory", path)
	}
	return nil
}

// VerifyPath verifies that path is based in basePath.
func VerifyPath(path, basePath string) error {
	c := filepath.Clean(filepath.Join(basePath, path))
	return InTrustedRoot(c, filepath.Clean(basePath))
}

// SanitizeFileName sanitizes the given filename
func SanitizeFileName(fileName string) string {
	// filepath.Clean to clean the path
	cleanName := filepath.Clean(fileName)
	// filepath.Base to ensure we only get the final element, not any directory path
	baseName := filepath.Base(cleanName)
	// Replace any remaining tricky characters that might have survived cleaning
	safeName := strings.ReplaceAll(baseName, "..", "")
	return safeName
}

func GenerateUniqueFileName(dir, baseName, ext string) string {
	counter := 1
	fileName := baseName + ext

	for {
		filePath := filepath.Join(dir, fileName)
		_, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			return fileName
		}

		counter++
		fileName = fmt.Sprintf("%s_%d%s", baseName, counter, ext)
	}
}
