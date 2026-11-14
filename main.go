package main

import (
	"fmt"
	"os"
	"strings"
)

const Version = "1.1.0"

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
	case "init":
		err = runInit(args)
	case "pack", "publish":
		err = runPack(args)
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
	case "sign-modules", "sign-all":
		err = runSignModules(args)
	case "verify", "check":
		err = runVerify(args)
	case "keygen":
		err = runKeygen(args)
	case "version", "-v", "--version":
		fmt.Printf("scpkg version %s (SpermaCore Package Manager)\n", Version)
		fmt.Println("developer signatures: Ed25519-SHA256 (canonical manifest)")
		fmt.Printf("official root key:    %s\n", officialPublicKeyHex)
		fmt.Printf("official dev key:     %s\n", officialDevPublicKeyHex)
		return
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
	fmt.Printf("scpkg version %s: package and module manager for spermacore\n\n", Version)
	fmt.Println("usage:")
	fmt.Println("  scpkg bootstrap [url]              fetch base modules from releases and compile")
	fmt.Println("  scpkg install <name|repo|url>      install module from mainrepo, github or path")
	fmt.Println("  scpkg init <name>                  scaffold new module with about.toml and template")
	fmt.Println("  scpkg pack [dir] [-k key]          sign, validate, and package module into zip")
	fmt.Println("  scpkg build [module]               compile modules using cmake and ninja")
	fmt.Println("  scpkg doctor                       diagnose environment and audit module signatures")
	fmt.Println("  scpkg clean                        remove build artifacts and caches")
	fmt.Println("  scpkg sc-new-path [path]           set or update default spermacore root path")
	fmt.Println("  scpkg sign <dir|file> [-k key]     sign module or file with developer key")
	fmt.Println("  scpkg sign-modules <dir> [-k key]  batch sign all modules in directory")
	fmt.Println("  scpkg verify <path>                verify cryptographic signature")
	fmt.Println("  scpkg keygen [keyfile]             generate ed25519 developer signing keypair")
	fmt.Println("  scpkg version                      print version and trusted keys")
}
