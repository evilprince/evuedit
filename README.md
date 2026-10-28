# evuedit

`evuedit` is a command-line helper for video and image editing tasks. It uses a tool-based interface: each editing tool owns its options and implementation, while shared CLI dispatch selects the tool. The `preset` tool applies video color presets; `evufilters` applies named effects to still images.

## Install

Install a prebuilt release on macOS or Linux without installing Go:

```sh
curl -fsSL https://raw.githubusercontent.com/evilprince/evuedit/main/scripts/install-release.sh | sh
```

The installer selects the matching platform build, verifies its checksum, installs to `~/.local/bin/evuedit`, and creates `~/evueditout`. If FFmpeg is missing, it offers to install it through your system package manager. Go is needed only for building from source. See [INSTALL.md](INSTALL.md) for version selection, PATH setup, external tools, source installation, and release publishing.

The prebuilt CLI still uses external media tools: FFmpeg (`ffmpeg` and `ffprobe`) for video presets and image filters, and ImageMagick (`magick`) for thumbnails.

## Requirements

- Linux or macOS
- FFmpeg available on `PATH` (`ffmpeg` and `ffprobe` commands)
- ImageMagick available on `PATH` (needed for `evuthumb`)
- Go 1.22 or later only when building from source

## Build from source

```sh
go build -o evuedit .
```

Run `./scripts/install.sh` to build and install the executable to `~/.local/bin/evuedit`. The installer also creates `~/evueditout` and can offer to install FFmpeg and ImageMagick if missing. Use `--with-ffmpeg` or `--with-imagemagick` to install either dependency directly, or the corresponding `--without-*` option to skip it. Separate helpers are available at `./scripts/install-ffmpeg.sh` and `./scripts/install-imagemagick.sh`. These package installers do not install Homebrew itself. Ensure `~/.local/bin` is on your `PATH`. To build for another supported OS/architecture, set Go's `GOOS` and `GOARCH` before running `go build` (for example, `GOOS=linux GOARCH=amd64 go build -o evuedit-linux-amd64 .`).

## Apply a color preset

```sh
evuedit --tool preset --apply masala-gold --file ./clip.mp4 --output clip-graded.mp4 --profile-lut "./Samsung Log to Rec709 3DLUT_v1.0.cube"
```

