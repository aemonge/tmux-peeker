#!/bin/sh
# tmux-peeker — Tmux Plugin Manager entry point.
#
# TPM (and compatible managers) execute every root-level *.tmux file after
# cloning this repository into the plugin directory. This script:
#
#   1. keeps a real binary copy at bin/tmux-peeker inside the plugin (never a
#      symlink), refreshed when the installed plugin version changes,
#   2. acquires that binary from the matching GitHub release when the clone
#      sits exactly on a released tag, building from the cloned source with
#      Go otherwise,
#   3. binds the popup behind prefix + <key> (@tmux-peeker-key, default m).
#
# It stays POSIX sh: TPM runs plugin scripts with the user's /bin/sh.

set -eu

PLUGIN_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="${PLUGIN_DIR}/bin"
BIN="${BIN_DIR}/tmux-peeker"
STAMP="${BIN_DIR}/.version"
REPO="aemonge/tmux-peeker"
DEFAULT_KEY="m"

key="$(tmux show-option -gqv '@tmux-peeker-key' 2>/dev/null || true)"
[ -n "$key" ] || key="$DEFAULT_KEY"

# Returns the clone's version when it is a single clean token (tag,
# tag-N-gHASH, or bare hash) and nothing otherwise. Anything else — no git
# metadata, git errors, or unexpected multi-line output — must not leak into
# ldflags or a release URL.
clone_version() {
    _v="$(git -C "$PLUGIN_DIR" describe --tags --always 2>/dev/null || true)"
    case "$_v" in
        *[!A-Za-z0-9._-]*|"") return 0 ;;
        *) printf '%s\n' "$_v" ;;
    esac
}

# Prints "<os> <arch>" in goreleaser terms, or fails on exotic platforms.
detect_platform() {
    _os="$(uname -s)"
    _arch="$(uname -m)"
    case "$_os" in
        Linux)  _os="linux" ;;
        Darwin) _os="darwin" ;;
        *)      return 1 ;;
    esac
    case "$_arch" in
        x86_64|amd64)  _arch="amd64" ;;
        arm64|aarch64) _arch="arm64" ;;
        *)             return 1 ;;
    esac
    printf '%s %s' "$_os" "$_arch"
}

download_release() {
    _tag="$1"
    _platform="$(detect_platform)" || return 1
    _os="${_platform% *}"
    _arch="${_platform#* }"
    _name="tmux-peeker_${_tag#v}_${_os}_${_arch}"
    _url="https://github.com/${REPO}/releases/download/${_tag}/${_name}.tar.gz"
    _tmp="$(mktemp -d 2>/dev/null)" || return 1
    if curl -fsSL "$_url" | tar -xzf - -C "$_tmp" 2>/dev/null \
        && [ -f "${_tmp}/tmux-peeker" ]; then
        mv -f "${_tmp}/tmux-peeker" "$BIN"
        chmod 755 "$BIN"
        rm -rf "$_tmp"
        return 0
    fi
    rm -rf "$_tmp"
    return 1
}

build_from_source() {
    command -v go >/dev/null 2>&1 || return 1
    _ldflags="-s -w"
    # Keep the binary's default "dev" version when the clone reports none.
    [ -n "$1" ] && _ldflags="$_ldflags -X main.version=$1"
    (cd "$PLUGIN_DIR" \
        && go build -ldflags "$_ldflags" -o "$BIN" ./cmd/tmux-peeker)
}

ensure_binary() {
    _version="$(clone_version)"

    if [ -x "$BIN" ] && [ -f "$STAMP" ] \
        && [ "$(cat "$STAMP" 2>/dev/null || true)" = "$_version" ]; then
        return 0
    fi

    mkdir -p "$BIN_DIR"
    tmux display-message 'tmux-peeker: fetching binary...' 2>/dev/null || true

    # Only a plain vTAG (no -N-gHASH suffix, no bare hash) sits exactly on a
    # released commit, so only then does a matching release archive exist.
    case "$_version" in
        v*[!0-9.]*) ;;
        v*)
            if download_release "$_version"; then
                printf '%s\n' "$_version" >"$STAMP"
                return 0
            fi
            ;;
    esac

    if build_from_source "$_version"; then
        printf '%s\n' "$_version" >"$STAMP"
        return 0
    fi

    return 1
}

if ensure_binary; then
    tmux bind-key "$key" run-shell \
        "TMUX_PEEKER_ORIGIN_SESSION=#{q:session_name} \"${BIN}\" popup"
else
    tmux display-message \
        'tmux-peeker: no binary. Install Go or download a release, then reload tmux config.' \
        2>/dev/null || true
fi
