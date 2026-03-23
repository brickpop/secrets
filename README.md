# secrets

One source of truth for environment variables, shared across multiple projects. Replaces scattered `.env` files with one age-encrypted store and per-project manifests.

---

If you work across many repositories — each with its own `.env` file full of keys, RPC URLs, and API tokens — you've felt the pain: secrets duplicated everywhere, rotations that miss half the repos, files that accidentally get committed.

`secrets` is a CLI tool that keeps all your secrets in one age-encrypted store and injects them into any project on demand. No plaintext files. No duplication. One passphrase.

```sh
# Store a secret once
secrets set ALCHEMY_API_KEY "1234..."

# Use it in any project
eval "$(secrets resolve)"   # loads the vars declared in .secrets.yaml
```

---

## How it works

Each project declares which variables it needs in a `.secrets.yaml` manifest (safe to commit — no values, just names):

```yaml
# myproject/.secrets.yaml
project: myproject
keys:
  - RPC_URL
  - PRIVATE_KEY
  - ETHERSCAN_API
```

Running `secrets resolve` reads the manifest, fetches those keys from the encrypted store, and prints shell-ready output. You `eval` it and your environment is set.

The store lives at `~/.local/share/secrets/` and is decrypted once at first use. The agent keeps it in memory for 8 hours by default — you type your passphrase once, then forget about it.

---

## Install

**From source:**

```sh
go install github.com/brickpop/secrets@latest
```

**From GitHub releases:**

```sh
# macOS (Apple Silicon)
curl -L https://github.com/brickpop/secrets/releases/latest/download/secrets_darwin_arm64.tar.gz | tar xz
sudo mv secrets /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/brickpop/secrets/releases/latest/download/secrets_linux_amd64.tar.gz | tar xz
sudo mv secrets /usr/local/bin/
```

---

## Quickstart

```sh
# 1. Create the store (passphrase optional — press enter for none)
secrets init

# 2. Store some secrets
secrets set PRIVATE_KEY          # prompts with hidden input (keeps it out of shell history)
secrets set RPC_URL "https://rpc.example.com"
secrets set ETHERSCAN_API "abc123"

# 3. Check what's stored
secrets ls

# 4. Fetch a single value
secrets get RPC_URL

# 5. In your project, declare which keys to load
cat > .secrets.yaml <<'EOF'
project: myproject
keys:
  - RPC_URL
  - PRIVATE_KEY
EOF

# 6. Load them into your shell
eval "$(secrets resolve)"
```

---

## Use cases

### Load secrets into a project

Commit `.secrets.yaml` to your repo. Anyone with the store can run:

```sh
eval "$(secrets resolve)"                          # bash/zsh
secrets resolve --format fish | source             # fish
```

No `.env` files to manage, rotate, or accidentally commit.

### Run scripts with secrets injected

```sh
# justfile
deploy:
    #!/usr/bin/env bash
    eval "$(secrets resolve)"
    forge script script/Deploy.s.sol --broadcast

test:
    #!/usr/bin/env bash
    eval "$(secrets resolve)"
    forge test
```

### Import an existing `.env` file

Migrating from `.env` files is a one-liner:

```sh
secrets import .env
```

Handles conflicts interactively: 
- Skip existing keys
- Overwrite them, or 
- Save under a renamed key. Use `--suffix staging` to import environment-specific files without collisions.

### Rename or reorganise keys

```sh
secrets mv OLD_KEY_NAME NEW_KEY_NAME
```

Atomic rename — the old key is deleted and the new one is written in one operation.

### Different key name variants

If you have multiple environments stored under different names, add a local `.secrets-map.yaml` (git-ignored) that maps project variable names to the key names on your store:

```yaml
# .secrets-map.yaml  (never commit this)
PRIVATE_KEY: PRIVATE_KEY_alice_hw
RPC_URL: RPC_URL_alchemy_pro
```

`secrets resolve` checks this file first. The `.secrets.yaml` manifest stays the same for everyone on the team — only the personal mapping differs.

---

## Command reference

| Command | Description |
|---------|-------------|
| `secrets init` | Create the encrypted store |
| `secrets set <key> [value]` | Add or update a secret |
| `secrets get <key>` | Print a secret to stdout |
| `secrets resolve` | Inject project secrets (reads `.secrets.yaml`) |
| `secrets ls` | List all keys |
| `secrets rm <key> [key...]` | Delete one or more keys |
| `secrets mv <from> <to>` | Rename a key |
| `secrets import <file>` | Import keys from a `.env` file |
| `secrets dump` | Dump all secrets (debugging / migration) |
| `secrets passwd` | Change the store passphrase |
| `secrets agent [--ttl N]` | Inspect or adjust the agent lifetime |
| `secrets agent stop` | Wipe memory and stop the agent |

### `set` flags

| Flag | Description |
|------|-------------|
| `--overwrite` | Replace existing key without prompting |
| `--skip` | Do nothing if key already exists |

When a key already exists with a different value and neither flag is given, `set` prompts interactively on a TTY (`[o]verwrite / [r]ename / [s]kip`). Setting a key to the same value it already has is a no-op.

### `import` flags

| Flag | Description |
|------|-------------|
| `--suffix <s>` | Append `_<s>` to all imported key names (e.g. `--suffix staging` → `KEY_staging`) |
| `--overwrite` | Replace all conflicting keys without prompting |
| `--skip` | Keep all existing keys, only import new ones |

### `resolve` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `posix` | Output format: `posix`, `fish`, `dotenv` |
| `-f`, `--file` | `.secrets.yaml` | Path to manifest file |
| `--partial` | `false` | Export empty values for missing keys instead of erroring |

### Output formats

| Format | Example output | Shell usage |
|--------|----------------|-------------|
| `posix` | `export KEY='value'` | `eval "$(secrets resolve)"` |
| `fish` | `set -x KEY 'value'` | `secrets resolve --format fish \| source` |
| `dotenv` | `KEY="value"` | Pipe to files or other tools |

---

## The agent

The first command that needs the store auto-starts the agent: it decrypts the store into memory, starts a background process, and serves subsequent requests without touching disk. You type your passphrase once and it stays unlocked for 8 hours.

You only need to interact with it directly if you want to change the lifetime:

```sh
secrets agent --ttl 4h    # restart with a shorter lifetime
secrets agent --ttl 0     # never expire
secrets agent stop        # wipe memory and exit immediately
```

The agent communicates over a Unix domain socket (`agent.sock` in the store directory). It never writes decrypted data to disk.

---

## Store layout

```
~/.local/share/secrets/
  store.age    # all secrets, age-encrypted
  meta.json    # store metadata (backend type)
  agent.sock   # ephemeral socket (only while agent is running)
```

Override the directory:

```sh
export SECRETS_STORE_DIR=/path/to/store
```

---

## Security

- **Encryption**: [age](https://age-encryption.org) with scrypt key derivation (`filippo.io/age`)
- **No plaintext on disk**: secrets are never written unencrypted
- **Memory zeroing**: decrypted buffers are zeroed when the agent exits
- **Permissions**: store directory `0700`, file `0600`
- **Atomic writes**: temp file + rename prevents partial writes / corruption
- **Empty passphrase**: fully supported — same security model as OpenSSH keys

---

## Development

Requires Go 1.22+, [just](https://github.com/casey/just), and `protoc` (only for proto regeneration).

```sh
just setup       # check/install dev toolchain
just check       # vet + lint + test
just test-all    # unit + integration tests
just smoke       # quick end-to-end smoke test
just build       # build binary
just proto       # regenerate agent.pb.go (then commit)
```
