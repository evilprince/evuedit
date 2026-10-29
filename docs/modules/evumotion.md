# Animated short videos with `evumotion`

`evumotion` creates a silent portrait MP4 from a background still, optional transparent foreground cutout, a headline, and a built-in JSON template. It uses FFmpeg for rendering. The first version includes the `food-promo` template and targets YouTube Shorts, Instagram Reels, or Facebook Reels on a 1080×1920 canvas.

## Requirements

- FFmpeg available on `PATH`
- A JPEG, PNG, or WebP background image
- Optional transparent PNG or WebP foreground cutout when animating a subject separately

## List templates

```sh
evuedit --module evumotion --help
evuedit --module evumotion --list
```

## Create a video

```sh
evuedit --module evumotion \
  --template food-promo \
  --platform instagram \
  --file ./dish.jpg \
  --headline "FRESH FROM THE GRILL" \
  --output food-promo.mp4
```

Output is written to `~/evueditout`. The template sets the 8-second duration, 30 fps frame rate, headline position and fade animation, crop, and H.264 MP4 export. The background image fills the portrait canvas with a centered cover crop. The output is silent in this version.

## Animate a separate subject

A flattened image cannot move one object independently while keeping its background intact. To move a subject independently, provide a transparent cutout as `--foreground`:

```sh
evuedit --module evumotion \
  --template food-promo \
  --platform facebook \
  --file ./kitchen-background.jpg \
  --foreground ./dish-cutout.png \
  --headline "HOT AND FRESH" \
  --output facebook-promo.mp4
```

The `food-promo` template applies a gentle float to the cutout. The template, not command-line options, controls animation style and timing. The supported headline animations are fade, slide-up, and pop; the starter template selects fade. Subject animation can be none, float, or slide-up.

## Inputs, output, and errors

- `--template`, `--platform`, `--file`, `--headline`, and `--output` are required. `--foreground` is optional.
- Platforms are `youtube`, `instagram`, and `facebook`; all currently use the vertical 1080×1920 canvas.
- The output name must end in `.mp4` and cannot include a directory. The CLI creates `~/evueditout` as needed and never overwrites an existing file.
- The source still is not modified. Foreground cutouts must be PNG or WebP to preserve transparency.
- The template catalog is embedded from [`motion_templates.json`](../../motion_templates.json). Rebuild the executable after changing the catalog.
- Errors identify unknown templates/platforms, unsupported image types, missing files or FFmpeg, invalid output names, and FFmpeg rendering failures.
