package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type scpkgConfig struct {
	RootPath string `json:"root_path"`
}

func configFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".scpkg")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func loadConfig() (*scpkgConfig, error) {
	cfgFile, err := configFilePath()
	if err != nil {
		return &scpkgConfig{}, err
	}

	data, err := os.ReadFile(cfgFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &scpkgConfig{}, nil
		}
		return &scpkgConfig{}, err
	}

	var cfg scpkgConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &scpkgConfig{}, nil
	}
	return &cfg, nil
}

func saveConfig(cfg *scpkgConfig) error {
	cfgFile, err := configFilePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgFile, data, 0644)
}

// resolveRootPath gets configured spermacore path or prompts user
func resolveRootPath(interactive bool) (string, error) {
	// check env override first
	if envPath := os.Getenv("SPERMACORE_PATH"); envPath != "" {
		return filepath.Clean(envPath), nil
	}

	cfg, _ := loadConfig()
	if cfg.RootPath != "" {
		if info, err := os.Stat(cfg.RootPath); err == nil && info.IsDir() {
			return cfg.RootPath, nil
		}
	}

	// if not interactive or current dir looks like spermacore, default to current
	if !interactive {
		return ".", nil
	}

	// check if current directory is already spermacore
	if _, err := os.Stat("core"); err == nil {
		if _, err := os.Stat("config.toml"); err == nil {
			abs, _ := filepath.Abs(".")
			cfg.RootPath = abs
			_ = saveConfig(cfg)
			return abs, nil
		}
	}

	fmt.Print("scpkg: enter path to spermacore root directory [default: .]: ")
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	input := strings.TrimSpace(text)

	if input == "" {
		input = "."
	}

	absPath, err := filepath.Abs(input)
	if err != nil {
		return input, nil
	}

	cfg.RootPath = absPath
	if err := saveConfig(cfg); err == nil {
		logSuccess("saved default root path: %s", absPath)
		logInfo("you can change it anytime with 'scpkg sc-new-path'")
	}

	return absPath, nil
}

// runNewPath updates the default spermacore root directory
func runNewPath(args []string) error {
	cfg, _ := loadConfig()

	var newPath string
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		newPath = strings.TrimSpace(args[0])
	} else {
		if cfg.RootPath != "" {
			fmt.Printf("scpkg: current spermacore root is: %s\n", cfg.RootPath)
		}
		fmt.Print("scpkg: enter new spermacore root path: ")
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		newPath = strings.TrimSpace(text)
	}

	if newPath == "" {
		return errors.New("empty path provided")
	}

	absPath, err := filepath.Abs(newPath)
	if err != nil {
		return err
	}

	if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
		logWarning("directory '%s' does not exist yet", absPath)
	}

	cfg.RootPath = absPath
	if err := saveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	logSuccess("spermacore root updated to: %s", absPath)
	return nil
}
