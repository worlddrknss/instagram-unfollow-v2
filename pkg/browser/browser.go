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
	"github.com/chromedp/cdproto/page"
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

// stealthScript contains JavaScript to override automation detection
const stealthScript = `
// ============================================
// CORE WEBDRIVER DETECTION BYPASS
// ============================================

// Override webdriver property with multiple approaches
Object.defineProperty(navigator, 'webdriver', {
	get: () => undefined,
	configurable: true
});

// Delete from prototype chain
try {
	delete Object.getPrototypeOf(navigator).webdriver;
} catch(e) {}

// Also handle __proto__ approach
try {
	delete navigator.__proto__.webdriver;
} catch(e) {}

// ============================================
// PLUGINS AND MIME TYPES
// ============================================

const mockPlugin = {
	0: {type: 'application/pdf', suffixes: 'pdf', description: 'Portable Document Format'},
	length: 1,
	name: 'PDF Viewer',
	description: 'Portable Document Format',
	filename: 'internal-pdf-viewer',
	item: function(i) { return this[i]; },
	namedItem: function(name) { return this[name]; }
};

Object.defineProperty(navigator, 'plugins', {
	get: () => {
		const plugins = [mockPlugin];
		plugins.item = (i) => plugins[i];
		plugins.namedItem = (name) => plugins.find(p => p.name === name);
		plugins.refresh = () => {};
		return plugins;
	},
});

Object.defineProperty(navigator, 'mimeTypes', {
	get: () => {
		const mimeTypes = [{
			type: 'application/pdf',
			suffixes: 'pdf',
			description: 'Portable Document Format',
			enabledPlugin: mockPlugin
		}];
		mimeTypes.item = (i) => mimeTypes[i];
		mimeTypes.namedItem = (name) => mimeTypes.find(m => m.type === name);
		return mimeTypes;
	},
});

// ============================================
// LANGUAGE AND LOCALE
// ============================================

Object.defineProperty(navigator, 'languages', {
	get: () => ['en-US', 'en'],
});

Object.defineProperty(navigator, 'language', {
	get: () => 'en-US',
});

// ============================================
// PERMISSIONS API
// ============================================

if (navigator.permissions) {
	const originalQuery = navigator.permissions.query.bind(navigator.permissions);
	navigator.permissions.query = (parameters) => (
		parameters.name === 'notifications' ?
			Promise.resolve({ state: Notification.permission }) :
			originalQuery(parameters)
	);
}

// ============================================
// CHROME OBJECT (must look like real Chrome)
// ============================================

window.chrome = {
	app: {
		isInstalled: false,
		InstallState: { DISABLED: 'disabled', INSTALLED: 'installed', NOT_INSTALLED: 'not_installed' },
		RunningState: { CANNOT_RUN: 'cannot_run', READY_TO_RUN: 'ready_to_run', RUNNING: 'running' }
	},
	runtime: {
		OnInstalledReason: { CHROME_UPDATE: 'chrome_update', INSTALL: 'install', SHARED_MODULE_UPDATE: 'shared_module_update', UPDATE: 'update' },
		OnRestartRequiredReason: { APP_UPDATE: 'app_update', OS_UPDATE: 'os_update', PERIODIC: 'periodic' },
		PlatformArch: { ARM: 'arm', ARM64: 'arm64', MIPS: 'mips', MIPS64: 'mips64', X86_32: 'x86-32', X86_64: 'x86-64' },
		PlatformNaclArch: { ARM: 'arm', MIPS: 'mips', MIPS64: 'mips64', X86_32: 'x86-32', X86_64: 'x86-64' },
		PlatformOs: { ANDROID: 'android', CROS: 'cros', LINUX: 'linux', MAC: 'mac', OPENBSD: 'openbsd', WIN: 'win' },
		RequestUpdateCheckStatus: { NO_UPDATE: 'no_update', THROTTLED: 'throttled', UPDATE_AVAILABLE: 'update_available' },
		connect: function() { return { onDisconnect: { addListener: function() {} }, onMessage: { addListener: function() {} }, postMessage: function() {} }; },
		sendMessage: function() {},
		id: undefined
	},
	csi: function() { return { pageT: Date.now(), startE: Date.now() - Math.random() * 1000, onloadT: Date.now() - Math.random() * 500 }; },
	loadTimes: function() {
		return {
			commitLoadTime: Date.now() / 1000 - Math.random() * 10,
			connectionInfo: 'h2',
			finishDocumentLoadTime: Date.now() / 1000 - Math.random() * 5,
			finishLoadTime: Date.now() / 1000 - Math.random() * 2,
			firstPaintAfterLoadTime: 0,
			firstPaintTime: Date.now() / 1000 - Math.random() * 8,
			navigationType: 'Other',
			npnNegotiatedProtocol: 'unknown',
			requestTime: Date.now() / 1000 - Math.random() * 15,
			startLoadTime: Date.now() / 1000 - Math.random() * 12,
			wasAlternateProtocolAvailable: false,
			wasNpnNegotiated: true
		};
	}
};

// ============================================
// NETWORK CONNECTION INFO
// ============================================

Object.defineProperty(navigator, 'connection', {
	get: () => ({
		effectiveType: '4g',
		rtt: 50 + Math.floor(Math.random() * 50),
		downlink: 10 + Math.random() * 5,
		saveData: false,
		onchange: null,
		addEventListener: function() {},
		removeEventListener: function() {},
		dispatchEvent: function() { return true; }
	}),
});

// ============================================
// HARDWARE INFO
// ============================================

Object.defineProperty(navigator, 'hardwareConcurrency', {
	get: () => 8,
});

Object.defineProperty(navigator, 'deviceMemory', {
	get: () => 8,
});

Object.defineProperty(navigator, 'maxTouchPoints', {
	get: () => 0
});

Object.defineProperty(navigator, 'vendor', {
	get: () => 'Google Inc.'
});

Object.defineProperty(navigator, 'platform', {
	get: () => navigator.userAgent.includes('Mac') ? 'MacIntel' : 
			   navigator.userAgent.includes('Win') ? 'Win32' : 'Linux x86_64'
});

// ============================================
// SCREEN PROPERTIES
// ============================================

Object.defineProperty(screen, 'colorDepth', { get: () => 24 });
Object.defineProperty(screen, 'pixelDepth', { get: () => 24 });

// Ensure screen dimensions match window
const screenWidth = window.outerWidth || 1920;
const screenHeight = window.outerHeight || 1080;
Object.defineProperty(screen, 'availWidth', { get: () => screenWidth });
Object.defineProperty(screen, 'availHeight', { get: () => screenHeight - 40 }); // Account for taskbar

// ============================================
// CANVAS FINGERPRINT PROTECTION
// ============================================

const originalToDataURL = HTMLCanvasElement.prototype.toDataURL;
HTMLCanvasElement.prototype.toDataURL = function(type) {
	if (type === 'image/png' || type === undefined) {
		const context = this.getContext('2d');
		if (context) {
			// Add subtle noise to canvas to create unique but consistent fingerprint
			const imageData = context.getImageData(0, 0, this.width, this.height);
			const data = imageData.data;
			// Very subtle modification that won't be visible
			for (let i = 0; i < data.length; i += 4) {
				data[i] = data[i] ^ 1; // Tiny XOR on red channel
			}
			context.putImageData(imageData, 0, 0);
		}
	}
	return originalToDataURL.apply(this, arguments);
};

const originalGetImageData = CanvasRenderingContext2D.prototype.getImageData;
CanvasRenderingContext2D.prototype.getImageData = function() {
	const imageData = originalGetImageData.apply(this, arguments);
	// Subtle modification
	for (let i = 0; i < imageData.data.length; i += 4) {
		imageData.data[i] = imageData.data[i] ^ 1;
	}
	return imageData;
};

// ============================================
// WEBGL FINGERPRINT PROTECTION
// ============================================

const getParameter = WebGLRenderingContext.prototype.getParameter;
WebGLRenderingContext.prototype.getParameter = function(parameter) {
	if (parameter === 37445) return 'Google Inc. (Apple)';
	if (parameter === 37446) return 'ANGLE (Apple, Apple M1, OpenGL 4.1)';
	return getParameter.call(this, parameter);
};

if (typeof WebGL2RenderingContext !== 'undefined') {
	const getParameter2 = WebGL2RenderingContext.prototype.getParameter;
	WebGL2RenderingContext.prototype.getParameter = function(parameter) {
		if (parameter === 37445) return 'Google Inc. (Apple)';
		if (parameter === 37446) return 'ANGLE (Apple, Apple M1, OpenGL 4.1)';
		return getParameter2.call(this, parameter);
	};
}

// ============================================
// AUDIO CONTEXT FINGERPRINT PROTECTION
// ============================================

if (typeof AudioContext !== 'undefined') {
	const origCreateOscillator = AudioContext.prototype.createOscillator;
	AudioContext.prototype.createOscillator = function() {
		const oscillator = origCreateOscillator.apply(this, arguments);
		// Return slightly modified oscillator
		return oscillator;
	};
	
	const origCreateAnalyser = AudioContext.prototype.createAnalyser;
	AudioContext.prototype.createAnalyser = function() {
		const analyser = origCreateAnalyser.apply(this, arguments);
		const origGetFloatFrequencyData = analyser.getFloatFrequencyData.bind(analyser);
		analyser.getFloatFrequencyData = function(array) {
			origGetFloatFrequencyData(array);
			// Add tiny noise
			for (let i = 0; i < array.length; i++) {
				array[i] += (Math.random() - 0.5) * 0.0001;
			}
		};
		return analyser;
	};
}

if (typeof OfflineAudioContext !== 'undefined') {
	const origOfflineCreateOscillator = OfflineAudioContext.prototype.createOscillator;
	OfflineAudioContext.prototype.createOscillator = function() {
		return origOfflineCreateOscillator.apply(this, arguments);
	};
}

// ============================================
// NOTIFICATION API
// ============================================

if (typeof Notification !== 'undefined') {
	Object.defineProperty(Notification, 'permission', {
		get: () => 'default'
	});
}

// ============================================
// IFRAME DETECTION PROTECTION
// ============================================

Object.defineProperty(HTMLIFrameElement.prototype, 'contentWindow', {
	get: function() {
		return window;
	}
});

// ============================================
// CDP (Chrome DevTools Protocol) DETECTION
// ============================================

// Hide CDP-related objects
delete window.cdc_adoQpoasnfa76pfcZLmcfl_Array;
delete window.cdc_adoQpoasnfa76pfcZLmcfl_Promise;
delete window.cdc_adoQpoasnfa76pfcZLmcfl_Symbol;

// Remove any variables starting with $ that CDP might inject
for (const key in window) {
	if (key.match(/^(\$|cdc_|__webdriver|__driver|__selenium|__fxdriver)/)) {
		try {
			delete window[key];
		} catch (e) {}
	}
}

// ============================================
// FUNCTION TOSTRING SPOOFING
// ============================================

const originalToString = Function.prototype.toString;
const spoofedFunctions = new Set([
	navigator.permissions?.query,
	WebGLRenderingContext.prototype.getParameter,
	HTMLCanvasElement.prototype.toDataURL,
	CanvasRenderingContext2D.prototype.getImageData,
].filter(Boolean));

Function.prototype.toString = function() {
	if (spoofedFunctions.has(this)) {
		return 'function ' + (this.name || '') + '() { [native code] }';
	}
	return originalToString.call(this);
};

// ============================================
// ERROR STACK TRACE CLEANING
// ============================================

const originalErrorPrepareStackTrace = Error.prepareStackTrace;
Error.prepareStackTrace = function(error, structuredStackTrace) {
	const filtered = structuredStackTrace.filter(frame => {
		const fileName = frame.getFileName() || '';
		return !fileName.includes('puppeteer') && 
			   !fileName.includes('chromedp') && 
			   !fileName.includes('playwright') &&
			   !fileName.includes('selenium') &&
			   !fileName.includes('webdriver') &&
			   !fileName.includes('automation');
	});
	if (originalErrorPrepareStackTrace) {
		return originalErrorPrepareStackTrace(error, filtered);
	}
	return filtered.map(f => f.toString()).join('\\n');
};

// ============================================
// MISC PROTECTIONS
// ============================================

// Override Date.prototype.getTimezoneOffset to be consistent
const originalGetTimezoneOffset = Date.prototype.getTimezoneOffset;
Date.prototype.getTimezoneOffset = function() {
	return originalGetTimezoneOffset.call(this);
};

// Make sure Intl returns consistent values
if (typeof Intl !== 'undefined' && Intl.DateTimeFormat) {
	const originalResolvedOptions = Intl.DateTimeFormat.prototype.resolvedOptions;
	Intl.DateTimeFormat.prototype.resolvedOptions = function() {
		const result = originalResolvedOptions.call(this);
		// Ensure consistent timezone reporting
		return result;
	};
}

// Suppress console methods that might reveal automation
['log', 'debug', 'info', 'warn'].forEach(method => {
	const original = console[method];
	console[method] = function(...args) {
		// Filter out automation-related messages
		const str = args.join(' ');
		if (str.includes('DevTools') || str.includes('automation')) {
			return;
		}
		return original.apply(console, args);
	};
});

// Make browser appear more "normal"
Object.defineProperty(document, 'hidden', { get: () => false });
Object.defineProperty(document, 'visibilityState', { get: () => 'visible' });
`

