#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
input="$repo_root/examples/testdata/sample-portrait.mp4"
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/evuedit-sample-test.XXXXXX")
trap 'rm -rf "$build_dir"' EXIT HUP INT TERM
test_home="$build_dir/home"
output_dir="$test_home/evueditout"
output_name="evuedit-sample-test-$$.mp4"
output="$output_dir/$output_name"

command -v go >/dev/null 2>&1 || { echo "Go is required to build evuedit." >&2; exit 1; }
command -v ffmpeg >/dev/null 2>&1 || { echo "FFmpeg is required." >&2; exit 1; }
command -v ffprobe >/dev/null 2>&1 || { echo "ffprobe is required." >&2; exit 1; }
[ -f "$input" ] || { echo "Sample fixture is missing: $input" >&2; exit 1; }

(
  cd "$repo_root"
  GOCACHE="${GOCACHE:-$build_dir/go-cache}" GOPATH="${GOPATH:-$build_dir/go-path}" go build -o "$build_dir/evuedit" .
)
HOME="$test_home" "$build_dir/evuedit" --tool preset --apply masala-gold --input-profile rec709 --file "$input" --output "$output_name"

codec=$(ffprobe -v error -select_streams v:0 -show_entries stream=codec_name -of default=noprint_wrappers=1:nokey=1 "$output")
pixel_format=$(ffprobe -v error -select_streams v:0 -show_entries stream=pix_fmt -of default=noprint_wrappers=1:nokey=1 "$output")
dimensions=$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=s=x:p=0 "$output")
[ "$codec" = "h264" ] || { echo "Expected H.264 output, got: $codec" >&2; exit 1; }
[ "$pixel_format" = "yuv420p" ] || { echo "Expected yuv420p output, got: $pixel_format" >&2; exit 1; }
[ "$dimensions" = "720x1280" ] || { echo "Expected portrait dimensions 720x1280, got: $dimensions" >&2; exit 1; }
printf 'Sample test passed: %s (%s, %s, %s)\n' "$output" "$codec" "$pixel_format" "$dimensions"
