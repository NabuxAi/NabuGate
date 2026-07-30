# NabuGate SDKs

One client per language, all against the same OpenAI-compatible gateway. Every
package covers the whole surface: chat, streaming, embeddings, images, speech,
the model catalogue and usage.

| Language | Package | Registry | Published |
|---|---|---|---|
| Node / TypeScript | `@nabugate/sdk` | npm | yes |
| Rust | `nabugate` | crates.io | yes |
| Dart / Flutter | `nabugate_sdk` | pub.dev | yes |
| Go | `github.com/nabuxai/nabugate-go` | Go modules | yes |
| Python | `nabugate` | PyPI | not yet |
| PHP / Laravel | `nabux/nabugate-laravel` | Packagist | not yet |

The two outstanding ones are blocked on an account action, not on the code:
PyPI asks for the account password before it will show the token page, and
Packagist already has an account under the same email that is not linked to
GitHub, so it wants a username-and-password sign-in before the GitHub connection
can be made. Both are one-time.

Each directory has its own README with installation and examples.

## The passthrough contract

The gateway forwards request bodies to the upstream provider untouched, so every
SDK lets an arbitrary parameter through — `tools`, `tool_choice`,
`response_format`, `seed`, penalties, anything a provider adds next. None of
them needs an SDK release to become usable. Do not "fix" an SDK by filtering the
request body to a known list of fields; that is the one behaviour every one of
these clients must not have.

## Releasing

Two paths, same result.

**From this machine** — fastest for a first release, uses whatever you are
already logged into:

```bash
npm login          # for npm
cargo login        # for crates.io
export TWINE_USERNAME=__token__ TWINE_PASSWORD=pypi-...   # for PyPI

./packages/publish.sh              # everything that is ready
./packages/publish.sh node rust    # or only the named ones
```

It skips a target whose credentials are missing rather than failing the run, and
tells you the one command that would fix it.

**From CI** — the durable path once the repository secrets exist.

All six ship at the same version, so a project in any language sees the same
surface.

1. `.github/workflows/sdk-ci.yml` builds and lints every package on each change.
2. Tag `sdk-v<version>` (or run the release workflow by hand with a version).
3. `.github/workflows/release-sdks.yml` publishes npm, PyPI, crates.io and
   pub.dev, and pushes subtree mirrors for the two registries that resolve a
   package from its own repository root:
   - `packages/laravel` → `nabuxai/nabugate-laravel` (Packagist)
   - `packages/go` → `nabuxai/nabugate-go` (`go get`)

The mirrors are generated; never commit to them directly.

### One-time setup

| Secret / setting | Where | For |
|---|---|---|
| `NPM_TOKEN` | repository secret | npm publish — must be a **granular** token with *bypass 2FA* enabled, or npm refuses with a 403. The one in use expires 90 days after creation, and npm is restricting this token type from January 2027; moving the workflow to Trusted Publishing removes both problems. |
| `CARGO_REGISTRY_TOKEN` | repository secret | crates.io publish |
| `MIRROR_TOKEN` | repository secret, `contents: write` on both mirrors | subtree mirrors |
| PyPI trusted publisher | pypi.org project settings | PyPI, no stored token |
| pub.dev automated publishing | pub.dev package settings | pub.dev, no stored token |
| Packagist submission | packagist.org, once, pointing at the mirror | Packagist |
