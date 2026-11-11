package main

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
)

// runBuild compiles plugins using cmake
func runBuild(args []string) error {
	rootDir, err := resolveRootPath(false)
	if err != nil {
		return err
	}

	target := ""
	if len(args) > 0 {
		target = args[0]
	}
	return buildCMake(rootDir, target)
}

func buildCMake(rootDir, target string) error {
	cmakeBin, err := exec.LookPath("cmake")
	if err != nil {
		return errors.New("cmake not found in PATH")
	}

	gen := "Ninja"
	if _, err := exec.LookPath("ninja"); err != nil {
		gen = ""
	}

	buildDir := filepath.Join(rootDir, "build")
	cfgArgs := []string{"-B", buildDir, "-S", rootDir}
	if gen != "" {
		cfgArgs = append(cfgArgs, "-G", gen)
	}

	s := newSpinner("configuring cmake build system...")
	s.start()

	cfgCmd := exec.Command(cmakeBin, cfgArgs...)
	cfgCmd.Dir = rootDir
	out, err := cfgCmd.CombinedOutput()
	if err != nil {
		s.stopWithFail("cmake configuration failed")
		fmt.Println(string(out))
		return fmt.Errorf("cmake configure error: %w", err)
	}
	s.stopWithSuccess("cmake configured")

	s = newSpinner("compiling plugins...")
	s.start()

	buildArgs := []string{"--build", buildDir}
	if target != "" {
		buildArgs = append(buildArgs, "--target", target)
	}

	buildCmd := exec.Command(cmakeBin, buildArgs...)
	buildCmd.Dir = rootDir
	out, err = buildCmd.CombinedOutput()
	if err != nil {
		s.stopWithFail("compilation failed")
		fmt.Println(string(out))
		return fmt.Errorf("cmake build error: %w", err)
	}

	s.stopWithSuccess("compilation finished successfully")
	return nil
}
