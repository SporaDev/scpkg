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

// isSpermaCoreRoot verifies if a directory looks like a valid SpermaCore repository
func isSpermaCoreRoot(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}

	coreDir := filepath.Join(path, "core")
	if cInfo, err := os.Stat(coreDir); err != nil || !cInfo.IsDir() {
		return false
	}

	cmakeFile := filepath.Join(path, "CMakeLists.txt")
	tomlFile := filepath.Join(path, "config.toml")
	_, errCmake := os.Stat(cmakeFile)
	_, errToml := os.Stat(tomlFile)
	return errCmake == nil || errToml == nil
}

// findModules inspects root directory for modules folder and lists contained modules
func findModules(rootDir string) (exists bool, modPath string, count int, names []string) {
	modPath = filepath.Join(rootDir, "modules")
	info, err := os.Stat(modPath)
	if err != nil || !info.IsDir() {
		return false, modPath, 0, nil
	}

	entries, err := os.ReadDir(modPath)
	if err != nil {
		return true, modPath, 0, nil
	}

	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			count++
			names = append(names, e.Name())
		}
	}
	return true, modPath, count, names
}

// findSpermaCoreCandidates searches common locations for SpermaCore roots
func findSpermaCoreCandidates() []string {
	var candidates []string
	seen := make(map[string]bool)

	addCandidate := func(p string) {
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			return
		}
		if isSpermaCoreRoot(abs) {
			seen[abs] = true
			candidates = append(candidates, abs)
		}
	}

	addCandidate(".")
	addCandidate("..")
	addCandidate("../spermacore")
	addCandidate("../spermacore-clean")

	if home, err := os.UserHomeDir(); err == nil {
		addCandidate(filepath.Join(home, "Documents", "spermacore"))
		addCandidate(filepath.Join(home, "Documents", "spermacore-clean"))
		addCandidate(filepath.Join(home, "spermacore"))
		addCandidate(filepath.Join(home, "spermacore-clean"))
	}

	return candidates
}

// resolveRootPath gets configured spermacore path, searches candidates, or prompts user
func resolveRootPath(interactive bool) (string, error) {
	// 1. check env override first
	if envPath := os.Getenv("SPERMACORE_PATH"); envPath != "" {
		clean := filepath.Clean(envPath)
		if isSpermaCoreRoot(clean) {
			return clean, nil
		}
	}

	// 2. check saved config
	cfg, _ := loadConfig()
	if cfg.RootPath != "" && isSpermaCoreRoot(cfg.RootPath) {
		return cfg.RootPath, nil
	}

	// 3. check current directory
	if isSpermaCoreRoot(".") {
		abs, _ := filepath.Abs(".")
		cfg.RootPath = abs
		_ = saveConfig(cfg)
		return abs, nil
	}

	// 4. search candidate paths
	candidates := findSpermaCoreCandidates()
	defaultPath := "."
	if len(candidates) > 0 {
		defaultPath = candidates[0]
	}

	if !interactive {
		if len(candidates) > 0 {
			cfg.RootPath = defaultPath
			_ = saveConfig(cfg)
			return defaultPath, nil
		}
		return ".", nil
	}

	fmt.Printf("scpkg: enter path to spermacore root directory [default: %s]: ", defaultPath)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	input := strings.TrimSpace(text)

	if input == "" {
		input = defaultPath
	}

	absPath, err := filepath.Abs(input)
	if err != nil {
		return input, nil
	}

	if !isSpermaCoreRoot(absPath) {
		logWarning("'%s' does not appear to be a valid SpermaCore root (missing core/ or CMakeLists.txt)", absPath)
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
		candidates := findSpermaCoreCandidates()
		defHint := ""
		if len(candidates) > 0 {
			defHint = fmt.Sprintf(" [detected: %s]", candidates[0])
		}
		fmt.Printf("scpkg: enter new spermacore root path%s: ", defHint)
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		newPath = strings.TrimSpace(text)
		if newPath == "" && len(candidates) > 0 {
			newPath = candidates[0]
		}
	}

	if newPath == "" {
		return errors.New("empty path provided")
	}

	absPath, err := filepath.Abs(newPath)
	if err != nil {
		return err
	}

	if !isSpermaCoreRoot(absPath) {
		logWarning("directory '%s' does not appear to be a SpermaCore root (missing core/ or CMakeLists.txt)", absPath)
	} else {
		logSuccess("verified SpermaCore root at: %s", absPath)
	}

	exists, modPath, count, names := findModules(absPath)
	if exists && count > 0 {
		logSuccess("found modules directory at %s (%d modules: %s)", modPath, count, strings.Join(names, ", "))
	} else if exists {
		logWarning("modules directory exists at %s but is empty (run 'scpkg bootstrap' to fetch)", modPath)
	} else {
		logWarning("modules directory not found in '%s' (run 'scpkg bootstrap' to install)", absPath)
	}

	cfg.RootPath = absPath
	if err := saveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	logSuccess("spermacore root updated to: %s", absPath)
	return nil
}
