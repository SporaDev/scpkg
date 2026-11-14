package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultMainRepoIndex = "https://raw.githubusercontent.com/SporaDev/scpkg-mainrepo/main/index.json"

type registryIndex struct {
	Modules map[string]struct {
		Version     string `json:"version"`
		Description string `json:"description"`
		Author      string `json:"author"`
		URL         string `json:"url"`
	} `json:"modules"`
}

func fetchRegistryModule(name string) (string, error) {
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Get(defaultMainRepoIndex)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry returned %d", resp.StatusCode)
	}

	var reg registryIndex
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return "", err
	}

	entry, ok := reg.Modules[strings.ToLower(name)]
	if !ok || entry.URL == "" {
		return "", fmt.Errorf("module '%s' not found in registry", name)
	}
	return entry.URL, nil
}

// runInstall fetches, verifies signatures, and compiles a module
func runInstall(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: scpkg install <name|user/repo|url|local_path>")
	}

	rootDir, err := resolveRootPath(true)
	if err != nil {
		return err
	}

	target := strings.TrimSpace(args[0])
	var owner, repo, zipURL string
	var localSourceDir string

	// 1. Check local directory
	if fi, err := os.Stat(target); err == nil && fi.IsDir() {
		localSourceDir, _ = filepath.Abs(target)
		repo = detectModuleName(localSourceDir)
		owner = "local"
	}

	// 2. Check bare module name
	if localSourceDir == "" && !strings.Contains(target, "/") && !strings.Contains(target, "\\") {
		// Try remote scpkg-mainrepo first
		if regURL, regErr := fetchRegistryModule(target); regErr == nil && regURL != "" {
			zipURL = regURL
			repo = target
			owner = "scpkg-mainrepo"
		} else {
			// Fallback to local SpermaCore roots
			candidates := []string{
				filepath.Join("..", "spermacore-clean", "modules", target),
				filepath.Join("..", "spermacore", "modules", target),
			}
			if home, err := os.UserHomeDir(); err == nil {
				candidates = append(candidates,
					filepath.Join(home, "Documents", "spermacore-clean", "modules", target),
					filepath.Join(home, "Documents", "spermacore", "modules", target),
				)
			}
			for _, c := range candidates {
				if fi, err := os.Stat(c); err == nil && fi.IsDir() {
					localSourceDir, _ = filepath.Abs(c)
					repo = detectModuleName(localSourceDir)
					owner = "spermacore"
					break
				}
			}
			if localSourceDir == "" {
				return fmt.Errorf("module '%s' not found in registry or local candidates", target)
			}
		}
	}

	// 3. Remote URL or GitHub slug
	if localSourceDir == "" && zipURL == "" {
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			zipURL = target
			parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(target, "https://"), "http://"), "/")
			if len(parts) >= 3 && parts[0] == "github.com" {
				owner = parts[1]
				repo = strings.TrimSuffix(parts[2], ".git")
			} else {
				repo = filepath.Base(zipURL)
				owner = "remote"
			}
		} else {
			parts := strings.Split(target, "/")
			if len(parts) == 2 {
				owner = parts[0]
				repo = strings.TrimSuffix(parts[1], ".git")
				zipURL = fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/main.zip", owner, repo)
			} else {
				return fmt.Errorf("unrecognized format '%s'", target)
			}
		}
	}

	tmpStageDir := filepath.Join(os.TempDir(), fmt.Sprintf("sc-mod-stage-%d", time.Now().UnixNano()))
	defer os.RemoveAll(tmpStageDir)
	_ = os.MkdirAll(tmpStageDir, 0755)

	if localSourceDir != "" {
		logInfo("staging module from: %s", localSourceDir)
		if err := copyDir(localSourceDir, tmpStageDir); err != nil {
			return err
		}
	} else {
		tmpZip := filepath.Join(os.TempDir(), fmt.Sprintf("sc-mod-%d.zip", time.Now().UnixNano()))
		defer os.Remove(tmpZip)

		logInfo("fetching module %s/%s", owner, repo)
		if err := downloadWithProgress(zipURL, tmpZip, repo); err != nil {
			if strings.Contains(zipURL, "refs/heads/main.zip") {
				alt := strings.Replace(zipURL, "refs/heads/main.zip", "refs/heads/master.zip", 1)
				if rErr := downloadWithProgress(alt, tmpZip, repo); rErr != nil {
					return fmt.Errorf("download failed: %w", err)
				}
			} else {
				return fmt.Errorf("download failed: %w", err)
			}
		}

		s := newSpinner(fmt.Sprintf("unpacking module %s...", repo))
		s.start()
		if err := extractStripTop(tmpZip, tmpStageDir); err != nil {
			s.stopWithFail("extraction failed")
			return err
		}
		s.stopWithSuccess(fmt.Sprintf("unpacked module %s", repo))
	}

	// 4. Verify cryptographic signature
	res := VerifyModuleDir(tmpStageDir)
	if res.Module != "" {
		repo = res.Module
	}

	switch res.Status {
	case StatusOfficial:
		logSuccess("VERIFIED OFFICIAL MODULE: '%s'", repo)
		logInfo("  • Developer:  %s", res.Author)
		logInfo("  • Key:        %s", res.PublicKey)
		logInfo("  • Files:      %d source files intact", res.Files)

	case StatusCommunity:
		printUnofficialWarning(res, repo, false)
		if !askConfirm("scpkg: do you want to install this UNOFFICIAL module? [y/N]: ") {
			logInfo("installation cancelled by user")
			return nil
		}
		logWarning("proceeding with community module installation...")

	case StatusUnsigned:
		printUnofficialWarning(res, repo, true)
		if !askConfirm("scpkg: do you want to install this UNSIGNED module? [y/N]: ") {
			logInfo("installation cancelled by user")
			return nil
		}
		logWarning("proceeding with unsigned module installation...")

	case StatusTampered:
		printTamperAlert(res, repo)
		return fmt.Errorf("security check failed for '%s': %w", repo, res.Error)
	}

	// 5. Deploy to destination
	destDir := filepath.Join(rootDir, "modules", repo)
	_ = os.MkdirAll(destDir, 0755)

	s := newSpinner(fmt.Sprintf("installing into modules/%s...", repo))
	s.start()
	if err := copyDir(tmpStageDir, destDir); err != nil {
		s.stopWithFail("install failed")
		return err
	}
	s.stopWithSuccess(fmt.Sprintf("installed into modules/%s", repo))

	// 6. Build
	logInfo("building module %s...", repo)
	if err := buildCMake(rootDir, repo); err != nil {
		logWarning("build warning: %v (run 'scpkg build %s' later)", err, repo)
	}

	logSuccess("module '%s' installed successfully", repo)
	return nil
}

