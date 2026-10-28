# Video Editing Helper CLI — Development Plan

## Project goal

Build `evuedit`, a documented command-line helper for common video editing tasks. It should be straightforward to install and run from a terminal on Linux and macOS. Video encoding and filtering should use FFmpeg rather than reimplementing codec support.

## Recommended implementation language: Go

Use **Go** for the CLI. It produces a single native executable for each target platform, has a strong standard library for command-line programs and process management, and makes it practical to publish Linux and macOS builds without requiring users to install a language runtime. The CLI can invoke FFmpeg as an external dependency and report a clear installation/error message if FFmpeg is missing.

Python would be a reasonable choice for a quick prototype or a Python-heavy editing pipeline, but it adds runtime and packaging choices for end users. Since the goal is an installable terminal tool, Go is the better default. Keep the initial implementation small and verify FFmpeg availability at startup for commands that need it.

## Command shape

The initial interface can follow the proposed form, with a small usability adjustment: make the preset name an explicit value to `--apply`.

```sh
evuedit --tool preset --apply <presetName> --file <inputFile> --output <outputFile>
```

Example:

```sh
evuedit --tool preset --apply masala-gold --file ./clip.mp4 --output clip-graded.mp4
```

Use `evuedit --help` and `evuedit --tool preset --help` to expose usage, supported presets, and examples. The first implementation should never overwrite the input by default; validate paths and fail with actionable messages.

## Phases

### Phase 1 — Color preset application

- Establish the Go project structure and `evuedit` command entry point.
- Implement the `preset` tool with `--apply`, `--file`, and `--output` arguments.
- Define a small, named set of built-in color presets, each expressed as documented FFmpeg filter settings. Start with a few distinct looks (for example, neutral contrast, warm, cool, and monochrome) and make clear that they are starting points rather than universal grades.
- Check that the input exists, the output filename is safe, the preset name is supported, and FFmpeg is installed and runnable.
- Run FFmpeg without shell interpolation; surface useful progress/failure information and preserve the FFmpeg exit status where appropriate.
- Always write the output under `~/evueditout`, using `--output` as a filename. Create the folder at install time and on demand; never modify the source file.
- Add unit coverage for argument validation and preset configuration, plus an integration check using a generated short test clip when FFmpeg is available.
- Document installation/prerequisites, command syntax, preset names, examples, and common errors in `README.md` and built-in help.

**Phase 1 completion check:** a user can install/run the CLI on Linux or macOS, discover the available presets from documentation/help, and apply one to a video into a separate output file.

### Phase 2 — YouTube Shorts and Instagram Reels output (complete)

- Keep the preset listing (`evuedit --tool preset --list`) and descriptions available.
- Require portrait input; reject landscape/square input with an actionable invalid-format error instead of cropping or padding it.
- Write MP4 using H.264 (`libx264`, CRF 20, medium preset, `yuv420p`) and AAC audio (128 kb/s), with fast-start metadata for web playback.
- Show FFmpeg's live encoding progress and stop processing when the user interrupts the CLI.
- Validate the source video with `ffprobe`, require an `.mp4` output filename, prevent input/output collisions, and never overwrite an existing output.
- Document accepted orientation, output format, quality defaults, and processing behavior.

### Phase 3 — Preset catalog, use cases, and tags (complete)

- Keep preset definitions in the versioned `presets.json` catalog separate from CLI implementation code; each definition contains its stable preset ID, color properties, description, and tags.
- Store the tag vocabulary and use-case definitions in the separate versioned `use_cases.json` catalog so recommendations can evolve without changing grade values.
- Match presets to a use case only when the preset contains all tags assigned to that use case.
- Provide CLI discovery with `--list-use-cases` and `--use-case <id>` while preserving existing preset IDs and application behavior.
- Validate catalog versions, unique IDs, required fields, supported values, and tag references with errors that identify the invalid entry or field.
- Add module tests and documentation for the JSON structure, tags, matching rules, and recommendation commands.

### Phase 4 — Image filters (complete)

