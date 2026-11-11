package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// runDoctor diagnoses tools and environment
func runDoctor() error {
	logInfo("diagnosing environment...")

	rootDir, _ := resolveRootPath(false)
	fmt.Printf("[✓] spermacore root: %s\n", rootDir)

	// check go
	fmt.Printf("[✓] go runtime: %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	// check cmake
	if p, err := exec.LookPath("cmake"); err == nil {
		fmt.Printf("[✓] cmake: %s\n", p)
	} else {
		fmt.Println("[!] cmake: not found (required for building c++ modules)")
	}

	// check c++ compiler
	compilers := []string{"g++", "clang++", "cl"}
	foundCompiler := false
	for _, c := range compilers {
		if p, err := exec.LookPath(c); err == nil {
			fmt.Printf("[✓] c++ compiler: %s (%s)\n", c, p)
			foundCompiler = true
			break
		}
	}
	if !foundCompiler {
		fmt.Println("[!] c++ compiler: not found (install gcc, clang, or msvc)")
	}

	// check ninja
	if p, err := exec.LookPath("ninja"); err == nil {
		fmt.Printf("[✓] ninja build: %s\n", p)
	} else {
		fmt.Println("[-] ninja: not found (optional, improves build speed)")
	}

	// check tdlib binary
	tdlibName := "tdjson.dll"
	if runtime.GOOS != "windows" {
		tdlibName = "libtdjson.so"
	}
	tdlibPaths := []string{
		filepath.Join(rootDir, tdlibName),
		filepath.Join(rootDir, "data", "telegram", tdlibName),
		filepath.Join(rootDir, "releases", tdlibName),
	}
	tdlibFound := false
	for _, p := range tdlibPaths {
		if _, err := os.Stat(p); err == nil {
			fmt.Printf("[✓] tdlib binary: found at %s\n", p)
			tdlibFound = true
			break
		}
	}
	if !tdlibFound {
		fmt.Printf("[!] tdlib binary: %s not found in data/telegram/ or root\n", tdlibName)
	}

	// check port 8080
	ln, err := net.Listen("tcp", "127.0.0.1:8080")
	if err == nil {
		ln.Close()
		fmt.Println("[✓] port 8080: available for webui")
	} else {
		fmt.Println("[!] port 8080: currently in use")
	}

	return nil
}
