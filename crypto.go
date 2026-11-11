package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const officialPublicKeyHex = "f936d5ba5bfa8ce786f4a88fceec45d3e0b7cb347ec87b8d4f40f0c055627236"

// hasOfficialSignature verifies ed25519 signature from remote or local .sig
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

// runSign signs a file with an ed25519 private key
func runSign(args []string) error {
	if len(args) < 3 || (args[1] != "-k" && args[1] != "-key") {
		return errors.New("usage: scpkg sign <file> -k <private_key_file>")
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

	hash := sha256.Sum256(fileBytes)
	sig := ed25519.Sign(privKeyBytes, hash[:])

	sigPath := filePath + ".sig"
	sigHex := hex.EncodeToString(sig)
	if err := os.WriteFile(sigPath, []byte(sigHex+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write signature: %w", err)
	}

	logSuccess("signed %s -> %s", filePath, sigPath)
	return nil
}

// runKeygen creates an ed25519 keypair
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

	logSuccess("generated private key: %s (keep secret)", keyFile)
	logInfo("public key: %s", pubHex)
	return nil
}