- Add the separate `evufilters` module; keep image filters distinct from video color presets.
- Support JPEG, PNG, and WebP input/output, with one still image frame per output.
- Provide `warm`, `cool`, `monochrome`, `vivid`, `soft`, and `neutral` filters with documented FFmpeg mappings.
- Store image filter definitions in a versioned `image_filters.json` catalog, separate from video presets.
- Support standard image resize targets (`square`, `portrait`, `story`, `landscape`, `youtube`, and `instagram`), explicit output format conversion, and JPEG/WebP quality control.
- Write output under `~/evueditout`, reject unsafe paths and existing outputs, and report FFmpeg failures clearly.
- Add module-specific tests and the `docs/modules/evufilters.md` usage guide.
- Document examples, filter effects, formats, and common errors.

### Phase 5 — Thumbnail creation with `evuthumb` (complete)

Create a deterministic thumbnail generator for YouTube and Instagram canvases. The module uses Go, ImageMagick, and versioned JSON layout templates. It renders the source image, gradient, and text as separate layers; it does not use generative AI to redraw the dish or other source subject. Ten templates are available from `templates/`, with imported `street-food` replacing the base catalog entry of the same ID.

Proposed command:

```sh
evuedit --module evuthumb --template street-food --platform youtube \
  --file ./dish.jpg --headline "THE BEST STREET FOOD" --output street-food.jpg
```

For multiple variants, use a UTF-8 text file with one headline per line:

```sh
evuedit --module evuthumb --template street-food --platform instagram \
  --file ./dish.jpg --headlines-file ./headlines.txt --output street-food.jpg
```

Template catalog shape (one versioned JSON file can contain multiple templates):

```json
{
  "version": 1,
  "templates": [
    {
      "id": "street-food",
      "version": 1,
      "canvas": {
        "youtube": {"width": 1280, "height": 720},
        "instagram": {"width": 1080, "height": 1920}
      },
      "layout": {
        "imageMode": "cover",
        "subjectPosition": "right",
        "gradient": "left-to-right",
        "headlinePosition": "left-center",
        "headlineMaxLines": 3,
        "headlineMaxWords": 5,
        "font": "assets/fonts/Anton-Regular.ttf",
        "fontSizeRatio": 0.09,
        "textColor": "#FFFFFF",
        "accentColor": "#FFD43B",
        "shadow": true
      },
      "export": {"format": "jpeg", "quality": 92, "colorSpace": "sRGB"}
    }
  ]
}
```

Planned scope and behavior:

- Accept a still image, template ID, platform (`youtube` or `instagram`), and output filename. Accept headlines through exactly one of `--headline "quoted text"` or `--headlines-file <path>`; the text file is UTF-8 with one headline per line, and blank/whitespace-only lines are ignored.
- Render one image for a single `--headline`. In file mode, render one variation per non-empty line and name outputs from the requested output base with a stable zero-padded sequence (for example, `street-food-001.jpg`, `street-food-002.jpg`). Preflight all headline constraints and output names before rendering so a validation error or existing output does not leave a partial batch.
- Write generated files under `~/evueditout`; do not overwrite source images or existing outputs.
- Load templates from a versioned catalog, validate unique IDs, canvas dimensions, supported layout positions, fonts, colors, headline limits, and export settings. Keep template JSON separate from rendering code.
- Use ImageMagick for deterministic crop/cover placement, gradient overlay, text rendering, shadow, and JPEG export. Convert the final image to sRGB and honor configured quality.
- Fit the headline automatically: wrap at word boundaries within the maximum word/line counts, reduce font size until it fits the available region, then report a useful error if it cannot fit at the documented minimum size. Never silently clip text.
- Resolve bundled font paths relative to installed template assets, and report missing fonts or ImageMagick with installation guidance.
- Treat ImageMagick as an external runtime dependency, not a separate evuedit plugin: discover `magick` (or a verified ImageMagick `convert` on legacy installations), check that it runs, and use an optional `EVU_IMAGEMAGICK_PATH` override for nonstandard installations. The installer can install it through the system package manager; no manual configuration is needed for a standard install.
- Add template discovery/help, ten JSON templates, tests for single/file headline input, batch naming/preflight, template validation, and headline fitting, an ImageMagick integration check when available, and `docs/modules/evuthumb.md`.
- Resolve the default `sans-serif` font to a common macOS/Linux font file when available, because ImageMagick font-family aliases vary by installation.
- Implement comparison image input (`--compare-file`), deal price text (`--price`), restaurant subtitle text (`--secondary-text`), directional gradients, labels, badges, borders, outlines, panels, and safe areas declared by the imported layouts.

