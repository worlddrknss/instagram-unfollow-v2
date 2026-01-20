package extraction

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Unzip(zipFile, destDir string) error {
	reader, err := zip.OpenReader(zipFile)
	if err != nil {
		return err
	}
	defer reader.Close()

	destDir = filepath.Clean(destDir)

	for _, file := range reader.File {
		if err := extractFile(file, destDir); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(file *zip.File, destDir string) error {
	path := filepath.Join(destDir, file.Name)

	// Guard against ZipSlip
	if !strings.HasPrefix(path, filepath.Clean(destDir)+string(os.PathSeparator)) {
		return fmt.Errorf("illegal file path: %s", path)
	}

	if file.FileInfo().IsDir() {
		return os.MkdirAll(path, os.ModePerm)
	}

	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}

	dstFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	srcFile, err := file.Open()
	if err != nil {
		return err
	}
	defer srcFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
