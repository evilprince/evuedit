package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMotionTemplateCatalogValidation(t *testing.T) {
	// The embedded catalog must expose valid versioned settings and the expected starter template.
	catalog, err := loadMotionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Templates) != 1 || catalog.Templates[0].ID != "food-promo" || catalog.Templates[0].Canvas != (motionCanvas{Width: 1080, Height: 1920}) {
		t.Fatalf("unexpected motion catalog: %+v", catalog)
	}
}

func TestMotionFilterGraphSelectsTemplateAnimations(t *testing.T) {
	// Template-selected text effects and separate subject motion must appear in the FFmpeg graph.
	catalog, err := loadMotionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tmpl := catalog.Templates[0]
	graph := motionFilterGraph(tmpl, "/tmp/headline.txt", true)
	for _, want := range []string{"scale=1080:1920", "overlay=x='(W-w)/2':y='(H-h)/2+18*sin(t*2)'", "drawtext=font='Sans'", "textfile='/tmp/headline.txt'", "alpha='if(lt(t\\,0.400)"} {
		if !strings.Contains(graph, want) {
			t.Errorf("filter graph missing %q: %s", want, graph)
		}
	}
}

func TestMotionToolRunsFFmpegAndWritesOutput(t *testing.T) {
	// A fake FFmpeg records arguments and creates the expected file without encoding video.
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	input := filepath.Join(dir, "photo.jpg")
	foreground := filepath.Join(dir, "cutout.png")
	if err := os.WriteFile(input, []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreground, []byte("png"), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "args.log")
	ffmpeg := filepath.Join(bin, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$EVUMOTION_TEST_LOG\"\nfor last do :; done\nprintf video > \"$last\"\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EVUMOTION_TEST_LOG", log)
	t.Setenv("PATH", bin)
	var stdout, stderr bytes.Buffer
	err := (motionTool{}).Run([]string{"--template", "food-promo", "--platform", "facebook", "--file", input, "--foreground", foreground, "--headline", "FRESH FROM THE GRILL", "--output", "promo.mp4"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run returned %v; stderr=%s", err, stderr.String())
	}
	args, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-filter_complex", "overlay=", "drawtext=", "format=yuv420p", "1080:1920", "libx264", "yuv420p", "-color_range", "tv", "-an", "promo.mp4"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("FFmpeg args missing %q: %s", want, args)
		}
	}
	if _, err = os.Stat(filepath.Join(home, "evueditout", "promo.mp4")); err != nil {
		t.Fatalf("output was not created under evueditout: %v", err)
	}
}

func TestMotionToolValidatesArgumentsAndOutput(t *testing.T) {
	// Missing required options, invalid platform/extension, and output path traversal must fail before FFmpeg starts.
	var stdout, stderr bytes.Buffer
	cases := [][]string{{"--template", "food-promo"}, {"--template", "food-promo", "--platform", "tiktok", "--file", "x.jpg", "--headline", "x", "--output", "x.mp4"}, {"--template", "food-promo", "--platform", "youtube", "--file", "x.jpg", "--headline", "x", "--output", "../x.mp4"}, {"--template", "food-promo", "--platform", "youtube", "--file", "x.jpg", "--headline", "x", "--output", "x.mov"}}
	for _, args := range cases {
		if err := (motionTool{}).Run(args, &stdout, &stderr); err == nil {
			t.Errorf("Run(%v) succeeded, want validation error", args)
		}
	}
}
