# Thumbnail generation with `evuthumb`

`evuthumb` creates a still image for YouTube or Instagram from a source photo, a JSON layout template, and headline text. Rendering is deterministic: the original subject is kept, while cover crop, gradient, accent bar, shadow, and text are drawn by ImageMagick. Generative AI is not required.

## Requirements

- ImageMagick (`magick` preferred; a verified ImageMagick `convert` command is also accepted)
- JPEG, PNG, or WebP source image
- A system font family known to ImageMagick, or a font file referenced by the template

Install ImageMagick with `./scripts/install-imagemagick.sh`, or let `./scripts/install.sh` offer to install it. You can point to a nonstandard executable with `EVU_IMAGEMAGICK_PATH=/path/to/magick`.

## List templates

```sh
evuedit --module evuthumb --list
evuedit --module evuthumb --help
```

The available templates provide YouTube (1280×720) and Instagram (1080×1920) canvases.

## Generate one thumbnail

```sh
evuedit --module evuthumb \
  --template street-food \
  --platform youtube \
  --file ./dish.jpg \
  --headline "THE BEST STREET FOOD" \
  --output street-food.jpg
```

The headline can contain spaces when quoted in the shell. Use platform `instagram` for the vertical canvas.

## Generate from a headline file

Create a UTF-8 file with one headline per line. Empty lines are ignored.

```text
BEST STREET FOOD IN TOWN
TRY THIS SPICY DISH
OUR FAVORITE NIGHT MARKET
```

Run:

```sh
evuedit --module evuthumb \
  --template street-food \
  --platform instagram \
  --file ./dish.jpg \
  --headlines-file ./headlines.txt \
  --output street-food.jpg
```

The CLI preflights all headlines and destination names before rendering. Multiple headlines produce numbered files such as `street-food-001.jpg` and `street-food-002.jpg`. It refuses to overwrite any existing destination.

## Template catalog

The versioned base catalog is [`thumb_templates.json`](../../thumb_templates.json). The CLI also embeds each JSON template in [`templates/`](../../templates/) and merges them by ID; a file in that folder replaces a base catalog entry with the same ID. Rebuild `evuedit` after editing either source. The imported set provides ten distinct templates.

Three templates accept additional content on the command line:

```sh
evuedit --module evuthumb --template food-comparison --platform youtube \
  --file ./budget-dish.jpg --compare-file ./premium-dish.jpg \
  --headline "BUDGET OR PREMIUM" --output comparison.jpg

evuedit --module evuthumb --template food-deal --platform instagram \
  --file ./dish.jpg --headline "GREAT FOOD UNDER 500" --price 499 --output deal.jpg

evuedit --module evuthumb --template restaurant-review --platform youtube \
  --file ./dish.jpg --headline "A HIDDEN GEM IN MUMBAI" \
  --secondary-text "Bandra, Mumbai · 4.8/5" --output review.jpg
```

The comparison template requires `--compare-file`; the food-deal template requires `--price`; and the restaurant-review template requires `--secondary-text`. Other templates reject those options. Template fonts in `assets/fonts/` fall back to a common installed system font when the bundled example font is absent.

Available template IDs are `cafe-coffee`, `dessert`, `fine-dining`, `food-challenge`, `food-comparison`, `food-deal`, `instagram-cover`, `restaurant-review`, `street-food`, and `youtube-review`. `--list` prints their descriptions. The JSON settings control cover or split-cover cropping, directional gradients, headline placement and safe areas, labels, optional panels and borders, text outlines, comparison labels, and JPEG export quality. Split comparison uses the two supplied images, one on each side.

The initial template requests the `sans-serif` font. The CLI first resolves that default to a common installed font file on macOS or Linux, then falls back to ImageMagick's font lookup. A template can instead name a font family or a font file; absolute paths are used directly and relative paths resolve from the current working directory. Ensure any custom font is installed on the machine running the CLI.

## Layout and headline behavior

- `imageMode: cover` scales the source to fill the canvas and crops excess pixels. `subjectPosition` selects the crop anchor (`left`, `center`, or `right`).
- The `left-to-right` gradient darkens the headline side and fades toward transparency; `none` disables it.
- `left-to-right-soft`, `bottom-up`, and `bottom-up-soft` select alternate gradient directions and falloff.
- Labels, price and subtitle badges, panel overlays, borders, text outlines, and split-image dividers render when enabled by the template.
- Headline positions include left/center/right, top-center, and bottom-left. Template safe-area ratios keep text away from platform UI regions.
- Text wraps on word boundaries within the template word and line limits. ImageMagick measures the actual font, and the CLI reduces point size until the headline fits. If it cannot fit at the minimum size, it errors before rendering any image.
- The accent color is drawn as a vertical bar alongside the headline. The optional shadow is rendered behind the headline.
- Export is JPEG in sRGB at the template's configured quality (92 for the initial template).

## Output and errors

Output is always written under `~/evueditout`; `--output` must be a JPEG filename only. The CLI creates the directory when needed. It leaves the input unchanged and rejects existing output files and input/output collisions.

The module reports errors for unknown templates or platforms, unsupported input formats, invalid headline sources, too many words, headlines that cannot fit, unavailable fonts, missing or failed ImageMagick commands, and existing outputs. For a nonstandard ImageMagick installation, set `EVU_IMAGEMAGICK_PATH` to the executable path.
