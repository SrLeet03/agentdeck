#!/bin/sh
# Installs deck (agentdeck) from GitHub releases.
#   curl -fsSL https://raw.githubusercontent.com/SrLeet03/agentdeck/main/install.sh | sh
# Env: DECK_VERSION (default: latest, e.g. v0.2.0), DECK_INSTALL_DIR (default: ~/.local/bin)
set -eu

repo="SrLeet03/agentdeck"
dir="${DECK_INSTALL_DIR:-$HOME/.local/bin}"

case "$(uname -s)" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) echo "deck: unsupported OS $(uname -s) (macOS and Linux only)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) echo "deck: unsupported CPU $(uname -m)" >&2; exit 1 ;;
esac

tag="${DECK_VERSION:-latest}"
if [ "$tag" = latest ]; then
	url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${repo}/releases/latest")"
	tag="${url##*/}"
fi
version="${tag#v}"
file="deck_${version}_${os}_${arch}.tar.gz"
base="https://github.com/${repo}/releases/download/${tag}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "deck: downloading ${tag} for ${os}/${arch}"
curl -fsSL -o "$tmp/$file" "$base/$file"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"

want="$(awk -v f="$file" '$2 == f { print $1 }' "$tmp/checksums.txt")"
if command -v sha256sum >/dev/null 2>&1; then
	got="$(sha256sum "$tmp/$file" | awk '{ print $1 }')"
else
	got="$(shasum -a 256 "$tmp/$file" | awk '{ print $1 }')"
fi
if [ -z "$want" ] || [ "$want" != "$got" ]; then
	echo "deck: checksum mismatch for $file" >&2
	exit 1
fi

tar -xzf "$tmp/$file" -C "$tmp" deck
mkdir -p "$dir"
install -m 755 "$tmp/deck" "$dir/deck"
echo "deck: installed $("$dir/deck" version) to $dir/deck"

case ":$PATH:" in
	*":$dir:"*) ;;
	*) echo "deck: add $dir to your PATH, e.g.  echo 'export PATH=\"$dir:\$PATH\"' >> ~/.profile" ;;
esac
echo "deck: run 'deck doctor', then 'deck'"
