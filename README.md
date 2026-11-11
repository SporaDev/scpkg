# scpkg

Modular, zero-dependency package manager and bootstrapper for SpermaCore modules.

## Features

* Pure Go standard library: no external dependencies.
* Smooth TUI: terminal spinners, real-time download progress bar, and clean status indicators.
* Project path management: stores and remembers your spermacore root path.
* Download and extract official module releases (`releases/download/modules/modules.zip`).
* Cryptographic signature verification using Ed25519.
* Native CMake and Ninja compilation for C++20 ABI plugins.
* Community safety warnings for third-party plugins.

## Commands

```bash
# Bootstrap spermacore: prompts for root path if unset, fetches modules and compiles
go run . bootstrap

# Set or update the default spermacore root directory
go run . sc-new-path [path]

# Install a third-party module from github
go run . install username/module-repo

# Compile modules with cmake
go run . build [module]

# Diagnose environment and dependencies
go run . doctor

# Clean build artifacts
go run . clean

# Keypair generation and release signing (for developers)
go run . keygen
go run . sign <file> -k <private_key>
```

## Structure

```text
scpkg/
├── main.go         # Entry point and command router
├── config.go       # Path persistence (~/.scpkg/config.json) and sc-new-path
├── tui.go          # Spinners, progress bars and terminal styling
├── bootstrap.go    # Project bootstrapping workflow
├── install.go      # GitHub module installation and safety prompts
├── build.go        # CMake and Ninja orchestration
├── archive.go      # Safe streaming downloads and zip extraction
├── crypto.go       # Ed25519 signing and verification
├── doctor.go       # Environment health checks
└── clean.go        # Cache and build directory cleanup
```

## License

BSD-2-Clause License.