The preset catalog currently declares Samsung Log input and Rec.709 SDR working output. Samsung Log footage needs the technical Samsung Log-to-Rec.709 LUT before the creative controls can be applied; pass that `.cube` file with `--profile-lut`. Samsung publishes the transform from its [Samsung Log Video developer page](https://developer.samsung.com/mobile/samsung-log-video.html). The LUT file is not bundled with `evuedit`; download it from Samsung and keep it locally. For footage already in Rec.709, use `--input-profile rec709` to skip the Samsung Log transform for that run.

### Shorts/Reels input and output

The preset tool is intended for YouTube Shorts and Instagram Reels. Input must be portrait video (`height > width`); landscape and square clips fail with an `invalid format` error. The CLI does not crop, pad, or rotate footage. It preserves the source's portrait dimensions and encodes MP4 with H.264 (`libx264`, `yuv420p`, CRF 20, medium encoder preset) and AAC audio at 128 kb/s. Fast-start metadata is enabled for web playback. The source should already be correctly oriented and framed vertically. `ffprobe` (normally installed with FFmpeg) is required to validate the input dimensions.

`--output` takes a filename only and must end in `.mp4`. Every generated file is written to `~/evueditout/<filename>`; directory components in `--output` are rejected. The CLI creates `~/evueditout` if it does not exist. The source is left unchanged, and an existing output file is never overwritten. FFmpeg displays live encoding progress and diagnostics in the terminal; press Ctrl-C to cancel.

## Presets

For complete usage examples, input requirements, output settings, and descriptions of every look, see the [preset module guide](docs/modules/preset.md). The [module documentation index](docs/modules/README.md) lists module guides and the documentation standard for future tools.

Discover recommendations with `evuedit --tool preset --list-use-cases`, then inspect matches with `evuedit --tool preset --use-case food-video`. Preset color properties and tags are stored separately from the tag vocabulary and use-case definitions in `presets.json` and `use_cases.json`; both are embedded at build time.

| Name | Effect |
| --- | --- |
| `masala-gold` | Warm golden color with restrained highlights and lifted shadows |
| `midnight-diner` | Darker, higher contrast, subdued color, and deeper shadows |
| `cafe-cream` | Soft, bright look with warm tones and open shadows |
| `street-food-pop` | Vivid color and punchier contrast |
| `chefs-table` | Restrained, natural color with protected highlights |
| `cinematic-warm` | Warm highlights with gently reduced saturation |
| `cool` | Cooler blue-toned image with moderate contrast |
| `monochrome` | Black and white with a modest contrast lift |
| `neutral-contrast` | Slightly richer contrast and color |

See available tools and command usage in the terminal:

```sh
evuedit --help
evuedit --tool preset --help
evuedit --module presets --help
evuedit --tool preset --list
```

Each preset stores `exposureEV`, `contrast`, `saturation`, `highlights`, `shadows`, `warmth`, and `lutIntensity`. Exposure is measured in stops; contrast and saturation are multipliers; highlights, shadows, and warmth use a normalized `-1.0` to `1.0` range. Exposure, contrast/saturation, tone curve, and warmth map to FFmpeg filters in that order after the input-profile LUT. `lutIntensity` is reserved for creative LUT blending and must remain `0.0` until a creative LUT stage is added. Presets are starting points, and their appearance depends on the source footage.

The catalog's top-level `inputProfile` and `workingOutput` apply to the catalog as a whole. Supported values are `samsung-log` and `rec709` for input, and `rec709-sdr` for output. Changing these values changes the color pipeline for every listed preset. For `rec709` input, omit `--profile-lut`.

## Image filters

List available image filters with `evuedit --module evufilters --list`. Apply a filter with `evuedit --module evufilters --filter warm --file ./photo.jpg --output photo-warm.jpg`, or generate every filter variation with `evuedit --module evufilters --filter all --file ./photo.jpg --output photo.jpg`. In `all` mode the output is named `photo-<filter>.jpg` for each available filter. Resize with `--resize instagram` (1080×1920) or `--resize youtube` (1280×720), convert with `--format webp`, and set JPEG/WebP quality with `--quality 85`. Supported image formats are JPEG, PNG, and WebP. Image filter definitions are kept in the versioned `image_filters.json` catalog. See the [image filter module guide](docs/modules/evufilters.md) for the catalog structure, effects, formats, and output settings.

## Thumbnail generation

Create YouTube and Instagram thumbnails from a JSON layout template and a headline with `evuedit --module evuthumb --template street-food --platform youtube --file ./dish.jpg --headline "THE BEST STREET FOOD" --output thumbnail.jpg`. The ten templates in `templates/` are embedded in the CLI. `food-comparison` accepts `--compare-file`, `food-deal` accepts `--price`, and `restaurant-review` accepts `--secondary-text`. To generate multiple headline variations, pass `--headlines-file ./headlines.txt`; each non-empty line creates a numbered output. ImageMagick is required. See the [thumbnail module guide](docs/modules/evuthumb.md) for template settings and layout behavior.

## Errors

The CLI reports errors for missing or unsupported options, unknown presets, landscape or square input, unavailable input/output paths, an output filename that is not MP4, an existing output file, missing profile LUT, or missing/failed FFmpeg/ffprobe. FFmpeg's diagnostic output is passed through to the terminal.

## Adding another tool

New editing features should be implemented as separate tools. Implement the `Tool` interface (`Name`, `Description`, and `Run`) and register the tool in `main.go`. Define that tool's flags in its own `Run` method using a dedicated `flag.FlagSet`. This keeps options such as trim ranges or resize dimensions scoped to the tool that uses them, while preserving the shared `evuedit --tool <name> ...` interface.

## Tests

Run the test suite with `go test ./...`. The test case list and a description of each case are in [TESTING.md](TESTING.md).
