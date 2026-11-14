package main

import (
	"crypto/ed25519"
	"encoding/hex"
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
	if isSpermaCoreRoot(rootDir) {
		fmt.Printf("[✓] spermacore root: %s (verified)\n", rootDir)
	} else {
		fmt.Printf("[!] spermacore root: %s (warning: missing core/ or CMakeLists.txt)\n", rootDir)
	}

	// check modules directory and signature integrity
	hasMod, modPath, count, names := findModules(rootDir)
	if hasMod && count > 0 {
		fmt.Printf("[✓] modules folder: found at %s (%d modules)\n", modPath, count)
		for _, name := range names {
			subDir := filepath.Join(modPath, name)
			res := VerifyModuleDir(subDir)
			switch res.Status {
			case StatusOfficial:
				fmt.Printf("    • %-14s %s[✓] official%s (signed by %s)\n", name, colorGreen, colorReset, res.Author)
			case StatusCommunity:
				fmt.Printf("    • %-14s %s[!] unofficial%s (community signed: %s)\n", name, colorYellow, colorReset, res.Author)
			case StatusUnsigned:
				fmt.Printf("    • %-14s %s[-] unsigned%s (no signature)\n", name, colorDim, colorReset)
			case StatusTampered:
				fmt.Printf("    • %-14s %s[✗] tampered%s (%v)\n", name, colorRed, colorReset, res.Error)
			}
		}
	} else if hasMod {
		fmt.Printf("[-] modules folder: found at %s (empty, run 'scpkg bootstrap' to fetch)\n", modPath)
	} else {
		fmt.Printf("[!] modules folder: not found in %s (run 'scpkg bootstrap' to fetch)\n", rootDir)
	}

	// check developer signing key
	if priv, err := resolvePrivateKey(""); err == nil {
		pub := priv.Public().(ed25519.PublicKey)
		pubHex := hex.EncodeToString(pub)
		fmt.Printf("[✓] developer signing key: loaded (%s..., official: %v)\n", pubHex[:16], isOfficialPublicKey(pubHex))
	} else {
		fmt.Printf("[-] developer signing key: not configured (optional, run 'scpkg keygen' for signing modules)\n")
	}

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
