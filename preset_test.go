package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPresetToolHelpListsAvailablePresets(t *testing.T) {
	// Tool help must show the syntax and every preset so users can discover valid names.
	var stdout, stderr bytes.Buffer
	err := (presetTool{}).Run([]string{"--help"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("help returned error: %v", err)
	}
	for _, expected := range append([]string{"--apply <presetName>", "--file <inputFile>", "--output <outputFile.mp4>", "--input-profile <profile>", "--profile-lut <file>", "--list-use-cases", "--use-case <id>", "Built-in presets and parameters", "exposureEV=", "contrast=", "saturation=", "highlights=", "shadows=", "warmth=", "lutIntensity=", "portrait", ".mp4"}, presetNames(catalogForTest(t))...) {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("help output does not contain %q", expected)
		}
	}
}

func TestPresetCatalogLoadsVersionedSamsungLogPresets(t *testing.T) {
	// The embedded catalog should preserve the declared color pipeline and every named preset parameter.
	catalog, err := loadPresetCatalog()
	if err != nil {
		t.Fatalf("loadPresetCatalog returned error: %v", err)
	}
	if catalog.Version != 2 || catalog.InputProfile != "samsung-log" || catalog.WorkingOutput != "rec709-sdr" {
		t.Fatalf("catalog header = version %d, input %q, output %q", catalog.Version, catalog.InputProfile, catalog.WorkingOutput)
	}
	wantNames := []string{"cafe-cream", "chefs-table", "cinematic-warm", "cool", "masala-gold", "midnight-diner", "monochrome", "neutral-contrast", "street-food-pop"}
	if !reflect.DeepEqual(presetNames(catalog), wantNames) {
		t.Fatalf("preset names = %#v, want %#v", presetNames(catalog), wantNames)
	}
	masala, ok := findPreset(catalog, "masala-gold")
	if !ok || masala.ExposureEV != 0 || masala.Contrast != 1.08 || masala.Saturation != 1.08 || masala.Highlights != -0.12 || masala.Shadows != 0.12 || masala.Warmth != 0.18 || masala.LUTIntensity != 0 {
		t.Fatalf("masala-gold parameters were not preserved: %#v", masala)
	}
}

func TestUseCaseCatalogMatchesPresetsByTags(t *testing.T) {
	// Separate tag definitions connect use cases to preset recommendations without coupling use-case names to preset IDs.
	presets := catalogForTest(t)
	useCases, err := loadUseCaseCatalog()
	if err != nil {
		t.Fatalf("loadUseCaseCatalog returned error: %v", err)
	}
	if err := validatePresetTagReferences(presets, useCases); err != nil {
		t.Fatalf("catalog tag references are invalid: %v", err)
	}
	food, ok := findUseCase(useCases, "food-video")
	if !ok {
		t.Fatal("food-video use case not found")
	}
	var got []string
	for _, item := range presetsForUseCase(presets, food) {
		got = append(got, item.ID)
	}
	want := []string{"chefs-table", "masala-gold", "midnight-diner", "street-food-pop"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("food-video matches = %#v, want %#v", got, want)
	}
	lowLight, _ := findUseCase(useCases, "low-light-dining")
	got = got[:0]
	for _, item := range presetsForUseCase(presets, lowLight) {
		got = append(got, item.ID)
	}
	if !reflect.DeepEqual(got, []string{"midnight-diner"}) {
		t.Fatalf("low-light-dining matches = %#v, want only midnight-diner", got)
	}
	if err := validatePresetTagReferences(presets, useCaseCatalog{Tags: []presetTag{{ID: "known", Description: "known tag"}}}); err == nil || !strings.Contains(err.Error(), "unknown tag") {
		t.Fatalf("unknown preset tag validation error = %v", err)
	}
}

