# scpkg

Modular package manager for SpermaCore modules.

## Features

* **Zero external dependencies**: pure Go standard library.
* **Cryptographic Signatures**: Ed25519 canonical manifest signing.
* **Security Guard**: prominent warnings on unofficial or unsigned native C++ plugins.
* **Registry Integration**: supports `scpkg-mainrepo` index and direct GitHub / local installations.
* **Module Packaging**: `init` and `pack` commands for module development and publishing.

## Commands

```bash
# Bootstrap base modules from SpermaCore releases
scpkg bootstrap [url]

# Install module from mainrepo, GitHub or local path
scpkg install <name|user/repo|url|path>

# Create a new module scaffold (about.toml + starter C++ plugin)
scpkg init <module_name>

# Validate, sign and pack module into zip for release
scpkg pack [dir] [-k key] [-o out.zip]

# Compile modules with CMake and Ninja
scpkg build [module]

# Diagnose environment and audit installed module signatures
scpkg doctor

# Clean build artifacts
scpkg clean

# Verify cryptographic signature of module or file
scpkg verify <path>

# Sign module or file with developer key
scpkg sign <dir|file> [-k key] [-author "Name"]

# Batch sign all modules in directory
scpkg sign-modules <dir> [-k key]

# Generate Ed25519 developer signing keypair
scpkg keygen [keyfile]

# Version and official developer keys
scpkg version
```

## Module Description (`about.toml`)

Every module includes an `about.toml` descriptor:

```toml
name = "my_module"
version = "1.0.0"
author = "DeveloperName"
human_description = "Short human readable description"
llm_description = "Description of tools for agent calling"
```

## License

BSD-2-Clause License.
