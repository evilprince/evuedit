#!/bin/sh
set -eu

repo='evilprince/evuedit'
install_dir=${EVUEDIT_INSTALL_DIR:-"${HOME}/.local/bin"}
version=${EVUEDIT_VERSION:-latest}
ffmpeg_mode=prompt
case "$0" in
	sh|-sh|bash|-bash) script_dir= ;;
	*) script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" 2>/dev/null && pwd || true) ;;
esac

usage() {
	cat <<'EOF'
Install the prebuilt evuedit CLI from GitHub Releases.

Usage: scripts/install-release.sh [--version latest|TAG] [--install-dir PATH] [--with-ffmpeg | --without-ffmpeg]

Options:
  --version TAG       Install a release tag such as v0.1.0 (default: latest).
  --install-dir PATH  Install the binary here (default: ~/.local/bin).
  --with-ffmpeg       Install FFmpeg if missing using the system package manager.
  --without-ffmpeg    Skip the FFmpeg install prompt.
  --help              Show this help.

The installer creates ~/evueditout. FFmpeg is required for presets and image
filters; ImageMagick is required for evuthumb. Install those dependencies separately.
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--version)
			[ "$#" -ge 2 ] || { printf '%s\n' '--version requires latest or a tag.' >&2; exit 2; }
			version=$2
			shift 2
			;;
		--install-dir)
			[ "$#" -ge 2 ] || { printf '%s\n' '--install-dir requires a path.' >&2; exit 2; }
			install_dir=$2
			shift 2
			;;
		--with-ffmpeg) ffmpeg_mode=install; shift ;;
		--without-ffmpeg) ffmpeg_mode=skip; shift ;;
		--help|-h) usage; exit 0 ;;
		*) printf 'Unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
	esac
done

os=$(uname -s)
case "$os" in
	Darwin) goos=darwin ;;
	Linux) goos=linux ;;
	*) printf 'Unsupported operating system: %s (supported: macOS and Linux).\n' "$os" >&2; exit 1 ;;
esac

arch=$(uname -m)
case "$arch" in
	x86_64|amd64) goarch=amd64 ;;
	aarch64|arm64) goarch=arm64 ;;
	*) printf 'Unsupported processor architecture: %s (supported: x86_64 and ARM64).\n' "$arch" >&2; exit 1 ;;
esac

asset="evuedit_${goos}_${goarch}.tar.gz"
if [ -n "${EVUEDIT_RELEASE_BASE_URL:-}" ]; then
	base_url=${EVUEDIT_RELEASE_BASE_URL%/}
elif [ "$version" = latest ]; then
	base_url="https://github.com/$repo/releases/latest/download"
else
	base_url="https://github.com/$repo/releases/download/$version"
fi

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/evuedit-install.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

download() {
	url=$1
	destination=$2
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --retry 2 "$url" -o "$destination"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$url" -O "$destination"
	else
		printf '%s\n' 'Install curl or wget to download evuedit.' >&2
		exit 1
	fi
}

if ! download "$base_url/$asset" "$tmp_dir/$asset"; then
	printf 'Could not download %s. Check that a GitHub Release exists for this version and that it includes this platform.\n' "$asset" >&2
	exit 1
fi
if ! download "$base_url/SHA256SUMS" "$tmp_dir/SHA256SUMS"; then
	printf '%s\n' 'Could not download release checksums; refusing to install an unverified binary.' >&2
	exit 1
fi

expected=$(awk -v asset="$asset" '$2 == asset { print $1; exit }' "$tmp_dir/SHA256SUMS")
[ -n "$expected" ] || { printf 'No checksum found for %s.\n' "$asset" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(cd "$tmp_dir" && sha256sum "$asset" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
	actual=$(cd "$tmp_dir" && shasum -a 256 "$asset" | awk '{print $1}')
else
	printf '%s\n' 'Need sha256sum or shasum to verify the release download.' >&2
	exit 1
fi
[ "$actual" = "$expected" ] || { printf 'Checksum verification failed for %s.\n' "$asset" >&2; exit 1; }

members=$(tar -tzf "$tmp_dir/$asset")
[ "$members" = evuedit ] || { printf 'Unexpected files in release archive: %s\n' "$members" >&2; exit 1; }
tar -xzf "$tmp_dir/$asset" -C "$tmp_dir"
"$tmp_dir/evuedit" --help >/dev/null

mkdir -p "$install_dir" "$HOME/evueditout"
cp "$tmp_dir/evuedit" "$install_dir/evuedit"
chmod 0755 "$install_dir/evuedit"

printf 'Installed evuedit to %s/evuedit\n' "$install_dir"
printf 'Created %s/evueditout\n' "$HOME"
case ":${PATH}:" in
	*":$install_dir:"*) ;;
	*) printf 'Add %s to your PATH to run evuedit from any directory.\n' "$install_dir" ;;
esac
install_ffmpeg() {
	if [ -n "$script_dir" ] && [ -f "$script_dir/install-ffmpeg.sh" ]; then
		sh "$script_dir/install-ffmpeg.sh"
	else
		download "https://raw.githubusercontent.com/$repo/main/scripts/install-ffmpeg.sh" "$tmp_dir/install-ffmpeg.sh"
		sh "$tmp_dir/install-ffmpeg.sh"
	fi
}

if ! command -v ffmpeg >/dev/null 2>&1 || ! command -v ffprobe >/dev/null 2>&1; then
	if [ "$ffmpeg_mode" = install ]; then
		install_ffmpeg
	elif [ "$ffmpeg_mode" = prompt ] && [ -t 0 ]; then
		printf '%s' 'FFmpeg is required for video presets and image filters. Install it now using your system package manager? [y/N] '
		read -r answer
		case "$answer" in
			y|Y|yes|YES|Yes) install_ffmpeg ;;
			*) printf '%s\n' 'Skipping FFmpeg. Install it later with scripts/install-ffmpeg.sh.' ;;
		esac
	else
		printf '%s\n' 'Note: FFmpeg is not on PATH; video presets and image filters will need it. Install later with scripts/install-ffmpeg.sh.'
	fi
fi
if ! command -v magick >/dev/null 2>&1 && ! { command -v convert >/dev/null 2>&1 && convert -version 2>&1 | head -n 1 | grep -q ImageMagick; }; then
	printf '%s\n' 'Note: ImageMagick is not on PATH; evuthumb will need it.'
fi
