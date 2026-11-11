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

const officialPublicKeyHex = "86d8d030eceb77abfb9c61e07c518da66687986820697f4bb86132309cfceb27"

// hasOfficialSignature verifies ed25519 signature from remote or local .sig
func hasOfficialSignature(sigURL, zipPath string) bool {
	pubBytes, err := hex.DecodeString(officialPublicKeyHex)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return false
	}

	var sigBytes []byte

	// try remote signature first
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

	// fallback to local .sig if remote not available
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
	return ed25519.Verify(pubBytes, hash[:], sigBytes)
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
	// derive 32-byte seed from passphrase
	seed := sha256.Sum256([]byte(keyStr))
	return ed25519.NewKeyFromSeed(seed[:]), nil
}

// runSign signs a file with an ed25519 private key or passphrase
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

	privKey, err := derivePrivateKey(keyData)
	if err != nil {
		return fmt.Errorf("invalid key: %w", err)
	}

	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read target file: %w", err)
	}

	hash := sha256.Sum256(fileBytes)
	sig := ed25519.Sign(privKey, hash[:])

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
