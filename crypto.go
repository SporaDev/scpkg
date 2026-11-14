package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	ModuleSigFormat         = "scpkg-module-sig/v1"
	ModuleSigFileName       = "module.sig"
	officialPublicKeyHex    = "86d8d030eceb77abfb9c61e07c518da66687986820697f4bb86132309cfceb27"
	officialDevPublicKeyHex = "46003198c63f61520d99f5bd2615975f794a5d80d3f20172576f7a0b01e77902"
)

type SignatureStatus string

const (
	StatusOfficial  SignatureStatus = "OFFICIAL"
	StatusCommunity SignatureStatus = "COMMUNITY"
	StatusUnsigned  SignatureStatus = "UNSIGNED"
	StatusTampered  SignatureStatus = "TAMPERED"
)

type ModuleSignature struct {
	Format    string            `json:"format"`
	Module    string            `json:"module"`
	Author    string            `json:"author"`
	SignedAt  string            `json:"signed_at"`
	Algorithm string            `json:"algorithm"`
	RootHash  string            `json:"root_hash"`
	Files     map[string]string `json:"files"`
	PublicKey string            `json:"public_key"`
	Signature string            `json:"signature"`
}

type ModuleVerificationResult struct {
	Status    SignatureStatus
	Module    string
	Author    string
	PublicKey string
	SignedAt  string
	Files     int
	Error     error
}

func isOfficialPublicKey(pubHex string) bool {
	clean := strings.ToLower(strings.TrimSpace(pubHex))
	if clean == officialPublicKeyHex || clean == officialDevPublicKeyHex {
		return true
	}
	if extra := os.Getenv("SCPKG_TRUSTED_KEYS"); extra != "" {
		for _, k := range strings.Split(extra, ",") {
			if strings.EqualFold(strings.TrimSpace(k), clean) {
				return true
			}
		}
	}
	if single := os.Getenv("SCPKG_OFFICIAL_KEY"); single != "" {
		if strings.EqualFold(strings.TrimSpace(single), clean) {
			return true
		}
	}
	return false
}

func shouldIncludeInModuleSignature(relPath string, isDir bool) bool {
	norm := filepath.ToSlash(relPath)
	parts := strings.Split(norm, "/")

	for _, p := range parts {
		switch strings.ToLower(p) {
		case "build", ".git", ".cache", ".idea", ".vscode", "node_modules", "cmakefiles":
			return false
		}
		if strings.HasPrefix(strings.ToLower(p), "cmake-build-") {
			return false
		}
	}

	if isDir {
		return true
	}

	base := strings.ToLower(filepath.Base(norm))
	if base == ModuleSigFileName || base == "signature.json" || base == ".signature" || strings.HasSuffix(base, ".sig") {
		return false
	}

	switch base {
	case ".gitignore", ".gitattributes", ".clang-format", ".ds_store", "thumbs.db":
		return false
	}

	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".dll", ".so", ".dylib", ".exe", ".obj", ".o", ".a", ".lib",
		".exp", ".ilk", ".pdb", ".tmp", ".bak", ".swp", ".zip", ".tar", ".gz":
		return false
	}

	return true
}

func computeModuleManifest(moduleDir string) (map[string]string, string, error) {
	files := make(map[string]string)
	var relPaths []string

	err := filepath.Walk(moduleDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(moduleDir, path)
		if relErr != nil || rel == "." {
			return nil
		}
		if !shouldIncludeInModuleSignature(rel, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}

		normRel := filepath.ToSlash(rel)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", normRel, readErr)
		}

		h := sha256.Sum256(data)
		hashHex := hex.EncodeToString(h[:])
		files[normRel] = hashHex
		relPaths = append(relPaths, normRel)
		return nil
	})

	if err != nil {
		return nil, "", err
	}
	if len(files) == 0 {
		return nil, "", fmt.Errorf("no source files found in '%s'", moduleDir)
	}

	sort.Strings(relPaths)
	var manifestBuf strings.Builder
	for _, p := range relPaths {
		manifestBuf.WriteString(p)
		manifestBuf.WriteString(":")
		manifestBuf.WriteString(files[p])
		manifestBuf.WriteString("\n")
	}

	rootSum := sha256.Sum256([]byte(manifestBuf.String()))
	return files, hex.EncodeToString(rootSum[:]), nil
}

