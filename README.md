# secrets

A single source of truth for environment variable secrets, shared across multiple projects. Replaces scattered `.env` files with one age-encrypted store and per-project manifests.

## Why?

If you maintain many repositories that need overlapping sets of private keys, RPC URLs, and API keys, you've probably got dozens of `.env` files with duplicated secrets that are hard to rotate and easy to accidentally commit. `secrets` centralizes them in one encrypted file and injects them into your shell on demand — no plaintext files on disk.

## Install

**From GitHub releases:**

```sh
# macOS (Apple Silicon)
curl -L https://github.com/brickpop/secrets/releases/latest/download/secrets_darwin_arm64.tar.gz | tar xz
sudo mv secrets /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/brickpop/secrets/releases/latest/download/secrets_linux_amd64.tar.gz | tar xz
sudo mv secrets /usr/local/bin/
```

**From source:**

```sh
go install github.com/brickpop/secrets@latest
```

## Quickstart

```sh
# 1. Create the store (passphrase optional)
secrets init

# 2. Add some secrets
secrets set PRIVATE_KEY
# Value: (hidden input)

secrets set RPC_URL "https://rpc.example.com"

# 3. Use them
secrets get RPC_URL
secrets ls

# 4. In a project, create .secrets.yaml
cat > .secrets.yaml <<'EOF'
project: myproject
keys:
  - RPC_URL
  - PRIVATE_KEY
EOF

# 5. Load the env vars
eval "$(secrets resolve)"
```

## Agent

The agent holds the decrypted store in memory so you only enter your passphrase once per session. Every command auto-starts it on first use, prompting for the passphrase once. After that, secrets are served from memory for 8 hours.

You only interact with it directly if you want to adjust the lifetime or stop it early:

```sh
secrets agent --ttl 4h    # restart with a shorter lifetime
secrets agent --ttl 0     # no expiry
secrets agent stop        # wipe memory and exit now
```

## Usage modes

### 1. Standalone — on demand

No project files needed. Just use the store directly:

```sh
secrets set PRIVATE_KEY
secrets get PRIVATE_KEY
secrets ls
```

Useful for one-off lookups or scripts that pull individual secrets.

### 2. Project — `.secrets.yaml`

Add a manifest to your project (commit it — it contains no secrets):

```yaml
project: myproject
keys:
  - RPC_URL
  - PRIVATE_KEY
  - ETHERSCAN_API
```

Then load all of them at once:

```sh
eval "$(secrets resolve)"
```

The store key name must match the environment variable name. If your store uses different names, see mode 3.

### 3. Project with remapped names — `.secrets-map.yaml`

A local file (git-ignored) that maps local variable names to store keys. Only needed when the names differ:

```yaml
PRIVATE_KEY: PRIVATE_KEY_alice_hw
RPC_URL: RPC_URL_alchemy_pro
```

Add to `.gitignore`:

```
.secrets-map.yaml
```

`secrets resolve` checks this file first. If no mapping exists for a key, the variable name is used directly as the store key. The `.secrets.yaml` is still the source of truth for which variables the project needs — the map file is purely personal.

## Justfile integration

The recommended pattern for projects using [just](https://github.com/casey/just):

```makefile
_load-env:
    #!/usr/bin/env bash
    set -euo pipefail
    if command -v secrets &>/dev/null && [ -f .secrets.yaml ]; then
        eval "$(secrets resolve)"
    elif [ -f .env ]; then
        set -a && source .env && set +a
    else
        echo "Warning: no .env or .secrets.yaml found" >&2
    fi

deploy: _load-env
    forge script script/Deploy.s.sol --broadcast

test: _load-env
    forge test
```

Projects that don't use `secrets` continue working with `.env` files as before.

## Command reference

| Command | Description |
|---------|-------------|
| `secrets init` | Create the encrypted store |
| `secrets set <key> [value]` | Add or update a secret (prompts if value omitted) |
| `secrets get <key>` | Print a secret to stdout (no trailing newline) |
| `secrets ls` | List all keys (sorted, one per line) |
| `secrets rm <key>` | Delete a key (`-f` to skip confirmation) |
| `secrets passwd` | Change the store passphrase |
| `secrets resolve` | Resolve manifest keys and print as shell variables |
| `secrets dump` | Dump all secrets (debugging/migration) |
| `secrets agent [--ttl N]` | Adjust agent lifetime (optional — auto-started by every command) |
| `secrets agent stop` | Wipe memory and stop the agent |

### Resolve flags

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `posix` | Output format: `posix`, `fish`, `dotenv` |
| `-f`, `--file` | `.secrets.yaml` | Path to manifest file |
| `--partial` | `false` | Export empty values for missing keys instead of erroring |

### Output formats

| Format | Example | Usage |
|--------|---------|-------|
| `posix` | `export KEY='value'` | `eval "$(secrets resolve)"` |
| `fish` | `set -x KEY 'value'` | `secrets resolve --format fish \| source` |
| `dotenv` | `KEY="value"` | Pipe to files or other tools |

## Store location

Default store directory: `~/.local/share/secrets/`

```
~/.local/share/secrets/
  store.age    # encrypted key-value data
  meta.json    # store metadata (backend type)
  agent.sock   # ephemeral agent socket (while running)
```

Override with environment variables (in priority order):

1. `SECRETS_STORE_DIR` — explicit override (also used by tests)
2. `XDG_DATA_HOME/secrets/` — XDG standard
3. `~/.local/share/secrets/` — XDG default

## Security

- Encryption: [age](https://age-encryption.org) with scrypt passphrase (via `filippo.io/age`)
- Decrypted secrets are held in `[]byte` and zeroed on close
- Store file permissions enforced: directory `0700`, file `0600`
- No plaintext ever written to disk
- Atomic writes (temp file + rename) prevent corruption
- Agent communicates over a Unix domain socket, not TCP

## Development

Requires Go 1.22+, [just](https://github.com/casey/just), and `protoc` (for proto regeneration only).

```sh
just setup       # check/install dev toolchain (protoc, protoc-gen-go)
just help        # list all recipes
just check       # vet + lint + test
just test        # unit tests
just test-all    # unit + integration tests
just smoke       # quick end-to-end smoke test
just build       # build binary
just proto       # regenerate agent.pb.go from agent.proto (then commit)
```
