package main

import (
	"archive/zip"
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	defaultBootstrapURL   = "https://github.com/SporaDev/SpermaCore/releases/download/modules/modules.zip"
	officialPublicKeyHex  = "f936d5ba5bfa8ce786f4a88fceec45d3e0b7cb347ec87b8d4f40f0c055627236"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	cmd := strings.ToLower(os.Args[1])
	args := os.Args[2:]

	var err error
	switch cmd {
	case "bootstrap":
		err = runBootstrap(args)
	case "install", "add":
		err = runInstall(args)
	case "build":
		err = runBuild(args)
	case "doctor":
		err = runDoctor()
	case "clean":
		err = runClean()
	case "sign":
		err = runSign(args)
	case "keygen":
		err = runKeygen()
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "scpkg: unknown command '%s'\n", cmd)
		printHelp()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "scpkg: error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("scpkg: package and module manager for spermacore")
	fmt.Println()
	fmt.Println("usage:")
	fmt.Println("  scpkg bootstrap [url]       download official modules, setup .env and compile")
	fmt.Println("  scpkg install <repo>        install a module from github (e.g. user/repo)")
	fmt.Println("  scpkg build [module]        compile modules using cmake and ninja")
	fmt.Println("  scpkg doctor                diagnose system tools and dependencies")
	fmt.Println("  scpkg clean                 remove build artifacts and caches")
	fmt.Println("  scpkg sign <file> -k <key>  sign a package with ed25519")
	fmt.Println("  scpkg keygen                generate ed25519 signing keypair")
}

// bootstrap downloads official modules, configures environment, and builds
func runBootstrap(args []string) error {
	url := defaultBootstrapURL
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		url = strings.TrimSpace(args[0])
	}

	fmt.Println("scpkg: bootstrapping spermacore runtime...")

	// setup .env if missing
	if _, err := os.Stat(".env"); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(".env.example"); err == nil {
			if copyErr := copyFile(".env.example", ".env"); copyErr == nil {
				fmt.Println("scpkg: created .env from template")
			}
		}
	}

	// download modules archive
	tmpZip := filepath.Join(os.TempDir(), fmt.Sprintf("sc-modules-%d.zip", time.Now().UnixNano()))
	defer os.Remove(tmpZip)

	fmt.Printf("scpkg: downloading %s...\n", url)
	if err := downloadWithProgress(url, tmpZip); err != nil {
		return fmt.Errorf("failed to download modules: %w", err)
	}

	// check signature if present
	sigURL := url + ".sig"
	if hasOfficialSignature(sigURL, tmpZip) {
		fmt.Println("scpkg: signature ok, verified from sporadev")
	}

	// extract archive
	fmt.Println("scpkg: unpacking archive...")
	if err := extractSafe(tmpZip, "."); err != nil {
		return fmt.Errorf("failed to unpack: %w", err)
	}
	fmt.Println("scpkg: unpacked into modules/")

	// compile modules if cmake exists
	if _, err := exec.LookPath("cmake"); err == nil {
		fmt.Println("scpkg: building plugins with cmake...")
		if err := buildCMake(""); err != nil {
			fmt.Printf("scpkg: warning: cmake build failed (%v), you can run 'scpkg build' later\n", err)
		} else {
			fmt.Println("scpkg: modules compiled successfully")
		}
	} else {
		fmt.Println("scpkg: note: cmake not found in path, skipping module build")
	}

	fmt.Println("scpkg: ready, start with 'go run ./core'")
	return nil
}

// install downloads a single module from git or url
func runInstall(args []string) error {
	if len(args) < 1 {
		return errors.New("missing module repository (usage: scpkg install <user/repo>)")
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
		fmt.Printf("scpkg: warning: author is not sporadev (%s/%s)\n", owner, repo)
		fmt.Println("scpkg: this is native c++, runs with full memory access")
		if !askConfirm("scpkg: install at your own risk? [y/n]: ") {
			fmt.Println("scpkg: aborted")
			return nil
		}
	}

	tmpZip := filepath.Join(os.TempDir(), fmt.Sprintf("sc-mod-%d.zip", time.Now().UnixNano()))
	defer os.Remove(tmpZip)

	fmt.Printf("scpkg: fetching %s...\n", zipURL)
	if err := downloadWithProgress(zipURL, tmpZip); err != nil {
		// retry with master branch if main failed
		if strings.Contains(zipURL, "refs/heads/main.zip") {
			altURL := strings.Replace(zipURL, "refs/heads/main.zip", "refs/heads/master.zip", 1)
			if retryErr := downloadWithProgress(altURL, tmpZip); retryErr != nil {
				return fmt.Errorf("failed to download module: %w", err)
			}
		} else {
			return fmt.Errorf("failed to download module: %w", err)
		}
	}

	destDir := filepath.Join("modules", repo)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	fmt.Printf("scpkg: unpacking into %s...\n", destDir)
	if err := extractStripTop(tmpZip, destDir); err != nil {
		return fmt.Errorf("failed to extract module: %w", err)
	}

	fmt.Printf("scpkg: building module %s...\n", repo)
	if err := buildCMake(repo); err != nil {
		fmt.Printf("scpkg: build warning: %v\n", err)
	}

	fmt.Printf("scpkg: module '%s' installed\n", repo)
	return nil
}

// build invokes cmake and ninja on modules
func runBuild(args []string) error {
	target := ""
	if len(args) > 0 {
		target = strings.TrimSpace(args[0])
	}
	return buildCMake(target)
}

