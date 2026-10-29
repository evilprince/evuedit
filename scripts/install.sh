#!/bin/sh
set -eu

project_dir=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
install_dir="${HOME}/.local/bin"
output_dir="${HOME}/evueditout"
imagemagick_mode=prompt
ffmpeg_mode=prompt

usage() {
	cat <<'EOF'
Usage: scripts/install.sh [--with-imagemagick | --without-imagemagick] [--with-ffmpeg | --without-ffmpeg]

Build and install evuedit under ~/.local/bin and create ~/evueditout.
If ImageMagick is missing, ask whether to install it using the system package manager.

Options:
  --with-imagemagick     Install ImageMagick when it is missing.
  --without-imagemagick  Skip ImageMagick installation.
  --with-ffmpeg          Install FFmpeg when it is missing.
  --without-ffmpeg       Skip FFmpeg installation.
  --help                 Show this help.
EOF
}

imagemagick_available() {
	if command -v magick >/dev/null 2>&1; then
		return 0
	fi
	command -v convert >/dev/null 2>&1 && convert -version 2>&1 | head -n 1 | grep -q 'ImageMagick'
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--with-imagemagick) imagemagick_mode=install ;;
		--without-imagemagick) imagemagick_mode=skip ;;
		--with-ffmpeg) ffmpeg_mode=install ;;
		--without-ffmpeg) ffmpeg_mode=skip ;;
		--help|-h) usage; exit 0 ;;
		*) printf 'Unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
	esac
	shift
done

command -v go >/dev/null 2>&1 || {
	printf '%s\n' "Go is required to install evuedit. Install Go, then run this script again." >&2
	exit 1
}

if command -v ffmpeg >/dev/null 2>&1 && command -v ffprobe >/dev/null 2>&1; then
	printf '%s\n' 'FFmpeg is already available.'
elif [ "$ffmpeg_mode" = install ]; then
	sh "$project_dir/scripts/install-ffmpeg.sh"
elif [ "$ffmpeg_mode" = prompt ] && [ -t 0 ]; then
	printf '%s' 'FFmpeg is required for video presets, image filters, and animated videos. Install it now using your system package manager? [y/N] '
	read -r answer
	case "$answer" in
		y|Y|yes|YES|Yes) sh "$project_dir/scripts/install-ffmpeg.sh" ;;
		*) printf '%s\n' 'Skipping FFmpeg. Install it later with scripts/install-ffmpeg.sh.' ;;
	esac
else
	printf '%s\n' 'FFmpeg installation skipped. Install it later with scripts/install-ffmpeg.sh.'
fi

if imagemagick_available; then
	printf '%s\n' "ImageMagick is already available."
elif [ "$imagemagick_mode" = install ]; then
	sh "$project_dir/scripts/install-imagemagick.sh"
elif [ "$imagemagick_mode" = prompt ] && [ -t 0 ]; then
	printf '%s' 'ImageMagick is required by the evuthumb module. Install it now using your system package manager? [y/N] '
	read -r answer
	case "$answer" in
		y|Y|yes|YES|Yes) sh "$project_dir/scripts/install-imagemagick.sh" ;;
		*) printf '%s\n' 'Skipping ImageMagick. Install it later with scripts/install-imagemagick.sh.' ;;
	esac
else
	printf '%s\n' 'ImageMagick installation skipped. Install it later with scripts/install-imagemagick.sh.'
fi

mkdir -p "$install_dir" "$output_dir"
cd "$project_dir"
go build -o "$install_dir/evuedit" .
chmod 0755 "$install_dir/evuedit"

printf 'Installed evuedit to %s\n' "$install_dir/evuedit"
printf 'Created output directory %s\n' "$output_dir"
printf 'Ensure %s is on your PATH.\n' "$install_dir"
