package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Config holds browser automation settings
type Config struct {
	UnfollowDelaySeconds int
	MaxPerHour           int
	Headless             bool
	UserDataDir          string
}

// Browser wraps chromedp context for Instagram automation
type Browser struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *slog.Logger
	config Config
}

// generateUserAgent creates a realistic, randomized user agent string
func generateUserAgent() string {
	// Chrome versions (recent stable releases)
	chromeVersions := []string{"120.0.0.0", "121.0.0.0", "122.0.0.0", "123.0.0.0", "124.0.0.0", "125.0.0.0"}

	// OS configurations
	osConfigs := []struct {
		platform string
		versions []string
	}{
		{
			platform: "Macintosh; Intel Mac OS X",
			versions: []string{"10_15_7", "11_0_0", "12_0_0", "13_0_0", "14_0_0"},
		},
		{
			platform: "Windows NT",
			versions: []string{"10.0", "11.0"},
		},
		{
			platform: "X11; Linux",
			versions: []string{"x86_64"},
		},
	}

	// Select random components
	chromeVersion := chromeVersions[rand.Intn(len(chromeVersions))]
	osConfig := osConfigs[rand.Intn(len(osConfigs))]
	osVersion := osConfig.versions[rand.Intn(len(osConfig.versions))]

	// Build the user agent string
	var osPart string
	switch osConfig.platform {
	case "Macintosh; Intel Mac OS X":
		osPart = fmt.Sprintf("%s %s", osConfig.platform, osVersion)
	case "Windows NT":
		osPart = fmt.Sprintf("%s %s; Win64; x64", osConfig.platform, osVersion)
	case "X11; Linux":
		osPart = fmt.Sprintf("%s %s", osConfig.platform, osVersion)
	}

	return fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", osPart, chromeVersion)
}

// New creates a new Browser instance with optional persistent session
func New(logger *slog.Logger, cfg Config) (*Browser, error) {
	userAgent := generateUserAgent()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-infobars", true),
		chromedp.UserAgent(userAgent),
	)

	if cfg.Headless {
		opts = append(opts, chromedp.Headless)
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	if cfg.UserDataDir != "" {
		opts = append(opts, chromedp.UserDataDir(cfg.UserDataDir))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(logger.Info))

	logger.Info("Browser agent information", "user_agent", userAgent)

	return &Browser{
		ctx:    ctx,
		cancel: func() { cancel(); allocCancel() },
		logger: logger,
		config: cfg,
	}, nil
}

// Close shuts down the browser
func (b *Browser) Close() {
	b.cancel()
}

// IsLoggedIn checks if we have an active Instagram session
func (b *Browser) IsLoggedIn() (bool, error) {
	var loggedIn bool

	err := chromedp.Run(b.ctx,
		chromedp.Navigate("https://www.instagram.com/"),
		chromedp.Sleep(3*time.Second),
		chromedp.Evaluate(`document.querySelector('a[href="/accounts/login/"]') === null && document.querySelector('svg[aria-label="Home"]') !== null`, &loggedIn),
	)

	return loggedIn, err
}

// checkLoggedInWithoutNavigate checks login status on current page without refreshing
func (b *Browser) checkLoggedInWithoutNavigate() bool {
	var loggedIn bool
	chromedp.Run(b.ctx,
		chromedp.Evaluate(`document.querySelector('svg[aria-label="Home"]') !== null || document.querySelector('a[href="/direct/inbox/"]') !== null`, &loggedIn),
	)
	return loggedIn
}

// WaitForManualLogin opens Instagram and waits for user to log in manually
func (b *Browser) WaitForManualLogin() error {
	// First navigate to homepage to check if already logged in
	err := chromedp.Run(b.ctx,
		chromedp.Navigate("https://www.instagram.com/"),
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		return err
	}

	// Check if already logged in
	if b.checkLoggedInWithoutNavigate() {
		b.logger.Info("Already logged in")
		return nil
	}

	// Not logged in - navigate to login page once and wait
	b.logger.Info("Not logged in - please log in manually (you have 10 minutes)")
	err = chromedp.Run(b.ctx,
		chromedp.Navigate("https://www.instagram.com/accounts/login/"),
		chromedp.Sleep(2*time.Second),
	)
	if err != nil {
		return err
	}

	// Poll until logged in - 10 minute timeout, check every 5 seconds without refreshing
	for i := 0; i < 120; i++ {
		time.Sleep(5 * time.Second)
		if b.checkLoggedInWithoutNavigate() {
			b.logger.Info("Login detected")
			return nil
		}
		if i%6 == 0 { // Log every 30 seconds
			b.logger.Info("Waiting for login...", slog.Int("elapsed_seconds", (i+1)*5))
		}
	}

	return fmt.Errorf("login timeout - 10 minutes elapsed")
}