func computeCanonicalPayload(format, module, author, rootHash, pubKeyHex string) [32]byte {
	var payload strings.Builder
	payload.WriteString(format)
	payload.WriteString("\nmodule:")
	payload.WriteString(strings.TrimSpace(module))
	payload.WriteString("\nauthor:")
	payload.WriteString(strings.TrimSpace(author))
	payload.WriteString("\nroothash:")
	payload.WriteString(strings.TrimSpace(rootHash))
	payload.WriteString("\npubkey:")
	payload.WriteString(strings.ToLower(strings.TrimSpace(pubKeyHex)))
	payload.WriteString("\n")

	return sha256.Sum256([]byte(payload.String()))
}

func detectModuleName(moduleDir string) string {
	aboutPath := filepath.Join(moduleDir, "about.toml")
	if data, err := os.ReadFile(aboutPath); err == nil {
		re := regexp.MustCompile(`(?m)^\s*name\s*=\s*["']([^"']+)["']`)
		if matches := re.FindSubmatch(data); len(matches) > 1 {
			return string(matches[1])
		}
	}
	return filepath.Base(moduleDir)
}

func SignModule(moduleDir string, privKey ed25519.PrivateKey, author string) (*ModuleSignature, error) {
	if abs, err := filepath.Abs(moduleDir); err == nil {
		moduleDir = abs
	}
	modName := detectModuleName(moduleDir)
	if strings.TrimSpace(author) == "" {
		author = "SporaDev Official"
	}

	files, rootHash, err := computeModuleManifest(moduleDir)
	if err != nil {
		return nil, err
	}

	pubKey := privKey.Public().(ed25519.PublicKey)
	pubKeyHex := hex.EncodeToString(pubKey)

	payloadHash := computeCanonicalPayload(ModuleSigFormat, modName, author, rootHash, pubKeyHex)
	sigBytes := ed25519.Sign(privKey, payloadHash[:])
	sigHex := hex.EncodeToString(sigBytes)

	sig := &ModuleSignature{
		Format:    ModuleSigFormat,
		Module:    modName,
		Author:    author,
		SignedAt:  time.Now().UTC().Format(time.RFC3339),
		Algorithm: "ed25519-sha256",
		RootHash:  rootHash,
		Files:     files,
		PublicKey: pubKeyHex,
		Signature: sigHex,
	}

	sigData, err := json.MarshalIndent(sig, "", "  ")
	if err != nil {
		return nil, err
	}

	sigPath := filepath.Join(moduleDir, ModuleSigFileName)
	if err := os.WriteFile(sigPath, append(sigData, '\n'), 0644); err != nil {
		return nil, err
	}
	return sig, nil
}

func VerifyModuleDir(moduleDir string) *ModuleVerificationResult {
	if abs, err := filepath.Abs(moduleDir); err == nil {
		moduleDir = abs
	}

	modName := detectModuleName(moduleDir)
	sigPath := filepath.Join(moduleDir, ModuleSigFileName)

	data, err := os.ReadFile(sigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &ModuleVerificationResult{
				Status: StatusUnsigned,
				Module: modName,
				Error:  errors.New("module.sig missing"),
			}
		}
		return &ModuleVerificationResult{
			Status: StatusTampered,
			Module: modName,
			Error:  err,
		}
	}

	var sig ModuleSignature
	if err := json.Unmarshal(data, &sig); err != nil {
		return &ModuleVerificationResult{
			Status: StatusTampered,
			Module: modName,
			Error:  err,
		}
	}

	if sig.Format != ModuleSigFormat {
		return &ModuleVerificationResult{
			Status: StatusTampered,
			Module: sig.Module,
			Error:  fmt.Errorf("unsupported format %q", sig.Format),
		}
	}

	diskFiles, diskRootHash, err := computeModuleManifest(moduleDir)
	if err != nil {
		return &ModuleVerificationResult{
			Status: StatusTampered,
			Module: sig.Module,
			Author: sig.Author,
			Error:  err,
		}
	}

	if len(diskFiles) != len(sig.Files) {
		return &ModuleVerificationResult{
			Status:    StatusTampered,
			Module:    sig.Module,
			Author:    sig.Author,
			PublicKey: sig.PublicKey,
			Error:     fmt.Errorf("file count mismatch (%d != %d)", len(sig.Files), len(diskFiles)),
		}
	}

	for p, h := range sig.Files {
		diskHash, exists := diskFiles[p]
		if !exists {
			return &ModuleVerificationResult{
				Status:    StatusTampered,
				Module:    sig.Module,
				Author:    sig.Author,
				PublicKey: sig.PublicKey,
				Error:     fmt.Errorf("missing file: %s", p),
			}
		}
		if !strings.EqualFold(diskHash, h) {
			return &ModuleVerificationResult{
				Status:    StatusTampered,
				Module:    sig.Module,
				Author:    sig.Author,
				PublicKey: sig.PublicKey,
				Error:     fmt.Errorf("content modified: %s", p),
			}
		}
	}

	if !strings.EqualFold(diskRootHash, sig.RootHash) {
		return &ModuleVerificationResult{
			Status:    StatusTampered,
			Module:    sig.Module,
			Author:    sig.Author,
			PublicKey: sig.PublicKey,
			Error:     errors.New("root hash mismatch"),
		}
	}

	pubBytes, err := hex.DecodeString(sig.PublicKey)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return &ModuleVerificationResult{
			Status: StatusTampered,
			Module: sig.Module,
			Error:  errors.New("invalid public key"),
		}
	}

	sigBytes, err := hex.DecodeString(sig.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return &ModuleVerificationResult{
			Status: StatusTampered,
			Module: sig.Module,
			Error:  errors.New("invalid signature length"),
		}
	}

	payloadHash := computeCanonicalPayload(sig.Format, sig.Module, sig.Author, sig.RootHash, sig.PublicKey)
	if !ed25519.Verify(pubBytes, payloadHash[:], sigBytes) {
		return &ModuleVerificationResult{
			Status:    StatusTampered,
			Module:    sig.Module,
			Author:    sig.Author,
			PublicKey: sig.PublicKey,
			Error:     errors.New("invalid signature"),
		}
	}

	status := StatusCommunity
	if isOfficialPublicKey(sig.PublicKey) {
		status = StatusOfficial
	}

	return &ModuleVerificationResult{
		Status:    status,
		Module:    sig.Module,
		Author:    sig.Author,
		PublicKey: sig.PublicKey,
		SignedAt:  sig.SignedAt,
		Files:     len(diskFiles),
	}
}

