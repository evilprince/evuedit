#!/bin/sh
set -eu

run_privileged() {
	if [ "$(id -u)" -eq 0 ]; then
		"$@"
	elif command -v sudo >/dev/null 2>&1; then
		sudo "$@"
	else
		printf '%s\n' 'This package-manager install needs root privileges; install ImageMagick manually or run with sudo available.' >&2
		exit 1
	fi
}

verify_imagemagick() {
	if command -v magick >/dev/null 2>&1; then
		magick -version
		return
	fi
	if command -v convert >/dev/null 2>&1 && convert -version 2>&1 | head -n 1 | grep -q 'ImageMagick'; then
		convert -version | head -n 1
		return
	fi
	printf '%s\n' 'Package installation finished, but no ImageMagick command was found on PATH.' >&2
	printf '%s\n' 'Open a new terminal or install an ImageMagick 7 build that provides the magick command.' >&2
	exit 1
}

if command -v magick >/dev/null 2>&1 || { command -v convert >/dev/null 2>&1 && convert -version 2>&1 | head -n 1 | grep -q 'ImageMagick'; }; then
	printf '%s\n' 'ImageMagick is already installed:'
	verify_imagemagick
	exit 0
fi

os=$(uname -s)
case "$os" in
	Darwin)
		if ! command -v brew >/dev/null 2>&1; then
			printf '%s\n' 'Homebrew is required for automatic ImageMagick installation on macOS.' >&2
			printf '%s\n' 'Install Homebrew, then run scripts/install-imagemagick.sh again; see https://brew.sh/.' >&2
			exit 1
		fi
		brew install imagemagick
		;;
	Linux)
		if command -v apt-get >/dev/null 2>&1; then
			run_privileged apt-get update
			run_privileged apt-get install -y imagemagick
		elif command -v dnf >/dev/null 2>&1; then
			run_privileged dnf install -y ImageMagick
		elif command -v yum >/dev/null 2>&1; then
			run_privileged yum install -y ImageMagick
		elif command -v pacman >/dev/null 2>&1; then
			run_privileged pacman -S --noconfirm imagemagick
		elif command -v zypper >/dev/null 2>&1; then
			run_privileged zypper --non-interactive install ImageMagick
		elif command -v apk >/dev/null 2>&1; then
			run_privileged apk add imagemagick
		else
			printf '%s\n' 'No supported package manager found (apt, dnf, yum, pacman, zypper, or apk).' >&2
			printf '%s\n' 'Install ImageMagick manually and ensure its command is on PATH.' >&2
			exit 1
		fi
		;;
	*)
		printf 'Unsupported operating system: %s\n' "$os" >&2
		exit 1
		;;
esac

printf '%s\n' 'ImageMagick installation verified:'
verify_imagemagick
