#!/bin/sh
set -eu

run_privileged() {
	if [ "$(id -u)" -eq 0 ]; then
		"$@"
	elif command -v sudo >/dev/null 2>&1; then
		sudo "$@"
	else
		printf '%s\n' 'This package-manager install needs root privileges; install FFmpeg manually or run with sudo available.' >&2
		exit 1
	fi
}

verify_ffmpeg() {
	command -v ffmpeg >/dev/null 2>&1 && command -v ffprobe >/dev/null 2>&1 || {
		printf '%s\n' 'FFmpeg installation finished, but ffmpeg and ffprobe were not both found on PATH.' >&2
		exit 1
	}
	ffmpeg -version | head -n 1
}

if command -v ffmpeg >/dev/null 2>&1 && command -v ffprobe >/dev/null 2>&1; then
	printf '%s\n' 'FFmpeg is already installed:'
	verify_ffmpeg
	exit 0
fi

os=$(uname -s)
case "$os" in
	Darwin)
		if ! command -v brew >/dev/null 2>&1; then
			printf '%s\n' 'Homebrew is required for automatic FFmpeg installation on macOS.' >&2
			printf '%s\n' 'Install Homebrew, then run scripts/install-ffmpeg.sh again; see https://brew.sh/.' >&2
			exit 1
		fi
		brew install ffmpeg
		;;
	Linux)
		if command -v apt-get >/dev/null 2>&1; then
			run_privileged apt-get update
			run_privileged apt-get install -y ffmpeg
		elif command -v dnf >/dev/null 2>&1; then
			run_privileged dnf install -y ffmpeg
		elif command -v yum >/dev/null 2>&1; then
			run_privileged yum install -y ffmpeg
		elif command -v pacman >/dev/null 2>&1; then
			run_privileged pacman -S --noconfirm ffmpeg
		elif command -v zypper >/dev/null 2>&1; then
			run_privileged zypper --non-interactive install ffmpeg
		elif command -v apk >/dev/null 2>&1; then
			run_privileged apk add ffmpeg
		else
			printf '%s\n' 'No supported package manager found (apt, dnf, yum, pacman, zypper, or apk).' >&2
			printf '%s\n' 'Install FFmpeg manually and ensure ffmpeg and ffprobe are on PATH.' >&2
			exit 1
		fi
		;;
	*)
		printf 'Unsupported operating system: %s\n' "$os" >&2
		exit 1
		;;
esac

printf '%s\n' 'FFmpeg installation verified:'
verify_ffmpeg
