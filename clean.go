package main

import (
	"os"
	"path/filepath"
)

// runClean cleans build folders and caches
func runClean() error {
	rootDir, err := resolveRootPath(false)
	if err != nil {
		return err
	}

	logInfo("cleaning build directories in %s...", rootDir)

	dirs := []string{
		filepath.Join(rootDir, "build"),
		filepath.Join(rootDir, ".ninja_deps"),
		filepath.Join(rootDir, ".ninja_log"),
	}

	for _, d := range dirs {
		if err := os.RemoveAll(d); err == nil {
			logInfo("removed %s", d)
		}
	}

	// clean build folders inside modules
	modulesDir := filepath.Join(rootDir, "modules")
	_ = filepath.Walk(modulesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if info.Name() == "build" {
			_ = os.RemoveAll(path)
			logInfo("removed %s", path)
			return filepath.SkipDir
		}
		return nil
	})

	logSuccess("clean completed")
	return nil
}
