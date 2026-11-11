package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runInstall fetches and compiles a module from github
func runInstall(args []string) error {
	if len(args) < 1 {
		return errors.New("missing repository argument (usage: scpkg install <user/repo>)")
	}

	rootDir, err := resolveRootPath(true)
	if err != nil {
		return err
	}

	target := strings.TrimSpace(args[0])
	var owner, repo, zipURL string

	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		zipURL = target
		parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(target, "https://"), "http://"), "/")
		if len(parts) >= 3 && parts[0] == "github.com" {
			owner = parts[1]
			repo = strings.TrimSuffix(parts[2], ".git")
		}
	} else {
		parts := strings.Split(target, "/")
		if len(parts) == 2 {
			owner = parts[0]
			repo = strings.TrimSuffix(parts[1], ".git")
			zipURL = fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/main.zip", owner, repo)
		} else {
			return fmt.Errorf("unrecognized repository format '%s'", target)
		}
	}

	// warn if author is not sporadev
	if !strings.EqualFold(owner, "sporadev") {
		logWarning("author is not sporadev (github.com/%s/%s)", owner, repo)
		logWarning("this is native c++, runs with full memory access")
		if !askConfirm("scpkg: install at your own risk? [y/n]: ") {
			logInfo("installation aborted by user")
			return nil
		}
	}

	tmpZip := filepath.Join(os.TempDir(), fmt.Sprintf("sc-mod-%d.zip", time.Now().UnixNano()))
	defer os.Remove(tmpZip)

	logInfo("fetching module %s/%s", owner, repo)
	if err := downloadWithProgress(zipURL, tmpZip, repo); err != nil {
		// retry with master branch
		if strings.Contains(zipURL, "refs/heads/main.zip") {
			altURL := strings.Replace(zipURL, "refs/heads/main.zip", "refs/heads/master.zip", 1)
			if retryErr := downloadWithProgress(altURL, tmpZip, repo); retryErr != nil {
				return fmt.Errorf("failed to download: %w", err)
			}
		} else {
			return fmt.Errorf("failed to download: %w", err)
		}
	}

	destDir := filepath.Join(rootDir, "modules", repo)
	_ = os.MkdirAll(destDir, 0755)

	s := newSpinner(fmt.Sprintf("extracting into modules/%s...", repo))
	s.start()
	if err := extractStripTop(tmpZip, destDir); err != nil {
		s.stopWithFail("extraction failed")
		return fmt.Errorf("extraction error: %w", err)
	}
	s.stopWithSuccess(fmt.Sprintf("extracted into modules/%s", repo))

	// attempt build
	logInfo("building module %s...", repo)
	if err := buildCMake(rootDir, repo); err != nil {
		logWarning("build warning: %v", err)
	}

	logSuccess("module '%s' installed successfully", repo)
	return nil
}

func askConfirm(prompt string) bool {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	text = strings.ToLower(strings.TrimSpace(text))
	return text == "y" || text == "yes"
}
