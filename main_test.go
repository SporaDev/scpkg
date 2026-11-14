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
	origCfg, _ := loadConfig()
	defer func() {
		if origCfg != nil {
			_ = saveConfig(origCfg)
		}
	}()

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

func TestModuleSignAndVerifyOfficial(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "sc-test-mod-official")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	_ = os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "about.toml"), []byte("name = \"testmod\"\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "testmod.cpp"), []byte("#include <stdio.h>\n"), 0644)

	// Use official developer key
	devPriv, err := resolvePrivateKey("")
	if err != nil {
		t.Fatalf("failed to resolve dev key: %v", err)
	}

	sig, err := SignModule(tmpDir, devPriv, "SporaDev Official")
	if err != nil {
		t.Fatalf("SignModule failed: %v", err)
	}

	if sig.Module != "testmod" {
		t.Fatalf("expected module testmod, got %s", sig.Module)
	}

	res := VerifyModuleDir(tmpDir)
	if res.Status != StatusOfficial {
		t.Fatalf("expected StatusOfficial, got %s (err: %v)", res.Status, res.Error)
	}
	if res.Files != 2 {
		t.Fatalf("expected 2 files verified, got %d", res.Files)
	}
}

func TestModuleExcludeBuildArtifacts(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "sc-test-mod-build")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	_ = os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "about.toml"), []byte("name = \"buildtest\"\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "main.cpp"), []byte("int main(){return 0;}\n"), 0644)

	devPriv, err := resolvePrivateKey("")
	if err != nil {
		t.Fatalf("resolvePrivateKey failed: %v", err)
	}

	_, err = SignModule(tmpDir, devPriv, "SporaDev Official")
	if err != nil {
		t.Fatalf("SignModule failed: %v", err)
	}

	// Now simulate a CMake build generating build/ directory and .dll / .obj
	buildDir := filepath.Join(tmpDir, "build")
	_ = os.MkdirAll(filepath.Join(buildDir, "cmake"), 0755)
	_ = os.WriteFile(filepath.Join(buildDir, "buildtest.dll"), []byte("binary dll content"), 0644)
	_ = os.WriteFile(filepath.Join(buildDir, "buildtest.obj"), []byte("binary obj content"), 0644)

	// Verification should still pass 100% because build/ is canonically excluded!
	res := VerifyModuleDir(tmpDir)
	if res.Status != StatusOfficial {
		t.Fatalf("expected StatusOfficial despite build artifacts, got %s (err: %v)", res.Status, res.Error)
	}
}

func TestModuleTamperDetection(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "sc-test-mod-tamper")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	_ = os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "about.toml"), []byte("name = \"tampertest\"\n"), 0644)
	cppPath := filepath.Join(tmpDir, "src", "main.cpp")
	_ = os.WriteFile(cppPath, []byte("int original();\n"), 0644)

	devPriv, err := resolvePrivateKey("")
	if err != nil {
		t.Fatalf("resolvePrivateKey failed: %v", err)
	}

	_, err = SignModule(tmpDir, devPriv, "SporaDev Official")
	if err != nil {
		t.Fatalf("SignModule failed: %v", err)
	}

	// Modify file
	_ = os.WriteFile(cppPath, []byte("int tampered();\n"), 0644)

	res := VerifyModuleDir(tmpDir)
	if res.Status != StatusTampered {
		t.Fatalf("expected StatusTampered after modifying source, got %s", res.Status)
	}
}

func TestModuleExtraFileDetection(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "sc-test-mod-extra")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	_ = os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "about.toml"), []byte("name = \"extratest\"\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "main.cpp"), []byte("int ok();\n"), 0644)

	devPriv, err := resolvePrivateKey("")
	if err != nil {
		t.Fatalf("resolvePrivateKey failed: %v", err)
	}

	_, err = SignModule(tmpDir, devPriv, "SporaDev Official")
	if err != nil {
		t.Fatalf("SignModule failed: %v", err)
	}

	// Add unauthorized extra source file
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "injected.cpp"), []byte("malicious injected code;\n"), 0644)

	res := VerifyModuleDir(tmpDir)
	if res.Status != StatusTampered {
		t.Fatalf("expected StatusTampered after injecting extra file, got %s", res.Status)
	}
}

func TestUnofficialCommunitySignature(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "sc-test-mod-community")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	_ = os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "about.toml"), []byte("name = \"communitymod\"\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "comm.cpp"), []byte("void hello();\n"), 0644)

	// Generate a third-party / community keypair
	_, commPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}

	_, err = SignModule(tmpDir, commPriv, "Random Community Dev")
	if err != nil {
		t.Fatalf("SignModule failed: %v", err)
	}

	res := VerifyModuleDir(tmpDir)
	if res.Status != StatusCommunity {
		t.Fatalf("expected StatusCommunity for unofficial signer, got %s", res.Status)
	}
}

func TestUnsignedModule(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "sc-test-mod-unsigned")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	_ = os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "about.toml"), []byte("name = \"unsignedmod\"\n"), 0644)

	res := VerifyModuleDir(tmpDir)
	if res.Status != StatusUnsigned {
		t.Fatalf("expected StatusUnsigned for module without module.sig, got %s", res.Status)
	}
}

func TestInitAndPack(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "sc-test-init-pack")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)
	_ = os.MkdirAll(tmpDir, 0755)

	modDir := filepath.Join(tmpDir, "samplemod")
	if err := runInit([]string{modDir}); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	aboutPath := filepath.Join(modDir, "about.toml")
	if _, err := os.Stat(aboutPath); err != nil {
		t.Fatalf("about.toml missing after init")
	}

	zipPath := filepath.Join(tmpDir, "samplemod.zip")
	if err := runPack([]string{modDir, "-o", zipPath}); err != nil {
		t.Fatalf("runPack failed: %v", err)
	}

	if _, err := os.Stat(zipPath); err != nil {
		t.Fatalf("zip missing after pack")
	}

	res := VerifyModuleDir(modDir)
	if res.Status != StatusOfficial {
		t.Fatalf("expected StatusOfficial after pack, got %s", res.Status)
	}
}

func TestPackUnsigned(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "sc-test-pack-nosign")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)
	_ = os.MkdirAll(tmpDir, 0755)

	modDir := filepath.Join(tmpDir, "usermod")
	if err := runInit([]string{modDir}); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	zipPath := filepath.Join(tmpDir, "usermod.zip")
	if err := runPack([]string{modDir, "--no-sign", "-o", zipPath}); err != nil {
		t.Fatalf("runPack failed: %v", err)
	}

	if _, err := os.Stat(zipPath); err != nil {
		t.Fatalf("zip missing after pack")
	}

	res := VerifyModuleDir(modDir)
	if res.Status != StatusUnsigned {
		t.Fatalf("expected StatusUnsigned with --no-sign, got %s", res.Status)
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
