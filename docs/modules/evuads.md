# Google Ads image generation with `evuads`

`evuads` creates deterministic Google Ads image assets from a source image using the dimensions in the embedded, versioned `ads_templates.json` catalog. ImageMagick performs the auto-oriented center crop and JPEG export. The input file is preserved.

## Commands

```sh
evuedit --module evuads --list
evuedit --module evuads --platform google --adtype performance_max --file ./product.jpg
evuedit --module evuads --platform google --adtype performance_max --file ./product.jpg --logo ./brand.png
```

Supported ad types are `performance_max`, `demand_gen`, `responsive_display`, `search_image_assets`, and `fixed_display`. The input `--file` generates image assets. Optional `--logo` generates logo assets from a separate source. Campaign logo entries are skipped when `--logo` is not supplied. `--output-dir` changes the output directory; by default files go under `~/evueditout`.

Output names follow `<source-stem>_<asset-id>_<adtype>.jpg`, for example `product_landscape_performance_max.jpg`. Existing files are never overwritten. All output paths are checked before rendering begins.

## Catalog

`ads_templates.json` has a top-level version, platform, shared JPEG output configuration, campaign map, and fixed display specification. Each campaign asset defines an ID, type, width, and height, plus optional ratio, minimum dimensions, required status, recommended count, maximum count, and safe area. Campaign copy constraints are retained in the catalog for future copy validation. `fixed_display` sizes are rendered by selecting `--adtype fixed_display` and use that section's tighter 150 KB limit.

The shared output configuration uses JPEG, quality 90, sRGB, and a 5 MB maximum. Assets larger than that limit are removed and reported with a suggestion to lower quality or dimensions. Google recommends keeping important content within the center 80% when a `safeArea` is defined; the tool does not inspect or reposition image content to enforce that guidance.

## Errors and dependencies

The module reports unsupported platform/ad type, missing input, unsupported image format, invalid catalog entries, existing outputs, missing ImageMagick, and rendered files over the configured size limit. Supported inputs are JPEG, PNG, and WebP. ImageMagick is required; install it with `scripts/install-imagemagick.sh` or configure `EVU_IMAGEMAGICK_PATH`.