func buildCMake(target string) error {
	cmakeBin, err := exec.LookPath("cmake")
	if err != nil {
		return errors.New("cmake not found in PATH")
	}

	gen := "Ninja"
	if _, err := exec.LookPath("ninja"); err != nil {
		gen = ""
	}

	cfgArgs := []string{"-B", "build"}
	if gen != "" {
		cfgArgs = append(cfgArgs, "-G", gen)
	}

	// configure step
	cfgCmd := exec.Command(cmakeBin, cfgArgs...)
	cfgCmd.Stdout = os.Stdout
	cfgCmd.Stderr = os.Stderr
	if err := cfgCmd.Run(); err != nil {
		return fmt.Errorf("cmake configure failed: %w", err)
	}

	// build step
	buildArgs := []string{"--build", "build"}
	if target != "" {
		buildArgs = append(buildArgs, "--target", target)
	}

	buildCmd := exec.Command(cmakeBin, buildArgs...)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("cmake build failed: %w", err)
	}

	return nil
}

// doctor checks required tools and files
func runDoctor() error {
	fmt.Println("scpkg: diagnosing environment...")

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
		tdlibName,
		filepath.Join("data", "telegram", tdlibName),
		filepath.Join("releases", tdlibName),
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

// clean removes build caches
func runClean() error {
	fmt.Println("scpkg: cleaning build directories...")
	dirs := []string{"build", ".ninja_deps", ".ninja_log"}
	for _, d := range dirs {
		if err := os.RemoveAll(d); err == nil {
			fmt.Printf("scpkg: removed %s\n", d)
		}
	}

	// clean build folders inside modules
	filepath.Walk("modules", func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if info.Name() == "build" {
			os.RemoveAll(path)
			fmt.Printf("scpkg: removed %s\n", path)
			return filepath.SkipDir
		}
		return nil
	})

	fmt.Println("scpkg: clean completed")
	return nil
}

// sign generates an ed25519 signature for a file
func runSign(args []string) error {
	if len(args) < 3 || args[1] != "-k" && args[1] != "-key" {
		return errors.New("usage: scpkg sign <file> -k <private_key_hex_file>")
	}

	filePath := args[0]
	keyPath := args[2]

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	keyHex := strings.TrimSpace(string(keyData))
	privKeyBytes, err := hex.DecodeString(keyHex)
	if err != nil || len(privKeyBytes) != ed25519.PrivateKeySize {
		return errors.New("invalid ed25519 private key length")
	}

	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read target file: %w", err)
	}

	// sign sha256 hash of file
	hash := sha256.Sum256(fileBytes)
	sig := ed25519.Sign(privKeyBytes, hash[:])

	sigPath := filePath + ".sig"
	sigHex := hex.EncodeToString(sig)
	if err := os.WriteFile(sigPath, []byte(sigHex+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write signature: %w", err)
	}

	fmt.Printf("scpkg: signed %s -> %s\n", filePath, sigPath)
	return nil
}

// keygen generates a new ed25519 keypair
func runKeygen() error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	privHex := hex.EncodeToString(priv)
	pubHex := hex.EncodeToString(pub)

	keyFile := "sc-private.key"
	if err := os.WriteFile(keyFile, []byte(privHex+"\n"), 0600); err != nil {
		return err
	}

	fmt.Printf("scpkg: generated private key: %s (keep secret!)\n", keyFile)
	fmt.Printf("scpkg: public key: %s\n", pubHex)
	return nil
}

// downloadWithProgress streams url body into destination file
func downloadWithProgress(url, dst string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s", resp.Status)
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	// stream bytes
	buf := make([]byte, 32*1024)
	var downloaded int64
	total := resp.ContentLength

	for {
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			nw, ew := out.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("short write")
				}
			}
			downloaded += int64(nw)
			if ew != nil {
				return ew
			}
		}
		if er != nil {
			if er != io.EOF {
				return er
			}
			break
		}
	}

	if total > 0 && downloaded != total {
		return fmt.Errorf("incomplete download: got %d of %d bytes", downloaded, total)
	}

	return nil
}

// extractSafe extracts zip while preventing zip slip path traversal
func extractSafe(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		cleanName := filepath.Clean(f.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			continue
		}

		targetPath := filepath.Join(destDir, cleanName)
		if f.FileInfo().IsDir() {
			os.MkdirAll(targetPath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// extractStripTop strips root directory of github archive when extracting
func extractStripTop(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		parts := strings.Split(filepath.ToSlash(f.Name), "/")
		if len(parts) <= 1 {
			continue
		}
		// skip top level directory created by github
		relPath := filepath.Join(parts[1:]...)
		cleanName := filepath.Clean(relPath)
		if strings.HasPrefix(cleanName, "..") {
			continue
		}

		targetPath := filepath.Join(destDir, cleanName)
		if f.FileInfo().IsDir() {
			os.MkdirAll(targetPath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// hasOfficialSignature checks ed25519 signature from remote or local .sig
func hasOfficialSignature(sigURL, zipPath string) bool {
	pubBytes, err := hex.DecodeString(officialPublicKeyHex)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return false
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(sigURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	defer resp.Body.Close()

	sigData, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	sigHex := strings.TrimSpace(string(sigData))
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return false
	}

	fileBytes, err := os.ReadFile(zipPath)
	if err != nil {
		return false
	}

	hash := sha256.Sum256(fileBytes)
	return ed25519.Verify(pubBytes, hash[:], sigBytes)
}

func askConfirm(prompt string) bool {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	text = strings.ToLower(strings.TrimSpace(text))
	return text == "y" || text == "yes"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
