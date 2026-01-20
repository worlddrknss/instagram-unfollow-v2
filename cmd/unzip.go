package main

import (
	"os"

	"log/slog"

	"github.com/worlddrknss/instagram-unfollow-v2/pkg/extraction"
)

func (app *application) unzipData(zipPath string) (string, error) {
	destDir := app.config.App.ExtractedPath
	if destDir == "" {
		var err error
		destDir, err = os.MkdirTemp("", "instagram-extracted-*")
		if err != nil {
			return "", err
		}
	}

	app.logger.Info("Unzipping data", slog.String("zipPath", zipPath), slog.String("destDir", destDir))
	if err := extraction.Unzip(zipPath, destDir); err != nil {
		return "", err
	}

	return destDir, nil
}