// randomWindowSize returns a randomized common screen resolution
func randomWindowSize() (int, int) {
	sizes := []struct{ w, h int }{
		{1920, 1080},
		{1680, 1050},
		{1536, 864},
		{1440, 900},
		{1366, 768},
		{1280, 800},
		{1280, 720},
		{2560, 1440},
	}
	s := sizes[rand.Intn(len(sizes))]
	return s.w, s.h
}

// generateUserAgent creates a realistic, randomized user agent string
func generateUserAgent() string {
	// Chrome versions (recent stable releases for 2025-2026)
	chromeVersions := []string{"131.0.0.0", "132.0.0.0", "133.0.0.0", "134.0.0.0", "135.0.0.0", "136.0.0.0"}

	// OS configurations
	osConfigs := []struct {
		platform string
		versions []string
	}{
		{
			platform: "Macintosh; Intel Mac OS X",
			versions: []string{"10_15_7", "13_0_0", "14_0_0", "14_5_0", "15_0_0"},
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

	// Comprehensive anti-detection flags
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		// Core anti-detection
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("enable-automation", false),
		chromedp.UserAgent(userAgent),

		// Disable features that reveal automation
		chromedp.Flag("disable-extensions", false),
		chromedp.Flag("disable-default-apps", false),
		chromedp.Flag("disable-component-extensions-with-background-pages", false),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),

		// GPU and rendering (avoid fingerprinting differences)
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("enable-webgl", true),
		chromedp.Flag("enable-3d-apis", true),

		// Randomized window size to avoid fingerprinting (common resolutions)
		chromedp.WindowSize(randomWindowSize()),

		// Exclude switches that reveal automation
		chromedp.ExecPath(""), // Will use default, but we set it to trigger proper detection
	)

	// Remove the ExecPath override we just set (it was a no-op)
	opts = opts[:len(opts)-1]

	if cfg.Headless {
		// Use new headless mode which is harder to detect
		opts = append(opts, chromedp.Flag("headless", "new"))
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	if cfg.UserDataDir != "" {
		opts = append(opts, chromedp.UserDataDir(cfg.UserDataDir))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(logger.Info))

	logger.Info("Browser agent information", "user_agent", userAgent)

	// Initialize browser and inject stealth script
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable page events to inject script on every page load
			if err := page.Enable().Do(ctx); err != nil {
				return err
			}
			// Add script to run before any other scripts on every page
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthScript).Do(ctx)
			return err
		}),
	); err != nil {
		cancel()
		allocCancel()
		return nil, fmt.Errorf("failed to initialize stealth mode: %w", err)
	}

	return &Browser{
		ctx:    ctx,
		cancel: func() { cancel(); allocCancel() },
		logger: logger,
		config: cfg,
	}, nil
}

