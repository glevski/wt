#!/bin/sh
# Installs the latest wt release binary for this machine, checksum-verified,
# as ~/.local/bin/worktree (override the directory with WT_INSTALL_DIR):
#
#   curl -fsSL https://github.com/glevski/wt/raw/main/install.sh | sh
#
# It then adds the line that defines the wt command to ~/.zshrc or ~/.bashrc,
# unless it is already there (set WT_NO_MODIFY_RC=1 to leave the rc file alone).
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

# With the install dir off PATH the rc line has to name the binary in full.
case ":$PATH:" in
  *":$install_dir:"*) bin=worktree ;;
  *)
    bin="\"$install_dir/worktree\""
    say "note: $install_dir is not on your PATH"
    ;;
esac

# wt itself is a shell function, defined by the init line in the rc file. Any
# shell other than zsh and bash only gets the hint — init has nothing for it.
modify_rc=1
case "$(basename "${SHELL:-}")" in
  zsh) shell_name=zsh rc="${ZDOTDIR:-$HOME}/.zshrc" ;;
  bash) shell_name=bash rc="$HOME/.bashrc" ;;
  *) shell_name=zsh rc="$HOME/.zshrc" modify_rc="" ;;
esac
[ -z "${WT_NO_MODIFY_RC:-}" ] || modify_rc=""
init_line="eval \"\$($bin init $shell_name)\""

# Fails, leaving the hint to do the job, when the rc file can't be written —
# a read-only one managed by a dotfiles tool, say.
add_init_line() {
  {
    [ ! -s "$rc" ] || echo
    printf '# wt (https://github.com/%s)\n%s\n' "$repo" "$init_line"
  } 2>/dev/null >> "$rc"
}

if grep -q 'worktree"* init' "$rc" 2>/dev/null; then
  say "$rc already has the wt init line"
elif [ -n "$modify_rc" ] && add_init_line; then
  say "added the wt command to $rc — open a new shell to use it"
else
  say "finish setup — add to $rc:"
  say "  $init_line"
fi
