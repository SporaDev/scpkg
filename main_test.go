package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestDoctor(t *testing.T) {
	err := runDoctor()
	if err != nil {
		t.Fatalf("runDoctor failed: %v", err)
	}
}

func TestConfigPath(t *testing.T) {
	cfg := &scpkgConfig{RootPath: "test-path"}
	err := saveConfig(cfg)
	if err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if loaded.RootPath != "test-path" {
		t.Fatalf("expected test-path, got %s", loaded.RootPath)
	}
}

func TestSignAndVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}

	tmpFile := filepath.Join(os.TempDir(), "test-data.txt")
	testData := []byte("hello spermacore")
	_ = os.WriteFile(tmpFile, testData, 0644)
	defer os.Remove(tmpFile)

	hash := sha256.Sum256(testData)
	sig := ed25519.Sign(priv, hash[:])

	if !ed25519.Verify(pub, hash[:], sig) {
		t.Fatalf("ed25519 verification failed")
	}

	tamperedHash := sha256.Sum256([]byte("tampered data"))
	if ed25519.Verify(pub, tamperedHash[:], sig) {
		t.Fatalf("verification should fail on tampered data")
	}

	pubHex := hex.EncodeToString(pub)
	if len(pubHex) != 64 {
		t.Fatalf("expected 64 char hex, got %d", len(pubHex))
	}
}

func TestSpermaCoreValidation(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "sc-test-repo")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	if isSpermaCoreRoot(tmpDir) {
		t.Fatalf("empty/nonexistent dir should not be spermacore root")
	}

	_ = os.MkdirAll(filepath.Join(tmpDir, "core"), 0755)
	if isSpermaCoreRoot(tmpDir) {
		t.Fatalf("dir missing CMakeLists.txt / config.toml should not be spermacore root")
	}

	_ = os.WriteFile(filepath.Join(tmpDir, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.16)"), 0644)
	if !isSpermaCoreRoot(tmpDir) {
		t.Fatalf("valid dir with core/ and CMakeLists.txt should be recognized as spermacore root")
	}
}

func TestModulesSearch(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "sc-test-modules")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	exists, _, count, _ := findModules(tmpDir)
	if exists || count != 0 {
		t.Fatalf("nonexistent modules dir should return false, 0")
	}

	modDir := filepath.Join(tmpDir, "modules")
	_ = os.MkdirAll(filepath.Join(modDir, "telegram"), 0755)
	_ = os.MkdirAll(filepath.Join(modDir, "websearch"), 0755)

	exists, _, count, names := findModules(tmpDir)
	if !exists || count != 2 {
		t.Fatalf("expected 2 modules, got %d (exists=%v)", count, exists)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %v", names)
	}
}

func TestLiveOfficialSignature(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live signature test in short mode")
	}

	tmpFile := filepath.Join(os.TempDir(), "live-modules.zip")
	defer os.Remove(tmpFile)

	err := downloadWithProgress(defaultBootstrapURL, tmpFile, "testing download")
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	sigURL := defaultBootstrapURL + ".sig"
	if !hasOfficialSignature(sigURL, tmpFile) {
		t.Fatalf("live official signature verification failed")
	}
}