// randomDelay adds a human-like random delay between actions
func (b *Browser) randomDelay(minMs, maxMs int) {
	delay := time.Duration(minMs+rand.Intn(maxMs-minMs)) * time.Millisecond
	time.Sleep(delay)
}

// humanClickJS simulates a human-like click using JavaScript with mouse event simulation
func (b *Browser) humanClickJS(jsClickCode string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		// Random delay before action (300-800ms)
		time.Sleep(time.Duration(300+rand.Intn(500)) * time.Millisecond)

		// Simulate mouse movement and click with human-like events
		var result bool
		return chromedp.Evaluate(fmt.Sprintf(`
			(function() {
				function simulateHumanClick(element) {
					if (!element) return false;
					
					const rect = element.getBoundingClientRect();
					const x = rect.left + rect.width * (0.3 + Math.random() * 0.4);
					const y = rect.top + rect.height * (0.3 + Math.random() * 0.4);
					
					// Mouse enter
					element.dispatchEvent(new MouseEvent('mouseenter', {
						bubbles: true, cancelable: true, view: window,
						clientX: x, clientY: y
					}));
					
					// Mouse move (simulating approach)
					for (let i = 0; i < 3; i++) {
						element.dispatchEvent(new MouseEvent('mousemove', {
							bubbles: true, cancelable: true, view: window,
							clientX: x + (Math.random() - 0.5) * 5,
							clientY: y + (Math.random() - 0.5) * 5
						}));
					}
					
					// Mouse down
					element.dispatchEvent(new MouseEvent('mousedown', {
						bubbles: true, cancelable: true, view: window,
						button: 0, buttons: 1, clientX: x, clientY: y
					}));
					
					// Focus
					if (element.focus) element.focus();
					
					// Mouse up after small delay
					setTimeout(() => {
						element.dispatchEvent(new MouseEvent('mouseup', {
							bubbles: true, cancelable: true, view: window,
							button: 0, clientX: x, clientY: y
						}));
						
						// Click
						element.dispatchEvent(new MouseEvent('click', {
							bubbles: true, cancelable: true, view: window,
							button: 0, clientX: x, clientY: y
						}));
						
						// Also call click() as fallback
						element.click();
					}, 50 + Math.random() * 100);
					
					return true;
				}
				
				%s
			})()
		`, jsClickCode), &result).Do(ctx)
	}
}

