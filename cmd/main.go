package main

import (
	"flag"
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"
)

type config struct {
	Instagram instagramConfig `yaml:"instagram"`
	App       appConfig       `yaml:"app"`
}

type instagramConfig struct {
	AutomationLimits automationLimits `yaml:"automation_limits"`
	Operations       operations       `yaml:"operations"`
}

type automationLimits struct {
	Actions rateLimit `yaml:"actions"`
}

type rateLimit struct {
	Hourly            int `yaml:"hourly"`
	Daily             int `yaml:"daily"`
	TimeWindowSeconds int `yaml:"time_window_seconds"`
}

type operations struct {
	Follow   operationLimit `yaml:"follow"`
	Unfollow operationLimit `yaml:"unfollow"`
}

type operationLimit struct {
	MaxPerHour int `yaml:"max_per_hour"`
	MaxPerDay  int `yaml:"max_per_day"`
}

type appConfig struct {
	Version              string        `yaml:"version"`
	ExtractedPath        string        `yaml:"extracted_path"`
	UnfollowDelaySeconds int           `yaml:"unfollow_delay_seconds"`
	MaxRetries           int           `yaml:"max_retries"`
	BackoffMultiplier    int           `yaml:"backoff_multiplier"`
	SafetyBufferPercent  int           `yaml:"safety_buffer_percent"`
	Session              sessionConfig `yaml:"session"`
}

type sessionConfig struct {
	MaxActionsPerSession       int  `yaml:"max_actions_per_session"`
	SessionRestartDelayMinutes int  `yaml:"session_restart_delay_minutes"`
	RandomizeHeaders           bool `yaml:"randomize_headers"`
}

type application struct {
	config config
	logger *slog.Logger
}

func getFlags() (string, string, bool) {
	var configPath string
	var dataPath string
	var runUnfollow bool
	flag.StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	flag.StringVar(&dataPath, "data", "", "Path to Instagram export zip file")
	flag.BoolVar(&runUnfollow, "unfollow", false, "Run the unfollow process")
	flag.Parse()

	if configPath != "" {
		return configPath, dataPath, runUnfollow
	}

	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		return envPath, dataPath, runUnfollow
	}

	return "config.yaml", dataPath, runUnfollow
}

func loadConfig(path string) (*config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	configPath, dataPath, doUnfollow := getFlags()
	cfg, err := loadConfig(configPath)
	if err != nil {
		logger.Error("Failed to load config", slog.String("path", configPath), slog.Any("error", err))
		os.Exit(1)
	}

	app := &application{
		config: *cfg,
		logger: logger,
	}

	app.logger.Info("Application started", slog.String("configPath", configPath))

	if dataPath != "" {
		dest, err := app.unzipData(dataPath)
		if err != nil {
			app.logger.Error("Failed to unzip data", slog.String("dataPath", dataPath), slog.Any("error", err))
			os.Exit(1)
		}
		app.logger.Info("Data unzipped", slog.String("destDir", dest))

		if err := app.parseToDB(dest); err != nil {
			app.logger.Error("Failed to parse and import data", slog.Any("error", err))
			os.Exit(1)
		}
	}

	if doUnfollow {
		if err := app.runUnfollow(); err != nil {
			app.logger.Error("Unfollow process failed", slog.Any("error", err))
			os.Exit(1)
		}
	}
}
