#!/bin/sh
# Installs the latest wt release binary for this machine, checksum-verified,
# as ~/.local/bin/worktree (override the directory with WT_INSTALL_DIR):
#
#   curl -fsSL https://github.com/glevski/wt/raw/main/install.sh | sh
#
set -eu

repo="glevski/wt"
install_dir="${WT_INSTALL_DIR:-$HOME/.local/bin}"

say() { printf 'wt: %s\n' "$*" >&2; }
fail() { say "$*"; exit 1; }

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin | linux) ;;
  *) fail "unsupported OS: $os — build from source: https://github.com/$repo" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "unsupported architecture: $arch — build from source: https://github.com/$repo" ;;
esac

command -v curl >/dev/null || fail "curl is required"

checksum() {
  if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi
}

asset="worktree-$os-$arch.tar.gz"
base="https://github.com/$repo/releases/latest/download"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "downloading $asset (latest release)"
curl -fsSL -o "$tmp/$asset" "$base/$asset" || fail "download failed: $base/$asset"
curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS" || fail "download failed: $base/SHA256SUMS"

want=$(awk -v a="$asset" '$2 == a { print $1 }' "$tmp/SHA256SUMS")
[ -n "$want" ] || fail "no checksum for $asset in SHA256SUMS"
got=$(checksum "$tmp/$asset" | awk '{ print $1 }')
[ "$got" = "$want" ] || fail "checksum mismatch for $asset (expected $want, got $got)"

tar -xzf "$tmp/$asset" -C "$tmp"
mkdir -p "$install_dir"
install -m 755 "$tmp/worktree" "$install_dir/worktree"

say "installed $("$install_dir/worktree" --version) to $install_dir/worktree"

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) say "note: $install_dir is not on your PATH" ;;
esac

shell_name=$(basename "${SHELL:-zsh}")
case "$shell_name" in
  bash) rc="~/.bashrc" ;;
  *) shell_name=zsh rc="~/.zshrc" ;;
esac
say "finish setup — add to $rc:"
say "  eval \"\$(worktree init $shell_name)\""
