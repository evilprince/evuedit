# Using color presets

The `preset` tool applies a named color look and creates a Shorts/Reels-ready MP4 for YouTube Shorts, Instagram Reels, or Facebook Reels.

## Requirements

- `evuedit` installed and available on `PATH`
- FFmpeg and `ffprobe` available on `PATH`
- Portrait input video (`height > width`); landscape and square video are rejected
- A Samsung Log to Rec.709 `.cube` LUT when using the catalog's default Samsung Log input profile

## See available presets

```sh
evuedit --tool preset --list
```

The list includes each preset's name and short description. The current catalog includes:

| Preset | Look |
| --- | --- |
| `masala-gold` | Warm golden color, restrained highlights, and lifted shadows |
| `midnight-diner` | Darker, higher contrast, subdued color, and deeper shadows |
| `cafe-cream` | Soft, bright look with warm tones and open shadows |
| `street-food-pop` | Vivid color and punchier contrast |
| `chefs-table` | Restrained, natural color with protected highlights |
| `cinematic-warm` | Warm highlights with gently reduced saturation |
| `cool` | Cooler blue-toned image with moderate contrast |
| `monochrome` | Black and white with a modest contrast lift |
| `neutral-contrast` | Slightly richer contrast and color |

Presets are starting points; results vary with the source footage.

## Apply a preset

For Samsung Log footage, download Samsung's Log-to-Rec.709 LUT from the [Samsung Log Video developer page](https://developer.samsung.com/mobile/samsung-log-video.html), then pass its local path:

```sh
evuedit --tool preset \
  --apply masala-gold \
  --file ./vertical-clip.mp4 \
  --profile-lut "$HOME/Downloads/Samsung-Log-to-Rec709.cube" \
  --output vertical-clip-masala.mp4
```

For footage already in Rec.709, select that input profile and omit `--profile-lut`:

```sh
evuedit --tool preset \
  --apply cafe-cream \
  --input-profile rec709 \
  --file ./vertical-clip.mp4 \
  --output vertical-clip-cafe.mp4
```

Use `evuedit --module presets --help` to list every preset with its description, tags, and parameter values. The equivalent tool form is `evuedit --tool preset --help`. The catalog's default input profile is `samsung-log`; the `--input-profile rec709` option overrides it for one run.

## Find presets by use case

List supported use cases and their tags:

```sh
evuedit --tool preset --list-use-cases
```

Ask for the presets recommended for a use case:

```sh
evuedit --tool preset --use-case low-light-dining
```

Use the printed preset ID with `--apply` to process a clip. A preset is recommended only when it has **all** tags listed for that use case. For example, `low-light-dining` requires the `food`, `low-light`, and `cinematic` tags, which narrows the match to `midnight-diner`.

The color properties and preset tags live in [`presets.json`](../../presets.json). The tag vocabulary and use-case-to-tag mapping live in [`use_cases.json`](../../use_cases.json). Both JSON files are embedded in the executable at build time, so catalog edits take effect after rebuilding the CLI. The files are separate from Go code; runtime user-editable catalog loading is not currently supported.

## Output and input rules

- `--output` accepts a filename only, and it must end in `.mp4`.
- Output is written to `~/evueditout/<filename>`; the CLI creates this directory when needed.
- Existing output files are not overwritten, and the source file is not modified.
- Input must be portrait (`height > width`). Horizontal and square videos fail with an `invalid format` error. Rotate or re-record the source vertically before applying a preset; the CLI does not crop, pad, or rotate it.
- Output keeps the source's portrait dimensions and uses H.264 video (`libx264`, `yuv420p`, CRF 20, medium preset), AAC audio at 128 kb/s, and MP4 fast-start metadata.
- FFmpeg displays encoding progress and diagnostics. Press Ctrl-C to cancel.

## Preset controls

Each preset in [`presets.json`](../../presets.json) has an ID, description, tags, `exposureEV`, `contrast`, `saturation`, `highlights`, `shadows`, `warmth`, and `lutIntensity` values. Exposure is measured in stops. Contrast and saturation are multipliers. Highlights, shadows, and warmth use a normalized `-1.0` to `1.0` range. The Samsung Log technical LUT is applied before these creative adjustments. `lutIntensity` is reserved for creative LUT blending and must remain `0.0` until that feature is supported.

