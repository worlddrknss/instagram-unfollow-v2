package main

import (
	"fmt"
	"log/slog"

	"github.com/worlddrknss/instagram-unfollow-v2/pkg/storage"
)

func (app *application) listCandidates() error {
	// Open database to get candidates
	db, err := storage.Open("instagram.db")
	if err != nil {
		return err
	}
	defer db.Close()

	// Get stats
	unfollowedCount, err := storage.UnfollowedCount(db)
	if err != nil {
		app.logger.Warn("Could not get unfollowed count", slog.Any("error", err))
	}

	// Get candidates
	candidates, err := storage.UnfollowCandidates(db)
	if err != nil {
		return err
	}

	app.logger.Info("Unfollow statistics",
		slog.Int("remaining", len(candidates)),
		slog.Int("already_unfollowed", unfollowedCount),
	)

	if len(candidates) == 0 {
		fmt.Println("\nNo remaining users to unfollow!")
		return nil
	}

	fmt.Printf("\n=== Remaining Users to Unfollow (%d) ===\n\n", len(candidates))
	for i, candidate := range candidates {
		fmt.Printf("%4d. %s\n", i+1, candidate.Username)
	}
	fmt.Println()

	return nil
}
