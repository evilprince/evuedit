package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageFilterHelpAndList(t *testing.T) {
	// Module help and --list should make the available image effects discoverable.
	filters, err := loadEmbeddedImageFilters()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := (imageFilterTool{}).Run([]string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help returned error: %v", err)
	}
	for _, name := range imageFilterNames(filters) {
		if !strings.Contains(stdout.String(), name) {
			t.Errorf("help output missing filter %q", name)
		}
	}
	stdout.Reset()
	if err := (imageFilterTool{}).Run([]string{"--list"}, &stdout, &stderr); err != nil {
		t.Fatalf("list returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "monochrome") || !strings.Contains(stdout.String(), "warm") || !strings.Contains(stdout.String(), "exposureEV=+0.15 contrast=1.08 saturation=1.10") {
		t.Fatalf("filter list is incomplete: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "all") {
		t.Fatalf("filter list does not describe batch mode: %s", stdout.String())
	}
}

func TestImageFilterCatalogLoadsVersionedDefinitions(t *testing.T) {
	// The versioned JSON catalog supplies filter IDs, descriptions, tags, and FFmpeg graphs.
	filters, err := loadEmbeddedImageFilters()
	if err != nil {
		t.Fatalf("load image filter catalog: %v", err)
	}
	if len(filters) != 14 {
		t.Fatalf("loaded %d filters, want 14", len(filters))
	}
	warm, ok := findImageFilter(filters, "warm")
	if !ok || warm.filterGraph == "" || warm.description == "" || len(warm.tags) == 0 {
		t.Fatalf("warm filter was not fully loaded: %+v", warm)
	}
	for _, input := range []string{
		`{"version":2,"filters":[{"id":"x","description":"x","filterGraph":"eq","tags":["x"]}]}`,
		`{"version":1,"filters":[{"id":"x","description":"x","filterGraph":"eq","tags":["x"]},{"id":"x","description":"duplicate","filterGraph":"eq","tags":["x"]}]}`,
		`{"version":1,"filters":[{"id":"x","description":"x","filterGraph":"","tags":["x"]}]}`,
	} {
		if _, err := loadImageFilterCatalog([]byte(input)); err == nil {
			t.Errorf("invalid catalog accepted: %s", input)
		}
	}
}

func TestImageFilterAllGeneratesOneNamedOutputPerFilter(t *testing.T) {
	// Batch mode runs every named filter and derives a distinct output filename from the supplied base.
	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	input := filepath.Join(dir, "source.png")
	if err := os.WriteFile(input, []byte("sample image"), 0600); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "ffmpeg-args.txt")
	if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte("#!/bin/sh\nprintf '640x480\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ffmpeg := "#!/bin/sh\nfor arg do printf '%s\\n' \"$arg\" >> \"$EVUFILTERS_TEST_ARG_LOG\"; last=\"$arg\"; done\n: > \"$last\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "ffmpeg"), []byte(ffmpeg), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", homeDir)
	t.Setenv("EVUFILTERS_TEST_ARG_LOG", logPath)
	var stdout, stderr bytes.Buffer
	if err := (imageFilterTool{}).Run([]string{"--filter", "all", "--file", input, "--output", "contact-sheet.png"}, &stdout, &stderr); err != nil {
		t.Fatalf("batch run returned error: %v; stderr: %s", err, stderr.String())
	}
	filters, err := loadEmbeddedImageFilters()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range filters {
		out := filepath.Join(homeDir, "evueditout", "contact-sheet-"+item.name+".png")
		if _, err := os.Stat(out); err != nil {
			t.Errorf("missing output for %s: %v", item.name, err)
		}
		if !strings.Contains(stdout.String(), "Wrote "+out) {
			t.Errorf("success output missing %s: %s", out, stdout.String())
		}
		if !strings.Contains(stderr.String(), "Applying \""+item.name+"\"") {
			t.Errorf("processing output missing filter %s: %s", item.name, stderr.String())
		}
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range filters {
		if !strings.Contains(string(args), item.graph()) {
			t.Errorf("FFmpeg arguments missing graph for %s", item.name)
		}
	}
}

func TestImageFilterPropertyCatalogPreservesFoodLooks(t *testing.T) {
	// The food-focused JSON entries preserve their supplied adjustment values and map all five controls to FFmpeg.
	filters, err := loadEmbeddedImageFilters()
	if err != nil {
		t.Fatal(err)
	}
	masala, ok := findImageFilter(filters, "masala-glow")
	if !ok || masala.properties == nil {
		t.Fatalf("masala-glow property definition missing: %+v", masala)
	}
	if got := *masala.properties; got != (imageFilterProperties{ExposureEV: 0.15, Contrast: 1.08, Saturation: 1.10, Warmth: 0.12, Texture: 0.10}) {
		t.Fatalf("masala-glow properties = %+v", got)
	}
	graph := masala.graph()
	for _, expected := range []string{"exposure=exposure=0.15", "eq=contrast=1.08:saturation=1.1", "colorbalance=", "unsharp=5:5:0.1"} {
		if !strings.Contains(graph, expected) {
			t.Errorf("masala-glow graph missing %q: %s", expected, graph)
		}
	}
	velvet, ok := findImageFilter(filters, "dessert-velvet")
	if !ok || !strings.Contains(velvet.graph(), "unsharp=5:5:-0.03") {
		t.Errorf("negative texture should map to slight softening, graph=%q", velvet.graph())
	}
}

func TestImageResizePresetsIncludePlatformDimensions(t *testing.T) {
	// Platform resize aliases must stay mapped to the exact requested canvases.
	for name, want := range map[string]imageResizeSize{
		"youtube":               {width: 1280, height: 720},
		"instagram":             {width: 1080, height: 1920},
		"facebook":              {width: 1080, height: 1920},
		"facebook-video":        {width: 1280, height: 720},
		"facebook-feed":         {width: 1080, height: 1350},
		"facebook-square":       {width: 1080, height: 1080},
		"facebook-reels":        {width: 1080, height: 1920},
		"facebook-link-preview": {width: 1200, height: 630},
	} {
		if got := imageResizeSizes[name]; got != want {
			t.Errorf("resize %s = %+v, want %+v", name, got, want)
		}
	}
}

func TestImageFilterAllChecksEveryOutputBeforeProcessing(t *testing.T) {
	// A collision in any batch target must fail before FFmpeg runs, avoiding a partial batch from a known collision.
	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	input := filepath.Join(dir, "source.png")
	if err := os.WriteFile(input, []byte("sample image"), 0600); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(homeDir, "evueditout")
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		t.Fatal(err)
	}
	collision := filepath.Join(outputDir, "batch-vivid.png")
	if err := os.WriteFile(collision, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "ffmpeg-args.txt")
	if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte("#!/bin/sh\nprintf '640x480\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "ffmpeg"), []byte("#!/bin/sh\nprintf called >> \"$EVUFILTERS_TEST_ARG_LOG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", homeDir)
	t.Setenv("EVUFILTERS_TEST_ARG_LOG", logPath)
	var stdout, stderr bytes.Buffer
	err := (imageFilterTool{}).Run([]string{"--filter", "all", "--file", input, "--output", "batch.png"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("collision error = %v", err)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("FFmpeg should not run when a batch target already exists (stat error: %v)", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "batch-cool.png")); !os.IsNotExist(err) {
		t.Fatalf("batch unexpectedly generated output before preflight completed (stat error: %v)", err)
	}
}

func TestImageFilterToolValidatesFilterPathsAndFormats(t *testing.T) {
	// Unsupported names, missing parameters, unsafe paths, invalid formats, and path conflicts fail before FFmpeg.
	var stdout, stderr bytes.Buffer
	err := (imageFilterTool{}).Run([]string{"--filter", "unknown"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unknown image filter") {
		t.Fatalf("unknown filter error = %v", err)
	}
	err = (imageFilterTool{}).Run([]string{"--filter", "warm", "--file", "image.png"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--file and --output are required") {
		t.Fatalf("missing output error = %v", err)
	}
	err = (imageFilterTool{}).Run([]string{"--filter", "warm", "--file", "image.gif", "--output", "out.png"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unsupported input image format") {
		t.Fatalf("unsupported input error = %v", err)
	}
	err = (imageFilterTool{}).Run([]string{"--filter", "warm", "--file", "image.png", "--output", "../out.png"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "filename only") {
		t.Fatalf("unsafe output error = %v", err)
	}
	err = (imageFilterTool{}).Run([]string{"--filter", "warm", "--file", "image.png", "--output", "out.png", "--resize", "tiny"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unknown resize size") {
		t.Fatalf("unknown resize error = %v", err)
	}
	err = (imageFilterTool{}).Run([]string{"--filter", "warm", "--file", "image.png", "--output", "out.png", "--quality", "101"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "between 1 and 100") {
		t.Fatalf("quality bounds error = %v", err)
	}
	err = (imageFilterTool{}).Run([]string{"--filter", "warm", "--file", "image.png", "--output", "out.png", "--format", "webp"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "conflicts with --format") {
		t.Fatalf("format mismatch error = %v", err)
	}
	err = (imageFilterTool{}).Run([]string{"--filter", "warm", "--file", "image.png", "--output", "out", "--format", "png", "--quality", "80"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "PNG is lossless") {
		t.Fatalf("PNG quality error = %v", err)
	}
}

func TestImageFilterToolRunsFFmpegAndDoesNotOverwrite(t *testing.T) {
	// Fake FFmpeg/ffprobe programs verify filter arguments, output formatting, and the fixed output directory.
	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	input := filepath.Join(dir, "source.png")
	if err := os.WriteFile(input, []byte("sample image"), 0600); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "ffmpeg-args.txt")
	ffprobe := "#!/bin/sh\nprintf '640x480\\n'\n"
	ffmpeg := "#!/bin/sh\nfor arg do printf '%s\\n' \"$arg\" >> \"$EVUFILTERS_TEST_ARG_LOG\"; last=\"$arg\"; done\n: > \"$last\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte(ffprobe), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "ffmpeg"), []byte(ffmpeg), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", homeDir)
	t.Setenv("EVUFILTERS_TEST_ARG_LOG", logPath)
	var stdout, stderr bytes.Buffer
	if err := (imageFilterTool{}).Run([]string{"--filter", "warm", "--file", input, "--output", "warm", "--format", "webp", "--quality", "75", "--resize", "facebook"}, &stdout, &stderr); err != nil {
		t.Fatalf("filter run returned error: %v; stderr: %s", err, stderr.String())
	}
	output := filepath.Join(homeDir, "evueditout", "warm.webp")
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("fake FFmpeg did not create output: %v", err)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	argText := string(args)
	for _, expected := range []string{"colorbalance=", "scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920", "-frames:v", "1", "libwebp", "-quality", "75", output, input} {
		if !strings.Contains(argText, expected) {
			t.Errorf("FFmpeg arguments missing %q: %s", expected, argText)
		}
	}
	if !strings.Contains(stdout.String(), "Wrote "+output) {
		t.Errorf("success output missing output path: %s", stdout.String())
	}
	if err := (imageFilterTool{}).Run([]string{"--filter", "warm", "--file", input, "--output", "warm", "--format", "webp", "--quality", "75", "--resize", "facebook"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output error = %v", err)
	}
}