func derivePrivateKey(keyData []byte) (ed25519.PrivateKey, error) {
	keyStr := strings.TrimSpace(string(keyData))
	if len(keyStr) == 128 {
		if b, err := hex.DecodeString(keyStr); err == nil && len(b) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(b), nil
		}
	}
	if len(keyStr) == 64 {
		if b, err := hex.DecodeString(keyStr); err == nil && len(b) == ed25519.SeedSize {
			return ed25519.NewKeyFromSeed(b), nil
		}
	}
	seed := sha256.Sum256([]byte(keyStr))
	return ed25519.NewKeyFromSeed(seed[:]), nil
}

func resolvePrivateKey(keyFlag string) (ed25519.PrivateKey, error) {
	if keyFlag != "" {
		data, err := os.ReadFile(keyFlag)
		if err != nil {
			return nil, err
		}
		return derivePrivateKey(data)
	}

	if envKey := os.Getenv("SCPKG_SIGN_KEY"); envKey != "" {
		if data, err := os.ReadFile(envKey); err == nil {
			return derivePrivateKey(data)
		}
		return derivePrivateKey([]byte(envKey))
	}

	candidates := []string{"sc-private.key", "sc-official-dev.key"}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".scpkg", "developer.key"),
			filepath.Join(home, ".scpkg", "sc-private.key"),
			filepath.Join(home, "Documents", "scpkg", "sc-private.key"),
		)
	}

	for _, cand := range candidates {
		if data, err := os.ReadFile(cand); err == nil {
			return derivePrivateKey(data)
		}
	}
	return nil, errors.New("private key not found")
}

func runSign(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: scpkg sign <file|module_dir> [-k key] [-author name]")
	}

	targetPath := args[0]
	var keyPath string
	author := "SporaDev Official"

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-k", "-key":
			if i+1 < len(args) {
				keyPath = args[i+1]
				i++
			}
		case "-author", "-a":
			if i+1 < len(args) {
				author = args[i+1]
				i++
			}
		}
	}

	privKey, err := resolvePrivateKey(keyPath)
	if err != nil {
		return err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		sig, err := SignModule(targetPath, privKey, author)
		if err != nil {
			return err
		}
		pubKey := privKey.Public().(ed25519.PublicKey)
		pubHex := hex.EncodeToString(pubKey)

		logSuccess("signed module '%s' (%d files)", sig.Module, len(sig.Files))
		logInfo("author:     %s", sig.Author)
		logInfo("public key: %s (official: %v)", pubHex, isOfficialPublicKey(pubHex))
		return nil
	}

	fileBytes, err := os.ReadFile(targetPath)
	if err != nil {
		return err
	}

	hash := sha256.Sum256(fileBytes)
	sig := ed25519.Sign(privKey, hash[:])

	sigPath := targetPath + ".sig"
	sigHex := hex.EncodeToString(sig)
	if err := os.WriteFile(sigPath, []byte(sigHex+"\n"), 0644); err != nil {
		return err
	}

	logSuccess("signed %s -> %s", targetPath, sigPath)
	return nil
}

