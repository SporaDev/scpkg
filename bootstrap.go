package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultBootstrapURL = "https://github.com/SporaDev/SpermaCore/releases/download/modules/modules.zip"

// runBootstrap bootstraps spermacore workspace
func runBootstrap(args []string) error {
	logInfo("starting spermacore bootstrap...")

	rootDir, err := resolveRootPath(true)
	if err != nil {
		return err
	}

	if isSpermaCoreRoot(rootDir) {
		logSuccess("spermacore root verified: %s", rootDir)
	} else {
		logWarning("target directory '%s' does not appear to be a SpermaCore root", rootDir)
	}

	hasMod, modPath, count, _ := findModules(rootDir)
	if hasMod && count > 0 {
		logInfo("detected existing modules directory with %d modules, updating...", count)
	} else {
		logInfo("modules directory not found, will be created at %s", modPath)
	}

	url := defaultBootstrapURL
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		url = strings.TrimSpace(args[0])
	}

	// copy .env if missing
	envPath := filepath.Join(rootDir, ".env")
	envExamplePath := filepath.Join(rootDir, ".env.example")
	if _, err := os.Stat(envPath); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(envExamplePath); err == nil {
			if copyErr := copyFile(envExamplePath, envPath); copyErr == nil {
				logSuccess("created .env from .env.example")
			}
		}
	}

	// download archive to temporary file
	tmpZip := filepath.Join(os.TempDir(), fmt.Sprintf("sc-modules-%d.zip", time.Now().UnixNano()))
	defer os.Remove(tmpZip)

	logInfo("fetching modules from %s", url)
	if err := downloadWithProgress(url, tmpZip, "downloading modules"); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// verify signature
	s := newSpinner("verifying ed25519 signature...")
	s.start()
	sigURL := url + ".sig"
	if hasOfficialSignature(sigURL, tmpZip) {
		s.stopWithSuccess("signature verified, author is sporadev")
	} else {
		s.stopWithSuccess("modules downloaded (no remote signature)")
	}

	// unpack archive
	s = newSpinner("unpacking into modules/...")
	s.start()
	_ = os.MkdirAll(modPath, 0755)

	if err := extractSafe(tmpZip, rootDir); err != nil {
		s.stopWithFail("extraction failed")
		return fmt.Errorf("extraction error: %w", err)
	}
	s.stopWithSuccess("modules unpacked successfully")

	// build modules
	if _, err := exec.LookPath("cmake"); err == nil {
		if err := buildCMake(rootDir, ""); err != nil {
			logWarning("cmake build failed (%v), run 'scpkg build' later", err)
		}
	} else {
		logWarning("cmake not found in PATH, skipping automatic build")
	}

	logSuccess("bootstrap complete! run 'go run ./core' in %s", rootDir)
	return nil
}