func TestPresetToolListsUseCasesAndRecommendations(t *testing.T) {
	// Users can discover use-case IDs, then request deterministic tag-based preset recommendations.
	var stdout, stderr bytes.Buffer
	if err := (presetTool{}).Run([]string{"--list-use-cases"}, &stdout, &stderr); err != nil {
		t.Fatalf("list use cases: %v", err)
	}
	if !strings.Contains(stdout.String(), "food-video") || !strings.Contains(stdout.String(), "food") {
		t.Fatalf("use-case listing missing IDs or tags: %s", stdout.String())
	}
	stdout.Reset()
	if err := (presetTool{}).Run([]string{"--use-case", "food-video"}, &stdout, &stderr); err != nil {
		t.Fatalf("show food recommendations: %v", err)
	}
	for _, expected := range []string{"Food video", "masala-gold", "street-food-pop"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("recommendations missing %q: %s", expected, stdout.String())
		}
	}
	if err := (presetTool{}).Run([]string{"--use-case", "unknown-case"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "unknown use case") {
		t.Fatalf("unknown use case error = %v", err)
	}
	if err := (presetTool{}).Run([]string{"--list", "--list-use-cases"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "use only one") {
		t.Fatalf("conflicting discovery options error = %v", err)
	}
}

func TestPresetFilterMapsCreativeControls(t *testing.T) {
	// The generated FFmpeg graph must apply the profile transform before creative adjustments.
	catalog := catalogForTest(t)
	cafe, _ := findPreset(catalog, "cafe-cream")
	filter, err := buildPresetFilter(cafe, "samsung-log", "rec709-sdr", "/tmp/samsung.cube")
	if err != nil {
		t.Fatalf("buildPresetFilter returned error: %v", err)
	}
	if !strings.HasPrefix(filter, "lut3d=file='") || !strings.Contains(filter, "exposure=exposure=0.15") || !strings.Contains(filter, "eq=contrast=0.96:saturation=0.97") || !strings.Contains(filter, "curves=master=") || !strings.Contains(filter, "colorbalance=") {
		t.Fatalf("filter graph does not contain all expected stages: %s", filter)
	}
	rec709Filter, err := buildPresetFilter(cafe, "rec709", "rec709-sdr", "")
	if err != nil {
		t.Fatalf("Rec.709 input should not require a profile LUT: %v", err)
	}
	if strings.Contains(rec709Filter, "lut3d=") {
		t.Fatalf("Rec.709 input unexpectedly received a Samsung Log LUT: %s", rec709Filter)
	}
}

func TestPresetToolListsPresetsAndRequiresProfileLUT(t *testing.T) {
	// Users can discover the catalog, and Samsung Log work must not silently skip its technical conversion LUT.
	var stdout, stderr bytes.Buffer
	if err := (presetTool{}).Run([]string{"--list"}, &stdout, &stderr); err != nil {
		t.Fatalf("preset listing returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "masala-gold") || !strings.Contains(stdout.String(), "samsung-log") || !strings.Contains(stdout.String(), "tags:") {
		t.Fatalf("preset list is missing profile or preset details: %s", stdout.String())
	}
	err := (presetTool{}).Run([]string{"--apply", "masala-gold", "--file", "input.mov", "--output", "output.mov"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--profile-lut") {
		t.Fatalf("missing profile LUT error = %v", err)
	}
}

func TestPresetToolValidatesPresetAndRequiredPaths(t *testing.T) {
	// Bad preset names and missing required paths must fail before launching FFmpeg.
	var stdout, stderr bytes.Buffer
	err := (presetTool{}).Run([]string{"--apply", "unknown"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unknown preset") {
		t.Fatalf("unknown preset error = %v", err)
	}
	err = (presetTool{}).Run([]string{"--apply", "masala-gold", "--file", "clip.mov"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--file and --output are required") {
		t.Fatalf("missing output error = %v", err)
	}
}

func TestPresetToolRejectsLandscapeAndNonMP4Output(t *testing.T) {
	// Shorts jobs require portrait source footage and use the MP4 delivery container.
	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", homeDir)
	input := filepath.Join(dir, "landscape.mp4")
	if err := os.WriteFile(input, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte("#!/bin/sh\nprintf '1920x1080\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "ffmpeg"), []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	args := []string{"--apply", "masala-gold", "--input-profile", "rec709", "--file", input, "--output", "short.mp4"}
	var stdout, stderr bytes.Buffer
	err := (presetTool{}).Run(args, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "invalid format") || !strings.Contains(err.Error(), "1920x1080") {
		t.Fatalf("landscape input error = %v", err)
	}
	args[len(args)-1] = "short.mov"
	err = (presetTool{}).Run(args, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), ".mp4 extension") {
		t.Fatalf("non-MP4 output error = %v", err)
	}
}

func TestPresetToolRejectsUnsafeOrUnavailablePaths(t *testing.T) {
	// Output names are constrained to ~/evueditout; the input must exist and outputs must not be clobbered.
	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	outputDir := filepath.Join(homeDir, "evueditout")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", homeDir)
	input := filepath.Join(outputDir, "input.mp4")
	output := "output.mp4"
	lut := filepath.Join(dir, "profile.cube")
	if err := os.WriteFile(input, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lut, []byte("LUT_3D_SIZE 2\n0 0 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	baseArgs := []string{"--apply", "masala-gold", "--file", input, "--profile-lut", lut}
	if err := (presetTool{}).Run(append(append([]string{}, baseArgs...), "--output", "input.mp4"), &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "must be different") {
		t.Fatalf("same-path error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, output), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (presetTool{}).Run(append(append([]string{}, baseArgs...), "--output", output), &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing-output error = %v", err)
	}
	if err := (presetTool{}).Run([]string{"--apply", "masala-gold", "--file", filepath.Join(dir, "missing.mp4"), "--output", "new.mp4", "--profile-lut", lut}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "read input") {
		t.Fatalf("missing-input error = %v", err)
	}
	if err := (presetTool{}).Run(append(append([]string{}, baseArgs...), "--output", filepath.Join(dir, "outside.mp4")), &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "filename only") {
		t.Fatalf("output directory error = %v", err)
	}
}

func TestPresetToolRunsFFmpegWithSelectedFilter(t *testing.T) {
	// A fake FFmpeg executable verifies argument construction and success handling without encoding media.
	dir := t.TempDir()
	input := filepath.Join(dir, "source.mov")
	homeDir := filepath.Join(dir, "home")
	output := "graded.mp4"
	outputPath := filepath.Join(homeDir, "evueditout", output)
	lut := filepath.Join(dir, "samsung-log-to-rec709.cube")
	logPath := filepath.Join(dir, "ffmpeg-args.txt")
	if err := os.WriteFile(input, []byte("fake video"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lut, []byte("fake lut"), 0600); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	fakeFFmpeg := filepath.Join(binDir, "ffmpeg")
	script := "#!/bin/sh\nfor arg do printf '%s\\n' \"$arg\" >> \"$EVUEDIT_TEST_ARG_LOG\"; last=\"$arg\"; done\n: > \"$last\"\n"
	if err := os.WriteFile(fakeFFmpeg, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	fakeFFprobe := filepath.Join(binDir, "ffprobe")
	if err := os.WriteFile(fakeFFprobe, []byte("#!/bin/sh\nprintf '1080x1920\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("EVUEDIT_TEST_ARG_LOG", logPath)
	t.Setenv("HOME", homeDir)

	var stdout, stderr bytes.Buffer
	err := (presetTool{}).Run([]string{"--apply", "masala-gold", "--file", input, "--output", output, "--profile-lut", lut}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("preset command returned error: %v; stderr: %s", err, stderr.String())
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("fake FFmpeg did not create output: %v", err)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake FFmpeg args: %v", err)
	}
	argText := string(args)
	if !strings.Contains(argText, "lut3d=file=") || !strings.Contains(argText, "samsung-log-to-rec709.cube") || !strings.Contains(argText, "eq=contrast=1.08:saturation=1.08") {
		t.Errorf("FFmpeg args do not contain the selected profile LUT and creative filter: %s", argText)
	}
	for _, expected := range []string{"libx264", "-crf", "20", "yuv420p", "aac", "128k", "+faststart", "-stats_period", "1"} {
		if !strings.Contains(argText, expected) {
			t.Errorf("FFmpeg args missing Shorts output setting %q: %s", expected, argText)
		}
	}
	if !strings.Contains(argText, input) || !strings.Contains(argText, outputPath) {
		t.Errorf("FFmpeg args do not contain input and output paths: %s", argText)
	}
	if !strings.Contains(stdout.String(), "Wrote "+outputPath) {
		t.Errorf("success output does not report output path: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(homeDir, "evueditout")); err != nil {
		t.Fatalf("output directory was not created: %v", err)
	}

	// A one-command Rec.709 override should skip the Samsung Log LUT for already corrected footage.
	if err := os.WriteFile(logPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	rec709Output := "graded-rec709.mp4"
	rec709Path := filepath.Join(homeDir, "evueditout", rec709Output)
	err = (presetTool{}).Run([]string{"--apply", "masala-gold", "--file", input, "--output", rec709Output, "--input-profile", "rec709"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Rec.709 override returned error: %v", err)
	}
	if _, err := os.Stat(rec709Path); err != nil {
		t.Fatalf("fake FFmpeg did not create Rec.709 output: %v", err)
	}
	args, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read Rec.709 FFmpeg args: %v", err)
	}
	if strings.Contains(string(args), "lut3d=") {
		t.Errorf("Rec.709 override unexpectedly passed a LUT: %s", args)
	}
}

func catalogForTest(t *testing.T) presetCatalog {
	t.Helper()
	catalog, err := loadPresetCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
