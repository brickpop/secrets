# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [0.1.0] - Unreleased

### Added
- Encrypted secret store using age/scrypt (`secrets init`, `secrets set`, `secrets get`, `secrets ls`, `secrets rm`)
- Passphrase management (`secrets passwd`) with empty passphrase support
- Per-project manifests (`.secrets.yaml`) with export to posix, fish, and dotenv formats
- Per-developer remapping via `.secrets-map.yaml`
- `--partial` flag for export: emit empty values for missing keys instead of erroring
- Background agent (`secrets agent`) holding decrypted store in memory with configurable TTL
- Agent is read-only: serves get/list over Unix domain socket
- Trial-decrypt for empty passphrases (no marker files, like OpenSSH)
- Pluggable `crypto.Backend` interface for future Yubikey/SSH agent support
- Atomic writes (temp file + rename) for crash safety
- Memory zeroing of decrypted secrets on close
- Permission checking with actionable fix commands
- XDG-compliant store location (`~/.local/share/secrets/`)
- `SECRETS_STORE_DIR` environment variable override
- GitHub Actions CI (vet, test, cross-compile) and release workflows
- goreleaser configuration for 5-target builds
- Comprehensive test suite: 70+ unit tests, 22 integration tests, smoke test
