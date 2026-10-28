#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
dist_dir=${1:-"$project_dir/dist"}
case "$dist_dir" in
	/*) ;;
	*) dist_dir="$PWD/$dist_dir" ;;
esac
mkdir -p "$dist_dir"

command -v go >/dev/null 2>&1 || {
	printf '%s\n' 'Go is required to build release packages.' >&2
	exit 1
}

stage_dir=$(mktemp -d "${TMPDIR:-/tmp}/evuedit-release.XXXXXX")
trap 'rm -rf "$stage_dir"' EXIT HUP INT TERM

cd "$project_dir"
for target in "darwin amd64" "darwin arm64" "linux amd64" "linux arm64"; do
	set -- $target
	goos=$1
	goarch=$2
	asset="evuedit_${goos}_${goarch}.tar.gz"
	target_dir="$stage_dir/${goos}_${goarch}"
	mkdir -p "$target_dir"
	printf 'Building %s/%s...\n' "$goos" "$goarch"
	GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -o "$target_dir/evuedit" .
	tar -czf "$dist_dir/$asset" -C "$target_dir" evuedit
done

(
	cd "$dist_dir"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum evuedit_*.tar.gz > SHA256SUMS
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 evuedit_*.tar.gz > SHA256SUMS
	else
		printf '%s\n' 'Need sha256sum or shasum to write the release checksums.' >&2
		exit 1
	fi
)

printf 'Release packages and SHA256SUMS are in %s\n' "$dist_dir"