func printUnofficialWarning(res *ModuleVerificationResult, repo string, unsigned bool) {
	fmt.Println()
	fmt.Println("\033[33m" + strings.Repeat("=", 64))
	fmt.Println("                ⚠️   SECURITY WARNING   ⚠️")
	fmt.Println("            UNOFFICIAL MODULE DETECTED")
	fmt.Println(strings.Repeat("=", 64) + "\033[0m")
	fmt.Printf("[!] Module:    '%s'\n", repo)
	if unsigned {
		fmt.Println("[!] Status:    UNSIGNED MODULE (NO SIGNATURE)")
	} else {
		fmt.Println("[!] Status:    COMMUNITY SIGNED (NOT AN OFFICIAL DEVELOPER)")
		fmt.Printf("[!] Author:    %s\n", res.Author)
		fmt.Printf("[!] Key:       %s\n", res.PublicKey)
	}
	fmt.Println()
	fmt.Println("\033[33m  NOTICE: Native C++ modules run with full system access")
	fmt.Println("  (memory, filesystem, network). Unofficial modules are unvetted.")
	fmt.Println(strings.Repeat("=", 64) + "\033[0m")
	fmt.Println()
}

func printTamperAlert(res *ModuleVerificationResult, repo string) {
	fmt.Println()
	fmt.Println("\033[31m" + strings.Repeat("=", 64))
	fmt.Println("            ⛔  CRITICAL SECURITY ALERT  ⛔")
	fmt.Println("           SIGNATURE VERIFICATION FAILED")
	fmt.Println(strings.Repeat("=", 64) + "\033[0m")
	fmt.Printf("[✗] Module '%s' signature verification failed!\n", repo)
	fmt.Printf("[✗] Reason: %v\n", res.Error)
	fmt.Println("  Files were modified or corrupted. Installation is blocked.")
	fmt.Println("\033[31m" + strings.Repeat("=", 64) + "\033[0m")
	fmt.Println()
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return nil
		}
		targetPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		return copyFile(path, targetPath)
	})
}

func askConfirm(prompt string) bool {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	text = strings.ToLower(strings.TrimSpace(text))
	return text == "y" || text == "yes"
}
