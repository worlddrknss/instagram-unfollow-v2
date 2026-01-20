package main

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/worlddrknss/instagram-unfollow-v2/pkg/extraction"
	"github.com/worlddrknss/instagram-unfollow-v2/pkg/storage"
)

func (app *application) parseToDB(extractedDir string) error {
	connectionsDir := filepath.Join(extractedDir, "connections", "followers_and_following")

	// Open database
	dbPath := "instagram.db"
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Parse and insert following
	followingPath := filepath.Join(connectionsDir, "following.json")
	following, err := extraction.ParseFollowing(followingPath)
	if err != nil {
		return fmt.Errorf("parse following: %w", err)
	}
	if err := storage.UpsertFollowing(db, following); err != nil {
		return fmt.Errorf("upsert following: %w", err)
	}
	app.logger.Info("Imported following", slog.Int("count", len(following)))

	// Find and parse all followers files
	followerFiles, err := extraction.FindFollowerFiles(connectionsDir)
	if err != nil {
		return fmt.Errorf("find follower files: %w", err)
	}

	var allFollowers []storage.Relationship
	for _, file := range followerFiles {
		followers, err := extraction.ParseFollowers(file)
		if err != nil {
			return fmt.Errorf("parse %s: %w", file, err)
		}
		allFollowers = append(allFollowers, followers...)
	}

	if err := storage.UpsertFollowers(db, allFollowers); err != nil {
		return fmt.Errorf("upsert followers: %w", err)
	}
	app.logger.Info("Imported followers", slog.Int("count", len(allFollowers)), slog.Int("files", len(followerFiles)))

	// Get unfollow candidates
	candidates, err := storage.UnfollowCandidates(db)
	if err != nil {
		return fmt.Errorf("get candidates: %w", err)
	}
	app.logger.Info("Found unfollow candidates", slog.Int("count", len(candidates)))

	return nil
}