// UnfollowResult represents the outcome of an unfollow attempt
type UnfollowResult int

const (
	UnfollowSuccess            UnfollowResult = iota
	UnfollowNotFollowing                      // User shows "Follow" button - we're not following them
	UnfollowProfileUnavailable                // Profile doesn't exist or was removed
	UnfollowError
)

// Unfollow unfollows a single user by username and returns the result
func (b *Browser) Unfollow(username string) (UnfollowResult, error) {
	profileURL := fmt.Sprintf("https://www.instagram.com/%s/", username)
	b.logger.Info("Checking user", slog.String("username", username))

	err := chromedp.Run(b.ctx,
		chromedp.Navigate(profileURL),
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		return UnfollowError, fmt.Errorf("navigate to profile: %w", err)
	}

	// Check if profile is unavailable (deleted, banned, or doesn't exist)
	var profileUnavailable bool
	err = chromedp.Run(b.ctx,
		chromedp.Evaluate(`
			(function() {
				// Check for "Profile isn't available" or similar error messages
				const pageText = document.body.innerText || '';
				if (pageText.includes("Profile isn't available") ||
				    pageText.includes("Sorry, this page isn't available") ||
				    pageText.includes("The link may be broken")) {
					return true;
				}
				return false;
			})()
		`, &profileUnavailable),
	)
	if err != nil {
		return UnfollowError, fmt.Errorf("check profile availability: %w", err)
	}

	if profileUnavailable {
		b.logger.Info("Profile unavailable, skipping", slog.String("username", username))
		return UnfollowProfileUnavailable, nil
	}

	// First, check if we're actually following this user
	// Look for "Following" button vs "Follow" button
	var followStatus string
	err = chromedp.Run(b.ctx,
		chromedp.Evaluate(`
			(function() {
				// Check all buttons for Following or Follow text
				const buttons = document.querySelectorAll('button');
				for (const btn of buttons) {
					const text = btn.textContent.trim();
					if (text === 'Following' || text.includes('Following')) {
						return 'following';
					}
					if (text === 'Follow' && !text.includes('Following')) {
						return 'not_following';
					}
				}
				// Also check div role=button
				const divButtons = document.querySelectorAll('div[role="button"]');
				for (const btn of divButtons) {
					const text = btn.textContent.trim();
					if (text === 'Following' || text.includes('Following')) {
						return 'following';
					}
					if (text === 'Follow' && !text.includes('Following')) {
						return 'not_following';
					}
				}
				return 'unknown';
			})()
		`, &followStatus),
	)
	if err != nil {
		return UnfollowError, fmt.Errorf("check follow status: %w", err)
	}

	if followStatus == "not_following" {
		b.logger.Info("Not following this user, skipping", slog.String("username", username))
		return UnfollowNotFollowing, nil
	}

	if followStatus == "unknown" {
		b.logger.Warn("Could not determine follow status", slog.String("username", username))
		return UnfollowError, fmt.Errorf("could not determine follow status for %s", username)
	}

	b.logger.Info("Unfollowing user", slog.String("username", username))

	// Click the "Following" button with dropdown to open modal
	var clicked bool
	err = chromedp.Run(b.ctx,
		chromedp.Evaluate(`
			(function() {
				// Find the Following button - it contains "Following" text and has a dropdown caret
				const buttons = document.querySelectorAll('button');
				for (const btn of buttons) {
					if (btn.textContent.includes('Following')) {
						btn.click();
						return true;
					}
				}
				// Also check div role=button
				const divButtons = document.querySelectorAll('div[role="button"]');
				for (const btn of divButtons) {
					if (btn.textContent.includes('Following')) {
						btn.click();
						return true;
					}
				}
				return false;
			})()
		`, &clicked),
		chromedp.Sleep(2*time.Second),
	)
	if err != nil {
		return UnfollowError, fmt.Errorf("click following button: %w", err)
	}
	if !clicked {
		return UnfollowError, fmt.Errorf("following button not found for %s", username)
	}

	// Click "Unfollow" in the modal
	err = chromedp.Run(b.ctx,
		chromedp.Evaluate(`
			(function() {
				// Look for Unfollow button in the modal
				const buttons = document.querySelectorAll('button');
				for (const btn of buttons) {
					if (btn.textContent.trim() === 'Unfollow') {
						btn.click();
						return true;
					}
				}
				// Also check spans inside buttons
				const spans = document.querySelectorAll('button span, div[role="button"] span');
				for (const span of spans) {
					if (span.textContent.trim() === 'Unfollow') {
						span.closest('button, div[role="button"]').click();
						return true;
					}
				}
				return false;
			})()
		`, nil),
		chromedp.Sleep(2*time.Second), // Wait for UI to update
	)
	if err != nil {
		return UnfollowError, fmt.Errorf("click unfollow in modal: %w", err)
	}

	// Verify the unfollow worked - button should now say "Follow" instead of "Following"
	var newStatus string
	err = chromedp.Run(b.ctx,
		chromedp.Evaluate(`
			(function() {
				const buttons = document.querySelectorAll('button');
				for (const btn of buttons) {
					const text = btn.textContent.trim();
					if (text === 'Following' || text.includes('Following')) {
						return 'following';
					}
					if (text === 'Follow' && !text.includes('Following')) {
						return 'not_following';
					}
				}
				const divButtons = document.querySelectorAll('div[role="button"]');
				for (const btn of divButtons) {
					const text = btn.textContent.trim();
					if (text === 'Following' || text.includes('Following')) {
						return 'following';
					}
					if (text === 'Follow' && !text.includes('Following')) {
						return 'not_following';
					}
				}
				return 'unknown';
			})()
		`, &newStatus),
	)
	if err != nil {
		return UnfollowError, fmt.Errorf("verify unfollow status: %w", err)
	}

	if newStatus == "following" {
		b.logger.Warn("Unfollow may have failed - still showing Following button", slog.String("username", username))
		return UnfollowError, fmt.Errorf("unfollow verification failed for %s - still following", username)
	}

	if newStatus == "not_following" {
		b.logger.Info("Unfollowed successfully - verified button changed to Follow", slog.String("username", username))
		return UnfollowSuccess, nil
	}

	// Unknown status - assume success but warn
	b.logger.Warn("Could not verify unfollow status, assuming success", slog.String("username", username))
	return UnfollowSuccess, nil
}

