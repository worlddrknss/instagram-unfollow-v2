package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/worlddrknss/instagram-unfollow-v2/pkg/browser"
	"github.com/worlddrknss/instagram-unfollow-v2/pkg/power"
	"github.com/worlddrknss/instagram-unfollow-v2/pkg/storage"
)

func (app *application) runUnfollow() error {
	// Prevent system from sleeping during automation
	sleepInhibitor := power.NewInhibitor(app.logger)
	if err := sleepInhibitor.Start(); err != nil {
		app.logger.Warn("Could not prevent system sleep", slog.Any("error", err))
	}
	defer sleepInhibitor.Stop()

	// Open database to get candidates
	db, err := storage.Open("instagram.db")
	if err != nil {
		return err
	}
	defer db.Close()

	// Log how many we've already unfollowed
	unfollowedCount, _ := storage.UnfollowedCount(db)
	app.logger.Info("Previously unfollowed users", slog.Int("count", unfollowedCount))

	// Check how many actions we've done in the last hour for rate limiting
	actionsLastHour, err := storage.ActionsInLastHour(db, "unfollow")
	if err != nil {
		app.logger.Warn("Could not check recent actions", slog.Any("error", err))
		actionsLastHour = 0
	}
	hourlyLimit := app.config.Instagram.AutomationLimits.Actions.Hourly
	remainingThisHour := hourlyLimit - actionsLastHour
	if remainingThisHour < 0 {
		remainingThisHour = 0
	}
	app.logger.Info("Rate limit status",
		slog.Int("actions_last_hour", actionsLastHour),
		slog.Int("hourly_limit", hourlyLimit),
		slog.Int("remaining", remainingThisHour),
	)

	if remainingThisHour <= 0 {
		app.logger.Info("Hourly rate limit reached, please wait and try again later")
		return nil
	}

	candidates, err := storage.UnfollowCandidates(db)
	if err != nil {
		return err
	}
	app.logger.Info("Loaded unfollow candidates", slog.Int("count", len(candidates)))

	if len(candidates) == 0 {
		app.logger.Info("No unfollow candidates found")
		return nil
	}

	// Setup browser
	homeDir, _ := os.UserHomeDir()
	userDataDir := filepath.Join(homeDir, ".instagram-unfollow", "chrome-profile")

	cfg := browser.Config{
		UnfollowDelaySeconds: app.config.App.UnfollowDelaySeconds,
		MaxPerHour:           remainingThisHour, // Use remaining capacity
		Headless:             false,             // Run visible so user can handle 2FA
		UserDataDir:          userDataDir,
	}

	b, err := browser.New(app.logger, cfg)
	if err != nil {
		return err
	}
	defer b.Close()

	// Wait for user to log in (will skip quickly if already logged in via persistent session)
	app.logger.Info("Checking login status...")
	if err := b.WaitForManualLogin(); err != nil {
		return err
	}

	// Run unfollow with DB marking
	delay := app.config.App.UnfollowDelaySeconds
	maxCount := remainingThisHour
	if maxCount > len(candidates) {
		maxCount = len(candidates)
	}

	successful := 0
	skipped := 0
	unavailable := 0
	for i, candidate := range candidates[:maxCount] {
		username := candidate.Username
		result, err := b.Unfollow(username)

		switch result {
		case browser.UnfollowSuccess:
			// Mark as unfollowed in database
			if err := storage.MarkUnfollowed(db, username); err != nil {
				app.logger.Error("Failed to mark unfollowed in DB", slog.String("username", username), slog.Any("error", err))
			}
			// Record action for rate limiting
			if err := storage.RecordAction(db, "unfollow", username); err != nil {
				app.logger.Error("Failed to record action", slog.Any("error", err))
			}
			// Remove from following table since we're no longer following
			if err := storage.RemoveFromFollowing(db, username); err != nil {
				app.logger.Error("Failed to remove from following table", slog.Any("error", err))
			}
			successful++

		case browser.UnfollowNotFollowing:
			// User shows "Follow" button - we're not actually following them
			// Mark them so we don't process again
			if err := storage.MarkNotFollowing(db, username); err != nil {
				app.logger.Error("Failed to mark not following", slog.String("username", username), slog.Any("error", err))
			}
			// Remove from following table since the data was stale
			if err := storage.RemoveFromFollowing(db, username); err != nil {
				app.logger.Error("Failed to remove from following table", slog.Any("error", err))
			}
			skipped++
			continue // Don't count against rate limit, skip delay

		case browser.UnfollowProfileUnavailable:
			// Profile doesn't exist or was removed
			// Remove from following table since the account is gone
			if err := storage.RemoveFromFollowing(db, username); err != nil {
				app.logger.Error("Failed to remove from following table", slog.Any("error", err))
			}
			// Mark as not following so we don't try again
			if err := storage.MarkNotFollowing(db, username); err != nil {
				app.logger.Error("Failed to mark not following", slog.String("username", username), slog.Any("error", err))
			}
			unavailable++
			continue // Don't count against rate limit, skip delay

		case browser.UnfollowError:
			app.logger.Error("Failed to unfollow", slog.String("username", username), slog.Any("error", err))
			continue
		}

		// Check hourly limit
		if successful >= remainingThisHour {
			app.logger.Info("Reached hourly limit", slog.Int("count", successful))
			break
		}

		// Delay between unfollows (except after last one)
		if i < maxCount-1 {
			app.logger.Info("Waiting before next unfollow", slog.Int("delay_seconds", delay))
			time.Sleep(time.Duration(delay) * time.Second)
		}
	}

	app.logger.Info("Unfollow session complete",
		slog.Int("unfollowed", successful),
		slog.Int("skipped_not_following", skipped),
		slog.Int("profiles_unavailable", unavailable),
	)
	return nil
}
