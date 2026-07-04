package utils

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func IsArchive(file string) bool {
	_, ok := archiveFormat(file)
	return ok
}

func ExtractArchive(archive, dst string) error {
	format, ok := archiveFormat(archive)
	if !ok {
		return fmt.Errorf("format specified by source filename is not an archive format: %s", archive)
	}

	extractRoot, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		return err
	}

	switch format {
	case archiveFormatZip:
		return extractZipArchive(archive, extractRoot)
	case archiveFormatTar:
		return extractTarArchive(archive, extractRoot, archive)
	case archiveFormatTarGz:
		return extractCompressedTarArchive(archive, extractRoot, archive, func(r io.Reader) (io.Reader, error) {
			gr, err := gzip.NewReader(r)
			if err != nil {
				return nil, err
			}
			return gr, nil
		})
	case archiveFormatTarBz2:
		return extractCompressedTarArchive(archive, extractRoot, archive, func(r io.Reader) (io.Reader, error) {
			return bzip2.NewReader(r), nil
		})
	default:
		return fmt.Errorf("unsupported archive format: %s", archive)
	}
}

type archiveType int

const (
	archiveFormatTar archiveType = iota
	archiveFormatTarGz
	archiveFormatTarBz2
	archiveFormatZip
)

func archiveFormat(path string) (archiveType, bool) {
	p := strings.ToLower(path)
	switch {
	case strings.HasSuffix(p, ".tar.gz"), strings.HasSuffix(p, ".tgz"):
		return archiveFormatTarGz, true
	case strings.HasSuffix(p, ".tar.bz2"), strings.HasSuffix(p, ".tbz2"):
		return archiveFormatTarBz2, true
	case strings.HasSuffix(p, ".tar"):
		return archiveFormatTar, true
	case strings.HasSuffix(p, ".zip"):
		return archiveFormatZip, true
	default:
		return 0, false
	}
}

func extractZipArchive(archive, extractRoot string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, f := range reader.File {
		safePath, err := safeArchiveTarget(extractRoot, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive contains a symlink")
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(safePath, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(safePath), 0o755); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		out, err := os.OpenFile(safePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			_ = in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		_ = in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}

	return nil
}

func extractCompressedTarArchive(archive, extractRoot, source string, decoder func(io.Reader) (io.Reader, error)) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()

	decoded, err := decoder(file)
	if err != nil {
		return err
	}

	if closer, ok := decoded.(io.Closer); ok {
		defer closer.Close()
	}

	return extractTarReader(decoded, extractRoot, source)
}

func extractTarArchive(archive, extractRoot, source string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	return extractTarReader(file, extractRoot, source)
}

func extractTarReader(r io.Reader, extractRoot, source string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		safePath, err := safeArchiveTarget(extractRoot, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("archive contains a symlink")
		case tar.TypeDir:
			if err := os.MkdirAll(safePath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA, tar.TypeGNUSparse:
			if err := os.MkdirAll(filepath.Dir(safePath), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(hdr.Mode)
			if mode.Perm() == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(safePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			continue
		default:
			return fmt.Errorf("unsupported tar entry type in %s: %s", source, hdr.Name)
		}
	}
}

func safeArchiveTarget(root, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("archive contains an empty path")
	}

	normalizedName := filepath.FromSlash(strings.ReplaceAll(name, "\\", "/"))
	cleanedName := filepath.Clean(normalizedName)
	if filepath.IsAbs(cleanedName) || cleanedName == ".." || strings.HasPrefix(cleanedName, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive contains an unsafe path: %s", name)
	}

	targetPath := filepath.Join(root, cleanedName)
	relativePath, err := filepath.Rel(root, targetPath)
	if err != nil {
		return "", err
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(os.PathSeparator)) || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("archive contains an unsafe path: %s", name)
	}

	return targetPath, nil
}
