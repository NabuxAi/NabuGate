# NabuGate SDKs

One client per language, all against the same OpenAI-compatible gateway. Every
package covers the whole surface: chat, streaming, embeddings, images, speech,
the model catalogue and usage.

| Language | Package | Registry |
|---|---|---|
| Node / TypeScript | `@nabugate/sdk` | npm |
| Python | `nabugate` | PyPI |
| Go | `github.com/nabuxai/nabugate-go` | Go modules |
| Rust | `nabugate` | crates.io |
| Dart / Flutter | `nabugate_sdk` | pub.dev |
| PHP / Laravel | `nabux/nabugate-laravel` | Packagist |

Each directory has its own README with installation and examples.

## The passthrough contract

The gateway forwards request bodies to the upstream provider untouched, so every
SDK lets an arbitrary parameter through — `tools`, `tool_choice`,
`response_format`, `seed`, penalties, anything a provider adds next. None of
them needs an SDK release to become usable. Do not "fix" an SDK by filtering the
request body to a known list of fields; that is the one behaviour every one of
these clients must not have.

## Releasing

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
| `NPM_TOKEN` | repository secret | npm publish |
| `CARGO_REGISTRY_TOKEN` | repository secret | crates.io publish |
| `MIRROR_TOKEN` | repository secret, `contents: write` on both mirrors | subtree mirrors |
| PyPI trusted publisher | pypi.org project settings | PyPI, no stored token |
| pub.dev automated publishing | pub.dev package settings | pub.dev, no stored token |
| Packagist submission | packagist.org, once, pointing at the mirror | Packagist |
