# Test cases

Run the project test suite from the repository root:

```sh
go test ./...
```

For a real FFmpeg end-to-end check using the included portrait fixture, run:

```sh
sh scripts/test-sample.sh
```

The script builds a temporary CLI binary, applies `masala-gold` to `examples/testdata/sample-portrait.mp4` as Rec.709 input, and checks for H.264, `yuv420p`, and 720x1280 output. It uses and removes a temporary home/output folder. The fixture source and checksum are recorded in `examples/testdata/README.md`.

The tests use Go's standard library. Motion and preset execution is checked with small fake `ffmpeg` and `ffprobe` executables, so tests do not encode real video or require FFmpeg to be installed. The fake executable tests use a POSIX shell, matching the project's Linux and macOS targets.

## CLI dispatch (`main_test.go`)

| Test case | Purpose |
| --- | --- |
| `TestExtractToolKeepsToolOptionsAndSupportsEqualsSyntax` | Confirms the global dispatcher accepts `--tool=name` and forwards all tool-specific arguments unchanged. |
| `TestRunSupportsPresetsModuleAliasAndDetailedHelp` | Confirms `--module presets --help` selects the preset module and prints preset descriptions, tags, and color parameter values. |
| `TestRunDispatchesToEvufiltersModule` | Confirms `--module evufilters --help` reaches the image filter tool and displays its options. |
| `TestRunDispatchesToEvuthumbModule` | Confirms `--module evuthumb --help` reaches the thumbnail module. |
| `TestExtractToolRejectsDuplicateSelectors` | Confirms ambiguous repeated `--tool`/`--module` selectors are rejected. |
| `TestRunDispatchesToSelectedTool` | Confirms the selected registered tool receives its own arguments and runs. |
| `TestRunRejectsUnknownToolAndRequiresSelector` | Confirms unknown tools and commands without a tool/module selector return clear errors. |

## Preset tool (`preset_test.go`)

| Test case | Purpose |
| --- | --- |
| `TestPresetToolHelpListsAvailablePresets` | Confirms help shows command syntax, the Samsung Log LUT option, and all catalog preset names. |
| `TestPresetCatalogLoadsVersionedSamsungLogPresets` | Confirms the catalog version, input/output profiles, all nine preset IDs, and exact `masala-gold` values. |
| `TestUseCaseCatalogMatchesPresetsByTags` | Confirms versioned tag/use-case catalog loading, valid cross-catalog tag references, all-tags matching behavior, and rejection of unknown preset tags. |
| `TestPresetToolListsUseCasesAndRecommendations` | Confirms use cases and tags are discoverable, matching presets are listed, unknown use cases fail clearly, and conflicting discovery flags are rejected. |
| `TestPresetFilterMapsCreativeControls` | Confirms the profile LUT stage comes before exposure, contrast/saturation, tone-curve, and warmth adjustments, and Rec.709 input skips the LUT. |
| `TestPresetToolListsPresetsAndRequiresProfileLUT` | Confirms users can list catalog entries and that Samsung Log work cannot silently skip its required technical conversion. |
| `TestPresetToolValidatesPresetAndRequiredPaths` | Confirms unknown presets and missing input/output paths are rejected. |
| `TestPresetToolRejectsLandscapeAndNonMP4Output` | Confirms Shorts processing rejects landscape input and non-MP4 output names. |
| `TestPresetToolRejectsUnsafeOrUnavailablePaths` | Confirms output names cannot escape `~/evueditout`, the input cannot also be the output, existing output is not overwritten, and a missing input fails. |
| `TestPresetToolRunsFFmpegWithSelectedFilter` | Uses fake FFmpeg/ffprobe programs to confirm portrait probing, color filter selection, H.264/AAC Shorts encoding defaults, fixed output directory, progress reporting configuration, success message, and per-run Rec.709 override without encoding video. |

## Image filter tool (`image_test.go`)

