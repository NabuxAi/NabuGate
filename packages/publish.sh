#!/usr/bin/env bash
#
# Publishes the NabuGate SDKs to their registries from this machine.
#
# The CI workflow (.github/workflows/release-sdks.yml) does the same thing from a
# tag, but needs repository secrets. This script uses whatever you are already
# logged into locally instead, which is the faster path for a first release.
#
#   ./packages/publish.sh            # publish every package that is ready
#   ./packages/publish.sh node rust  # publish only the named ones
#
# Each target reports its own outcome; one registry being unreachable or
# unauthenticated does not stop the others.
set -uo pipefail

VERSION="${VERSION:-1.0.0}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGETS=("$@")
[ ${#TARGETS[@]} -eq 0 ] && TARGETS=(node n8n python rust dart laravel go)

pass() { printf '  \033[32m✓\033[0m %s\n' "$1"; }
fail() { printf '  \033[31m✗\033[0m %s\n' "$1"; FAILED=1; }
skip() { printf '  \033[33m–\033[0m %s\n' "$1"; }
FAILED=0

wants() { for t in "${TARGETS[@]}"; do [ "$t" = "$1" ] && return 0; done; return 1; }

# ---------------------------------------------------------------------- npm
if wants node; then
  echo "npm — @nabugate/sdk"
  if ! npm whoami >/dev/null 2>&1; then
    skip "not logged in. Run: npm login"
  else
    ( cd "$ROOT/packages/node" \
      && npm install --no-package-lock --silent \
      && npm version "$VERSION" --no-git-tag-version --allow-same-version >/dev/null \
      && npm publish --access public ) \
      && pass "published $VERSION as $(npm whoami)" || fail "npm publish failed"
  fi
fi

# ----------------------------------------------------------- n8n Community Node
if wants n8n; then
  echo "n8n community node — n8n-nodes-nabugate"
  if ! npm whoami >/dev/null 2>&1; then
    skip "not logged in to npm. Run: npm login"
  else
    ( cd "$ROOT/packages/n8n-nodes-nabugate" \
      && npm version "$VERSION" --no-git-tag-version --allow-same-version >/dev/null \
      && npm publish --access public ) \
      && pass "published $VERSION to npm / n8n community as $(npm whoami)" || fail "n8n npm publish failed"
  fi
fi

# --------------------------------------------------------------------- PyPI
if wants python; then
  echo "PyPI — nabugate"
  if [ ! -f "$HOME/.pypirc" ] && [ -z "${TWINE_PASSWORD:-}" ]; then
    skip "no credentials. Create a token at pypi.org, then: export TWINE_USERNAME=__token__ TWINE_PASSWORD=pypi-..."
  else
    ( cd "$ROOT/packages/python" \
      && python3 -m pip install --quiet --upgrade build twine \
      && sed -i.bak "s/^version = .*/version = \"$VERSION\"/" pyproject.toml && rm -f pyproject.toml.bak \
      && rm -rf dist && python3 -m build --quiet \
      && python3 -m twine upload dist/* ) \
      && pass "published $VERSION" || fail "PyPI upload failed"
  fi
fi

# ---------------------------------------------------------------- crates.io
if wants rust; then
  echo "crates.io — nabugate"
  if [ ! -f "$HOME/.cargo/credentials.toml" ] && [ -z "${CARGO_REGISTRY_TOKEN:-}" ]; then
    skip "no token. Run: cargo login"
  else
    ( cd "$ROOT/packages/rust" \
      && sed -i.bak "0,/^version = .*/s//version = \"$VERSION\"/" Cargo.toml && rm -f Cargo.toml.bak \
      && cargo publish --allow-dirty ) \
      && pass "published $VERSION" || fail "cargo publish failed"
  fi
fi

# ------------------------------------------------------------------ pub.dev
if wants dart; then
  echo "pub.dev — nabugate_sdk"
  # `dart pub publish` opens a browser for consent on first use, so this one is
  # interactive by nature.
  ( cd "$ROOT/packages/dart" \
    && sed -i.bak "s/^version: .*/version: $VERSION/" pubspec.yaml && rm -f pubspec.yaml.bak \
    && dart pub get >/dev/null \
    && dart pub publish ) \
    && pass "published $VERSION" || fail "pub publish failed (it needs browser consent the first time)"
fi

# --------------------------------------------- Packagist and the Go module
#
# Both resolve a package from its own repository root, which a monorepo
# subdirectory cannot satisfy, so each is mirrored to a standalone repo. The
# mirrors are generated — never commit to them directly.
mirror() {
  prefix=$1; repo=$2
  sha=$(git -C "$ROOT" subtree split --prefix="$prefix" 2>/dev/null | tail -1 | tr -cd '0-9a-f')
  [ -n "$sha" ] || { fail "$repo: subtree split produced nothing"; return 1; }
  git -C "$ROOT" push --force --quiet "https://github.com/nabuxai/$repo.git" "$sha:refs/heads/main" \
    && git -C "$ROOT" push --force --quiet "https://github.com/nabuxai/$repo.git" "$sha:refs/tags/v$VERSION" \
    && pass "$repo mirrored and tagged v$VERSION" || fail "$repo mirror push failed"
}

if wants laravel; then
  echo "Packagist — nabux/nabugate-laravel"
  mirror packages/laravel nabugate-laravel
  skip "submit the mirror once at https://packagist.org/packages/submit (then it auto-updates)"
fi

if wants go; then
  echo "Go modules — github.com/nabuxai/nabugate-go"
  mirror packages/go nabugate-go
  if curl -fsS "https://proxy.golang.org/github.com/nabuxai/nabugate-go/@v/v$VERSION.info" >/dev/null 2>&1; then
    pass "the proxy is already serving v$VERSION"
  else
    skip "the proxy caches on first fetch: go get github.com/nabuxai/nabugate-go@v$VERSION"
  fi
fi

echo
[ $FAILED -eq 0 ] && echo "Done." || echo "Some targets did not publish — see the marks above."
exit $FAILED
