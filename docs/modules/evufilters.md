# Image filters with `evufilters`

`evufilters` applies a named effect to one still image. It is a separate image tool; the `preset` module continues to process video.

## Requirements and supported formats

- FFmpeg and `ffprobe` available on `PATH`
- JPEG (`.jpg`, `.jpeg`), PNG (`.png`), or WebP (`.webp`) input and output
- One output still is written per selected filter. Animated WebP inputs are reduced to their first frame.

WebP output requires an FFmpeg build with the `libwebp` encoder. JPEG and PNG use FFmpeg's `mjpeg` and `png` encoders.

## List available filters

```sh
evuedit --module evufilters --list
```

The list shows each filter name and description. Property-based filters also show their exposure, contrast, saturation, warmth, and texture values.

| Filter | Effect |
| --- | --- |
| `warm` | Add a subtle warm golden tone. |
| `cool` | Add a subtle cool blue tone. |
| `monochrome` | Convert to black and white with a modest contrast lift. |
| `vivid` | Increase contrast and color saturation. |
| `soft` | Soften contrast and saturation and slightly lift brightness. |
| `neutral` | Add a small, balanced contrast and saturation lift. |
| `masala-glow` | Warm, bright food look with gentle detail enhancement. |
| `midnight-feast` | Moody, higher-contrast evening food look. |
| `cafe-cream` | Bright and soft look for cafes and interiors. |
| `street-food-punch` | Punchy color and crisp detail for street food. |
| `chefs-editorial` | Restrained editorial color with clean detail. |
| `crispy-detail` | Sharper texture for food details. |
| `dessert-velvet` | Soft, warm look for desserts and rich textures. |
| `true-taste` | Near-neutral rendering that keeps food color natural. |

## Apply a filter

```sh
evuedit --module evufilters \
  --filter warm \
  --file ./photo.jpg \
  --output photo-warm.jpg
```

The equivalent selector is `--tool evufilters`. Use `evuedit --module evufilters --help` to see module help and filter names.

## Resize and convert

Use `--resize` with one of the built-in output dimensions. The image is scaled proportionally to cover the target and center-cropped to the exact dimensions:

| Size | Dimensions | Typical use |
| --- | --- | --- |
| `square` | 1080×1080 | Square social post |
| `portrait` | 1080×1350 | Portrait feed post |
| `story` | 1080×1920 | Stories and vertical posts |
| `landscape` | 1920×1080 | Widescreen post or thumbnail |
| `youtube` | 1280×720 | YouTube 16:9 image |
| `instagram` | 1080×1920 | Instagram vertical image |
| `facebook-video` | 1280×720 | Facebook video (16:9) |
| `facebook-feed` | 1080×1350 | Facebook feed (4:5) |
| `facebook-square` | 1080×1080 | Facebook square (1:1) |
| `facebook-reels` | 1080×1920 | Facebook Reels (9:16) |
| `facebook-link-preview` | 1200×630 | Facebook link preview (1.91:1) |
| `facebook` | 1080×1920 | Alias for `facebook-reels` |

For example, make an image for a Facebook feed post and convert to WebP:

```sh
evuedit --module evufilters --filter masala-glow \
  --file ./photo.jpg --output photo --resize facebook-feed --format webp --quality 85
```

When `--format` is provided without an extension on `--output`, the CLI adds the matching extension. If `--output` already has an extension, it must agree with `--format`. Without `--format`, the output extension selects the format as before. `--quality` accepts 1–100 for JPEG and WebP; the default is 90. JPEG quality is translated to FFmpeg's quality scale. PNG is lossless, so `--quality` is rejected for PNG output. Without `--resize`, output keeps the source dimensions.

## Generate outputs for every filter

Use `all` to create one image for each named filter:

```sh
evuedit --module evufilters \
  --filter all \
  --file ./photo.jpg \
  --output photo.jpg
```

Here, `--output` supplies the filename base and format. The CLI writes one `<base>-<filter>.<ext>` file per catalog filter under `~/evueditout`. It checks every destination before processing and stops before encoding if any already exists. Filters run sequentially; if FFmpeg fails partway through, outputs already completed remain available.

## Input and output behavior

- `--file` must point to a regular JPEG, PNG, or WebP image that `ffprobe` can inspect.
- `--output` is a filename only. Its extension selects the output format unless `--format` is provided; directory paths are rejected.
- `--format` can explicitly select `jpg`, `png`, or `webp`; if the output filename has no extension, the selected format's extension is added.
- `--quality` sets JPEG/WebP output quality from 1 to 100 (default 90); PNG is lossless and does not accept this option.
- `--resize` selects an exact standard canvas and center-crops after proportional scaling; without it the source dimensions are preserved.
- Output is written under `~/evueditout`; the CLI creates this folder when needed.
- The source is not modified, and existing output files are never overwritten.
- In `all` mode, the output base gets a `-<filter>` suffix before its extension; every destination is checked before the first filter runs.
- The output keeps the decoded image dimensions unless `--resize` is selected. When the input is animated WebP, the first frame is used.
- JPEG quality is mapped to FFmpeg's `mjpeg` quantizer; PNG uses compression level 6; WebP uses `libwebp` with the selected quality and compression level 6.
- Press Ctrl-C to cancel processing.

## Errors

The module reports errors for unknown filters, missing input/output arguments, unsupported extensions, missing or unreadable input images, output path traversal, existing outputs, missing FFmpeg/ffprobe, and FFmpeg encoding failures. If WebP output fails because `libwebp` is unavailable, install an FFmpeg build that includes that encoder or choose JPEG/PNG output.

## Filter definitions

Filter definitions live in the versioned [`image_filters.json`](../../image_filters.json) catalog, separate from the video preset catalog. Each entry has a stable `id`, user-facing `description`, discovery `tags`, and either an FFmpeg `filterGraph` or a `properties` object. Catalog version 1 is embedded in the CLI binary at build time and validated when the module starts. Property-based entries expose `exposureEV`, `contrast`, `saturation`, `warmth`, and `texture`.

For property-based filters, exposure is measured in stops and mapped to FFmpeg `exposure`; contrast and saturation are multipliers mapped to `eq`; warmth adds a red bias and reduces blue through `colorbalance`; texture maps to `unsharp` amount (negative values slightly soften). These values are starting points, and the rendered appearance depends on the source image.

Example catalog entry:

```json
{
  "id": "warm",
  "description": "Add a subtle warm golden tone.",
  "tags": ["warm", "natural"],
  "filterGraph": "colorbalance=rs=0.08:bs=-0.08:rm=0.05:bm=-0.05:rh=0.03:bh=-0.03"
}
```

Property-based entry example:

```json
{
  "id": "masala-glow",
  "description": "Bright, warm food imagery with gentle detail enhancement.",
  "tags": ["food", "warm", "vivid"],
  "properties": {
    "exposureEV": 0.15,
    "contrast": 1.08,
    "saturation": 1.10,
    "warmth": 0.12,
    "texture": 0.10
  }
}
```