func runSignModules(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: scpkg sign-modules <modules_dir> [-k key] [-author name]")
	}

	targetDir := args[0]
	var keyPath string
	author := "SporaDev Official"

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-k", "-key":
			if i+1 < len(args) {
				keyPath = args[i+1]
				i++
			}
		case "-author", "-a":
			if i+1 < len(args) {
				author = args[i+1]
				i++
			}
		}
	}

	privKey, err := resolvePrivateKey(keyPath)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}

	count := 0
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		modDir := filepath.Join(targetDir, e.Name())

		_, errAbout := os.Stat(filepath.Join(modDir, "about.toml"))
		_, errSrc := os.Stat(filepath.Join(modDir, "src"))
		if errAbout != nil && errSrc != nil {
			continue
		}

		sig, err := SignModule(modDir, privKey, author)
		if err != nil {
			logWarning("failed to sign module %s: %v", e.Name(), err)
			continue
		}
		logSuccess("signed module '%s' (%d source files)", sig.Module, len(sig.Files))
		count++
	}

	pubKey := privKey.Public().(ed25519.PublicKey)
	pubHex := hex.EncodeToString(pubKey)
	logSuccess("batch signed %d modules with key %s", count, pubHex)
	return nil
}

func runVerify(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: scpkg verify <file|module_dir>")
	}

	targetPath := args[0]
	info, err := os.Stat(targetPath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		res := VerifyModuleDir(targetPath)
		printVerificationCard(res)
		if res.Status == StatusTampered {
			return errors.New("verification failed")
		}
		return nil
	}

	sigPath := targetPath + ".sig"
	data, err := os.ReadFile(sigPath)
	if err != nil {
		return err
	}
	sigBytes, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return errors.New("invalid signature encoding")
	}

	fileBytes, err := os.ReadFile(targetPath)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(fileBytes)

	for _, kHex := range []string{officialPublicKeyHex, officialDevPublicKeyHex} {
		pubBytes, _ := hex.DecodeString(kHex)
		if len(pubBytes) == ed25519.PublicKeySize && ed25519.Verify(pubBytes, hash[:], sigBytes) {
			logSuccess("file '%s' verified (%s)", targetPath, kHex)
			return nil
		}
	}

	logError("file '%s' failed signature verification", targetPath)
	return errors.New("signature verification failed")
}

func printVerificationCard(res *ModuleVerificationResult) {
	switch res.Status {
	case StatusOfficial:
		logSuccess("module '%s': official (author: %s, files: %d)", res.Module, res.Author, res.Files)
	case StatusCommunity:
		logWarning("module '%s': community signed (author: %s)", res.Module, res.Author)
	case StatusUnsigned:
		logWarning("module '%s': unsigned", res.Module)
	case StatusTampered:
		logError("module '%s': integrity check failed (%v)", res.Module, res.Error)
	}
}

func runKeygen(args []string) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	privHex := hex.EncodeToString(priv)
	pubHex := hex.EncodeToString(pub)

	keyFile := "sc-private.key"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		keyFile = strings.TrimSpace(args[0])
	}

	if err := os.WriteFile(keyFile, []byte(privHex+"\n"), 0600); err != nil {
		return err
	}

	logSuccess("generated developer private key: %s (keep secret)", keyFile)
	logInfo("developer public key: %s", pubHex)
	return nil
}

func hasOfficialSignature(sigURL, zipPath string) bool {
	var sigBytes []byte

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(sigURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		sigData, err := io.ReadAll(resp.Body)
		if err == nil {
			sigHex := strings.TrimSpace(string(sigData))
			sigBytes, _ = hex.DecodeString(sigHex)
		}
	}

	if len(sigBytes) == 0 {
		localSig := zipPath + ".sig"
		if data, err := os.ReadFile(localSig); err == nil {
			sigHex := strings.TrimSpace(string(data))
			sigBytes, _ = hex.DecodeString(sigHex)
		}
	}

	if len(sigBytes) != ed25519.SignatureSize {
		return false
	}

	fileBytes, err := os.ReadFile(zipPath)
	if err != nil {
		return false
	}

	hash := sha256.Sum256(fileBytes)

	for _, kHex := range []string{officialPublicKeyHex, officialDevPublicKeyHex} {
		pubBytes, err := hex.DecodeString(kHex)
		if err == nil && len(pubBytes) == ed25519.PublicKeySize {
			if ed25519.Verify(pubBytes, hash[:], sigBytes) {
				return true
			}
		}
	}

	return false
}