| Test case | Purpose |
| --- | --- |
| `TestImageFilterHelpAndList` | Confirms module help and `--list` show the available named image effects. |
| `TestImageFilterCatalogLoadsVersionedDefinitions` | Confirms the embedded JSON catalog loads its filter properties and rejects unsupported versions, duplicate IDs, and missing filter graphs. |
| `TestImageFilterToolValidatesFilterPathsAndFormats` | Confirms unknown filters, missing required arguments, unsupported formats, and unsafe output paths fail before encoding. |
| `TestImageFilterToolRunsFFmpegAndDoesNotOverwrite` | Uses fake FFmpeg/ffprobe programs to verify filter graph selection, Facebook placement resize dimensions, explicit WebP conversion and quality, fixed output folder, success reporting, and existing-output protection. |
| `TestImageResizePresetsIncludePlatformDimensions` | Confirms YouTube maps to 1280×720 and Instagram/Facebook Reels map to 1080×1920, and the five Facebook image placements map to their named dimensions. |
| `TestImageFilterAllGeneratesOneNamedOutputPerFilter` | Uses fake FFmpeg/ffprobe programs to confirm `--filter all` runs each filter and writes a distinct suffixed output for each. |
| `TestImageFilterAllChecksEveryOutputBeforeProcessing` | Confirms an existing destination anywhere in an `all` batch prevents every encoding run before partial output is created. |

## Thumbnail tool (`thumb_test.go`)

| Test case | Purpose |
| --- | --- |
| `TestThumbTemplateCatalogValidation` | Confirms all ten JSON template files are embedded, imported `street-food` overrides the base entry, YouTube/Instagram/Facebook canvas dimensions load, and malformed catalog versions or duplicate IDs are rejected. |
| `TestThumbHeadlineWrappingAndBatchNames` | Confirms wrapping preserves all headline words and batch output filenames are numbered deterministically. |
| `TestThumbToolHelpListAndHeadlineValidation` | Confirms help/template discovery and mutually exclusive headline sources and template word limits. |
| `TestImportedThumbTemplatesRequireTheirDynamicInputs` | Confirms comparison, deal, and restaurant templates require the second image, price, and subtitle input respectively. |
| `TestReadThumbHeadlines` | Confirms headline files are read in order, with whitespace trimmed and blank lines ignored. |
| `TestThumbRenderingInvokesImageMagickLayers` | Uses a fake ImageMagick executable to verify Facebook Reels canvas sizing, cover sizing, gradient composition, headline rendering, sRGB/JPEG export, and output reporting. |

Real ImageMagick renders were run for each of the ten templates. The comparison, deal, and restaurant cases supplied `--compare-file`, `--price`, and `--secondary-text`; each produced a valid 1280×720 sRGB JPEG. The unit renderer check additionally verifies the Facebook Reels 1080×1920 canvas.

### Optional real ImageMagick integration check

The unit test uses a fake executable so it is fast and deterministic. With ImageMagick installed, verify the actual renderer and font setup against a real source photo:

```sh
evuedit --module evuthumb --template street-food --platform youtube \
  --file ./dish.jpg --headline "THE BEST STREET FOOD" --output thumb-smoke.jpg
magick identify "$HOME/evueditout/thumb-smoke.jpg"
```

Expected output is a JPEG with `1280x720` dimensions and sRGB color. Repeat with `--platform instagram` or `--platform facebook-reels` and a distinct output filename to verify the `1080x1920` vertical canvas. The `facebook` alias selects the same canvas. Use `facebook-video`, `facebook-feed`, `facebook-square`, or `facebook-link-preview` to check their respective canvases. This check also catches ImageMagick font lookup differences that the fake executable cannot detect.


## Motion tool (`motion_test.go`)

| Test case | Purpose |
| --- | --- |
| `TestMotionTemplateCatalogValidation` | Confirms the embedded versioned catalog loads its starter template and vertical canvas. |
| `TestMotionFilterGraphSelectsTemplateAnimations` | Confirms the template-selected headline fade and independent subject float are compiled into the FFmpeg filter graph. |
| `TestMotionToolRunsFFmpegAndWritesOutput` | Uses fake FFmpeg to verify foreground compositing, animated text, H.264/yuv420p silent MP4 settings, and output location. |
| `TestMotionToolValidatesArgumentsAndOutput` | Confirms missing arguments, unsupported platforms, directory traversal, and non-MP4 output names fail before processing. |
