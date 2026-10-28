# Sample video fixture

`sample-portrait.mp4` is a portrait MP4 sample downloaded for local CLI and end-to-end testing.

- Source: https://cdn.truefilesize.com/mp4/sample-portrait.mp4
- Source listing and license: https://truefilesize.com/video/mp4/portrait/ (listed as CC0 / public domain)
- SHA-256: `ff53e35118bf3628d2ed2958585d4f3de50b9ee8770790c49e0ef3ad9aae6c3f`
- Verified locally with ffprobe: 720x1280, H.264, 30 seconds

Run `sh scripts/test-sample.sh` from the repository root to build the CLI, apply the `masala-gold` preset as Rec.709 input, and verify the resulting MP4's video codec, pixel format, and portrait dimensions. The script uses a temporary home directory for its `evueditout` output and removes it after verification. This is ordinary Rec.709 footage; it is not Samsung Log footage and should not be processed with Samsung's technical Log conversion LUT.
