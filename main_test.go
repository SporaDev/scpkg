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

func TestDownloadModules(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	tmpFile := filepath.Join(os.TempDir(), "test-modules-download.zip")
	defer os.Remove(tmpFile)

	err := downloadWithProgress(defaultBootstrapURL, tmpFile)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}

	if info.Size() < 50000 {
		t.Fatalf("downloaded file too small: %d bytes", info.Size())
	}
}

func TestSignAndVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}

	tmpFile := filepath.Join(os.TempDir(), "test-data.txt")
	testData := []byte("hello spermacore")
	os.WriteFile(tmpFile, testData, 0644)
	defer os.Remove(tmpFile)

	hash := sha256.Sum256(testData)
	sig := ed25519.Sign(priv, hash[:])

	if !ed25519.Verify(pub, hash[:], sig) {
		t.Fatalf("ed25519 verification failed")
	}

	// test tamper detection
	tamperedHash := sha256.Sum256([]byte("tampered data"))
	if ed25519.Verify(pub, tamperedHash[:], sig) {
		t.Fatalf("verification should have failed on tampered data")
	}

	pubHex := hex.EncodeToString(pub)
	if len(pubHex) != 64 {
		t.Fatalf("expected 64 char pubhex, got %d", len(pubHex))
	}
}
