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

	// Setup browser once
	homeDir, _ := os.UserHomeDir()
	userDataDir := filepath.Join(homeDir, ".instagram-unfollow", "chrome-profile")

	cfg := browser.Config{
		UnfollowDelaySeconds: app.config.App.UnfollowDelaySeconds,
		MaxPerHour:           app.config.Instagram.AutomationLimits.Actions.Hourly,
		Headless:             false, // Run visible so user can handle 2FA
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

	hourlyLimit := app.config.Instagram.AutomationLimits.Actions.Hourly
	delay := app.config.App.UnfollowDelaySeconds
	sessionDuration := 1 * time.Hour

	// Main loop - runs until no more candidates
	for {
		// Log how many we've already unfollowed
		unfollowedCount, _ := storage.UnfollowedCount(db)
		app.logger.Info("Previously unfollowed users", slog.Int("count", unfollowedCount))

		// Get candidates
		candidates, err := storage.UnfollowCandidates(db)
		if err != nil {
			return err
		}
		app.logger.Info("Loaded unfollow candidates", slog.Int("count", len(candidates)))

		if len(candidates) == 0 {
			app.logger.Info("No more unfollow candidates - all done!")
			return nil
		}

		// Start session timer
		sessionStart := time.Now()
		app.logger.Info("Starting new session",
			slog.Time("session_start", sessionStart),
			slog.Int("max_unfollows", hourlyLimit),
		)

		// Process up to hourly limit
		maxCount := hourlyLimit
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
				// Remove from following table since we're no longer following
				if err := storage.RemoveFromFollowing(db, username); err != nil {
					app.logger.Error("Failed to remove from following table", slog.Any("error", err))
				}
				successful++

			case browser.UnfollowNotFollowing:
				// User shows "Follow" button - we're not actually following them
				if err := storage.MarkNotFollowing(db, username); err != nil {
					app.logger.Error("Failed to mark not following", slog.String("username", username), slog.Any("error", err))
				}
				if err := storage.RemoveFromFollowing(db, username); err != nil {
					app.logger.Error("Failed to remove from following table", slog.Any("error", err))
				}
				skipped++
				continue // Don't count against rate limit, skip delay

			case browser.UnfollowProfileUnavailable:
				// Profile doesn't exist or was removed
				if err := storage.RemoveFromFollowing(db, username); err != nil {
					app.logger.Error("Failed to remove from following table", slog.Any("error", err))
				}
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
			if successful >= hourlyLimit {
				app.logger.Info("Reached hourly limit", slog.Int("count", successful))
				break
			}

			// Delay between unfollows (except after last one)
			if i < maxCount-1 && successful < hourlyLimit {
				app.logger.Info("Waiting before next unfollow", slog.Int("delay_seconds", delay))
				time.Sleep(time.Duration(delay) * time.Second)
			}
		}

		// Session complete
		sessionEnd := time.Now()
		sessionElapsed := sessionEnd.Sub(sessionStart)

		app.logger.Info("Session complete",
			slog.Int("unfollowed", successful),
			slog.Int("skipped_not_following", skipped),
			slog.Int("profiles_unavailable", unavailable),
			slog.Duration("session_duration", sessionElapsed),
		)

		// If session took less than 1 hour, wait for the remainder
		if sessionElapsed < sessionDuration {
			waitTime := sessionDuration - sessionElapsed
			app.logger.Info("Waiting for session cooldown",
				slog.Duration("wait_time", waitTime),
				slog.Time("next_session", time.Now().Add(waitTime)),
			)
			time.Sleep(waitTime)
		}

		// Loop continues with next session
	}
}