AI boundary for this module:

| Task | Planned method | Optional/free route |
| --- | --- | --- |
| Add text and gradients | Deterministic ImageMagick rendering | ImageMagick |
| Crop and resize | Deterministic image processing | ImageMagick or Go image libraries |
| Remove a background | Optional AI segmentation, added later | Local ComfyUI workflow/model |
| Create a new background | Optional image generation, added later | Local ComfyUI and compatible open-weight model |
| Generate headline ideas | Optional language-model feature, added later | Local model if already available |
| Produce many thumbnails | Template-driven batch processing | `evuedit` CLI |

For food and review content, preserve the original dish and use AI only for background work when requested. Keep text rendering separate from image-generation models so headlines remain correctly spelled. ImageMagick and the Go CLI provide a repeatable first release without requiring AI.

### Phase 6 — Join clips with `evujoin` (planned, after `evuthumb`)

Add a dedicated `evujoin` module that combines video clips from a folder into one output video.

Proposed command:

```sh
evuedit --module evujoin --folder ./clips --output joined.mp4
```

Proposed behavior:

- Read regular video files directly inside the supplied folder (no recursive traversal); sort names alphabetically, case-insensitively, for predictable clip order.
- Recognize common video extensions `.mp4`, `.mov`, `.m4v`, and `.mkv`; ignore non-video files.
- Require at least two clips, verify each with `ffprobe`, and fail clearly if a candidate clip cannot be read.
- Write one `.mp4` under `~/evueditout`, create the folder when needed, never alter source clips, and refuse to overwrite an existing output.
- Use FFmpeg to create a broadly compatible H.264/AAC MP4. Prefer re-encoding so clips with different source codecs can be joined reliably.
- Keep joining independent from the portrait-only preset module; joining does not imply Shorts/Reels formatting.
- Add `--help`, focused unit tests using fake FFmpeg/ffprobe programs, an optional generated-clip integration check, and `docs/modules/evujoin.md` before marking the phase complete.

Decisions to settle while implementing: how to handle different resolutions, frame rates, orientations, and audio presence; whether to expose explicit clip ordering; and which output quality defaults to use. Choose and document safe defaults rather than silently crop or stretch clips.

### Phase 7 — Release and distribution

- Build and package versioned binaries for supported Linux and macOS architectures.
- Install the executable under the user's home directory and create `~/evueditout` during installation.
- Publish checksums and installation/upgrade/uninstall instructions.
- Add a release checklist covering clean-machine installation, FFmpeg detection, help output, and representative command runs.
- Document supported platforms, FFmpeg and ImageMagick requirements, version policy, and known limitations.
- Document `evuthumb` template and font asset installation.

## Documentation deliverables

- `README.md`: project overview, prerequisites, installation, quick start, and links to detailed usage.
- `docs/modules/<tool>.md`: one usage guide per CLI module, updated with each module's implementation; cover purpose, command/flags/defaults, examples, input/output contract, processing behavior, and errors.
- `docs/modules/README.md`: module guide index and documentation standard.
- Preset guide: included looks, intended effect, CLI usage, Shorts/Reels input/output rules, and FFmpeg processing settings.
- Development guide: build, test, release, and contribution instructions.
- Keep examples copyable and test them as part of release verification.

## Initial scope decisions

- **Runtime:** Go executable; FFmpeg is an external prerequisite for video operations.
- **Platforms:** Linux and macOS (including the architectures selected for release builds).
- **Phase 1 scope:** apply built-in color presets to one input video and write one output video.
- **Output location:** always generate files in `~/evueditout`; `--output` is a filename, not a path.
- **Safety:** never overwrite the input or an existing output implicitly; communicate validation and FFmpeg errors clearly.
- **Interface:** retain the requested `--tool preset` shape initially; consider conventional subcommands later only if they improve the interface without breaking documented usage.
- **Extensibility:** use a tool registry and a tool interface so each future tool owns its flags, validation, and execution. Shared dispatch remains responsible only for selecting a tool and showing the global tool list.
- **Short-form output:** Phase 2 targets portrait source footage for Shorts/Reels, keeps the source's portrait dimensions, and encodes a broadly compatible MP4 delivery file. Landscape input is an error; the CLI does not crop or pad it.
