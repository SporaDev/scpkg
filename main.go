package main

import (
	"fmt"
	"os"
	"strings"
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
	case "sc-new-path", "set-path":
		err = runNewPath(args)
	case "sign":
		err = runSign(args)
	case "keygen":
		err = runKeygen()
	case "help", "-h", "--help":
		printHelp()
	default:
		logError("unknown command '%s'", cmd)
		printHelp()
		os.Exit(1)
	}

	if err != nil {
		logError("%v", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("scpkg: package and module manager for spermacore")
	fmt.Println()
	fmt.Println("usage:")
	fmt.Println("  scpkg bootstrap [url]         fetch modules, setup .env and compile")
	fmt.Println("  scpkg install <repo>          install a module from github (e.g. user/repo)")
	fmt.Println("  scpkg build [module]          compile modules using cmake and ninja")
	fmt.Println("  scpkg doctor                  diagnose system tools and dependencies")
	fmt.Println("  scpkg clean                   remove build artifacts and caches")
	fmt.Println("  scpkg sc-new-path [path]      set or update default spermacore root path")
	fmt.Println("  scpkg sign <file> -k <key>    sign a package with ed25519")
	fmt.Println("  scpkg keygen                  generate ed25519 signing keypair")
}