// simulateScrollBehavior adds random scrolling to appear more human
func (b *Browser) simulateScrollBehavior() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		// Random small scroll to simulate reading
		scrollAmount := 50 + rand.Intn(150)
		return chromedp.Evaluate(fmt.Sprintf(`
			window.scrollBy({
				top: %d,
				left: 0,
				behavior: 'smooth'
			});
		`, scrollAmount), nil).Do(ctx)
	}
}

// Close shuts down the browser
func (b *Browser) Close() {
	b.cancel()
}

// IsLoggedIn checks if we have an active Instagram session
func (b *Browser) IsLoggedIn() (bool, error) {
	var loggedIn bool

	// Use randomDelay before navigation to appear more human
	b.randomDelay(500, 1500)

	err := chromedp.Run(b.ctx,
		chromedp.Navigate("https://www.instagram.com/"),
		chromedp.Sleep(time.Duration(2500+rand.Intn(1500))*time.Millisecond),
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

	// Human-like delay before navigation (1-3 seconds)
	b.randomDelay(1000, 3000)

	err := chromedp.Run(b.ctx,
		chromedp.Navigate(profileURL),
		// Random page load wait (2.5-5 seconds)
		chromedp.Sleep(time.Duration(2500+rand.Intn(2500))*time.Millisecond),
		// Simulate scrolling behavior
		b.simulateScrollBehavior(),
		chromedp.Sleep(time.Duration(500+rand.Intn(1000))*time.Millisecond),
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

	// Human-like pause before clicking (like reading the profile)
	b.randomDelay(800, 2000)

	// Click the "Following" button with human-like mouse simulation
	var clicked bool
	err = chromedp.Run(b.ctx,
		b.humanClickJS(`
			// Find the Following button - it contains "Following" text and has a dropdown caret
			let targetElement = null;
			const buttons = document.querySelectorAll('button');
			for (const btn of buttons) {
				if (btn.textContent.includes('Following')) {
					targetElement = btn;
					break;
				}
			}
			if (!targetElement) {
				// Also check div role=button
				const divButtons = document.querySelectorAll('div[role="button"]');
				for (const btn of divButtons) {
					if (btn.textContent.includes('Following')) {
						targetElement = btn;
						break;
					}
				}
			}
			return targetElement ? simulateHumanClick(targetElement) : false;
		`),
		chromedp.Evaluate(`
			(function() {
				const buttons = document.querySelectorAll('button');
				for (const btn of buttons) {
					if (btn.textContent.includes('Following')) return true;
				}
				const divButtons = document.querySelectorAll('div[role="button"]');
				for (const btn of divButtons) {
					if (btn.textContent.includes('Following')) return true;
				}
				return false;
			})()
		`, &clicked),
		// Variable wait for modal (1.5-3.5 seconds)
		chromedp.Sleep(time.Duration(1500+rand.Intn(2000))*time.Millisecond),
	)
	if err != nil {
		return UnfollowError, fmt.Errorf("click following button: %w", err)
	}
	if !clicked {
		return UnfollowError, fmt.Errorf("following button not found for %s", username)
	}

	// Small pause before clicking unfollow (human reading confirmation)
	b.randomDelay(500, 1200)

	// Click "Unfollow" in the modal with human-like simulation
	err = chromedp.Run(b.ctx,
		b.humanClickJS(`
			let targetElement = null;
			// Look for Unfollow button in the modal
			const buttons = document.querySelectorAll('button');
			for (const btn of buttons) {
				if (btn.textContent.trim() === 'Unfollow') {
					targetElement = btn;
					break;
				}
			}
			if (!targetElement) {
				// Also check spans inside buttons
				const spans = document.querySelectorAll('button span, div[role="button"] span');
				for (const span of spans) {
					if (span.textContent.trim() === 'Unfollow') {
						targetElement = span.closest('button, div[role="button"]');
						break;
					}
				}
			}
			return targetElement ? simulateHumanClick(targetElement) : false;
		`),
		// Variable wait for UI update (1.5-3 seconds)
		chromedp.Sleep(time.Duration(1500+rand.Intn(1500))*time.Millisecond),
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
	baseDelay := b.config.UnfollowDelaySeconds

	for i, username := range usernames[:maxCount] {
		result, err := b.Unfollow(username)
		if result != UnfollowSuccess {
			if err != nil {
				b.logger.Error("Failed to unfollow", slog.String("username", username), slog.Any("error", err))
			}
			// Add a small delay even on failures to avoid rapid requests
			b.randomDelay(2000, 5000)
			continue
		}
		successful++

		// Check hourly limit
		if successful >= b.config.MaxPerHour {
			b.logger.Info("Reached hourly limit", slog.Int("count", successful))
			break
		}

		// Variable delay between unfollows (base delay ± 30%)
		if i < maxCount-1 {
			// Add randomness: base delay with ±30% variation
			variation := int(float64(baseDelay) * 0.3)
			actualDelay := baseDelay - variation + rand.Intn(variation*2+1)
			delay := time.Duration(actualDelay) * time.Second

			// Every 5-10 unfollows, take a longer "break" (30-90 seconds extra)
			if successful > 0 && successful%(5+rand.Intn(6)) == 0 {
				extraBreak := time.Duration(30+rand.Intn(60)) * time.Second
				delay += extraBreak
				b.logger.Info("Taking a longer break to appear more natural", slog.Duration("total_delay", delay))
			} else {
				b.logger.Info("Waiting before next unfollow", slog.Duration("delay", delay))
			}
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