// UnfollowBatch unfollows multiple users with configured delays
func (b *Browser) UnfollowBatch(usernames []string, maxCount int) (int, error) {
	if maxCount <= 0 || maxCount > len(usernames) {
		maxCount = len(usernames)
	}

	successful := 0
	delay := time.Duration(b.config.UnfollowDelaySeconds) * time.Second

	for i, username := range usernames[:maxCount] {
		result, err := b.Unfollow(username)
		if result != UnfollowSuccess {
			if err != nil {
				b.logger.Error("Failed to unfollow", slog.String("username", username), slog.Any("error", err))
			}
			continue
		}
		successful++

		// Check hourly limit
		if successful >= b.config.MaxPerHour {
			b.logger.Info("Reached hourly limit", slog.Int("count", successful))
			break
		}

		// Delay between unfollows (except after last one)
		if i < maxCount-1 {
			b.logger.Info("Waiting before next unfollow", slog.Duration("delay", delay))
			time.Sleep(delay)
		}
	}

	return successful, nil
}

// SaveCookies saves current session cookies to a file
func (b *Browser) SaveCookies(path string) error {
	var cookies []*network.Cookie

	err := chromedp.Run(b.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			return err
		}),
	)
	if err != nil {
		return err
	}

	data, err := json.Marshal(cookies)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// LoadCookies loads session cookies from a file
func (b *Browser) LoadCookies(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var cookies []*network.Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return err
	}

	return chromedp.Run(b.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			for _, cookie := range cookies {
				err := network.SetCookie(cookie.Name, cookie.Value).
					WithDomain(cookie.Domain).
					WithPath(cookie.Path).
					WithSecure(cookie.Secure).
					WithHTTPOnly(cookie.HTTPOnly).
					Do(ctx)
				if err != nil {
					return err
				}
			}
			return nil
		}),
	)
}
