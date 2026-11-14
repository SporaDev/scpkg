package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func runInit(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: scpkg init <module_name>")
	}

	target := strings.TrimSpace(args[0])
	name := strings.ToLower(filepath.Base(target))
	modDir := target

	if err := os.MkdirAll(filepath.Join(modDir, "src"), 0755); err != nil {
		return err
	}

	aboutPath := filepath.Join(modDir, "about.toml")
	if _, err := os.Stat(aboutPath); errors.Is(err, os.ErrNotExist) {
		aboutContent := fmt.Sprintf(`name = "%s"
version = "1.0.0"
author = "YourName"
human_description = "%s module for SpermaCore"
llm_description = "%s plugin tools and functions"
`, name, name, name)
		if err := os.WriteFile(aboutPath, []byte(aboutContent), 0644); err != nil {
			return err
		}
	}

	cppPath := filepath.Join(modDir, "src", name+".cpp")
	if _, err := os.Stat(cppPath); errors.Is(err, os.ErrNotExist) {
		cppContent := fmt.Sprintf(`#include "plugin_api.h"

extern "C" SC_EXPORT int sc_plugin_init(void* ctx) {
    (void)ctx;
    return 0;
}
`)
		if err := os.WriteFile(cppPath, []byte(cppContent), 0644); err != nil {
			return err
		}
	}

	gitIgnore := filepath.Join(modDir, ".gitignore")
	if _, err := os.Stat(gitIgnore); errors.Is(err, os.ErrNotExist) {
		_ = os.WriteFile(gitIgnore, []byte("build/\n*.dll\n*.so\n*.obj\n"), 0644)
	}

	logSuccess("initialized module '%s' at %s", name, modDir)
	logInfo("edit %s/about.toml to set descriptions", modDir)
	return nil
}

func detectModuleAuthor(moduleDir string) string {
	aboutPath := filepath.Join(moduleDir, "about.toml")
	if data, err := os.ReadFile(aboutPath); err == nil {
		re := regexp.MustCompile(`(?m)^\s*author\s*=\s*["']([^"']+)["']`)
		if matches := re.FindSubmatch(data); len(matches) > 1 {
			return string(matches[1])
		}
	}
	return "Community Author"
}

func runPack(args []string) error {
	dir := "."
	var keyPath string
	var outZip string
	var author string
	noSign := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-k", "-key":
			if i+1 < len(args) {
				keyPath = args[i+1]
				i++
			}
		case "-o", "-out":
			if i+1 < len(args) {
				outZip = args[i+1]
				i++
			}
		case "-author", "-a":
			if i+1 < len(args) {
				author = args[i+1]
				i++
			}
		case "--no-sign", "-no-sign", "--unsigned", "-unsigned":
			noSign = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				dir = args[i]
			}
		}
	}

	aboutPath := filepath.Join(dir, "about.toml")
	if _, err := os.Stat(aboutPath); err != nil {
		return fmt.Errorf("about.toml not found in '%s' (run 'scpkg init <name>' first)", dir)
	}

	modName := detectModuleName(dir)
	if author == "" {
		author = detectModuleAuthor(dir)
	}
	if outZip == "" {
		outZip = modName + ".zip"
	}

	if !noSign {
		privKey, err := resolvePrivateKey(keyPath)
		if err == nil {
			sig, err := SignModule(dir, privKey, author)
			if err != nil {
				return fmt.Errorf("signing failed: %w", err)
			}
			logSuccess("signed module '%s' (author: %s)", modName, sig.Author)
		} else {
			logWarning("private key not found; packing unsigned module")
		}
	} else {
		logInfo("packing unsigned module (--no-sign)")
	}

	if err := createModuleZip(dir, outZip); err != nil {
		return fmt.Errorf("pack zip error: %w", err)
	}
	logSuccess("packed module archive -> %s", outZip)

	type entry struct {
		Version     string `json:"version"`
		Description string `json:"description"`
		Author      string `json:"author"`
		URL         string `json:"url"`
	}
	snip, _ := json.MarshalIndent(map[string]entry{
		modName: {
			Version:     "1.0.0",
			Description: fmt.Sprintf("%s plugin", modName),
			Author:      author,
			URL:         fmt.Sprintf("https://github.com/SporaDev/scpkg-mainrepo/releases/download/modules/%s", filepath.Base(outZip)),
		},
	}, "", "  ")

	fmt.Println()
	logInfo("index.json snippet for scpkg-mainrepo:")
	fmt.Println(string(snip))
	return nil
}
