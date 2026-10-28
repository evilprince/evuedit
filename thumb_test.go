package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThumbTemplateCatalogValidation(t *testing.T) {
	// The versioned template catalog must retain platform canvases and reject malformed versions and duplicate IDs.
	catalog, err := loadEmbeddedThumbTemplateCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Templates) != 10 {
		t.Fatalf("loaded %d templates, want 10", len(catalog.Templates))
	}
	item, ok := findThumbTemplate(catalog, "street-food")
	if !ok || item.Canvas["youtube"] != (thumbCanvas{Width: 1280, Height: 720}) || item.Canvas["instagram"] != (thumbCanvas{Width: 1080, Height: 1920}) || item.Layout.LabelText != "STREET FOOD" {
		t.Fatalf("street-food canvases are incorrect: %+v", item.Canvas)
	}
	for _, id := range []string{"cafe-coffee", "dessert", "fine-dining", "food-challenge", "food-comparison", "food-deal", "instagram-cover", "restaurant-review", "street-food", "youtube-review"} {
		if _, ok := findThumbTemplate(catalog, id); !ok {
			t.Errorf("imported template %q is missing from the runtime catalog", id)
		}
	}
	for _, input := range []string{
		`{"version":2,"templates":[{}]}`,
		`{"version":1,"templates":[{"id":"duplicate"},{"id":"duplicate"}]}`,
	} {
		if _, err := loadThumbTemplateCatalog([]byte(input)); err == nil {
			t.Errorf("invalid template catalog accepted: %s", input)
		}
	}
}

func TestImportedThumbTemplatesRequireTheirDynamicInputs(t *testing.T) {
	// Templates that render data badges or two photos require the corresponding CLI parameters.
	input := filepath.Join(t.TempDir(), "dish.jpg")
	if err := os.WriteFile(input, []byte("placeholder"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	base := []string{"--platform", "youtube", "--file", input, "--headline", "A GREAT FOOD REVIEW", "--output", "thumb.jpg"}
	for _, test := range []struct{ template, want string }{
		{"food-comparison", "requires --compare-file"},
		{"food-deal", "requires --price"},
		{"restaurant-review", "requires --secondary-text"},
	} {
		args := append([]string{"--template", test.template}, base...)
		if err := (thumbTool{}).Run(args, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("template %s error = %v, want %q", test.template, err, test.want)
		}
	}
}

func TestThumbHeadlineWrappingAndBatchNames(t *testing.T) {
	// Headline words wrap without being dropped, and batch filenames are numbered predictably.
	lines := wrapThumbWords(strings.Fields("THE BEST STREET FOOD"), 150, 40)
	if len(lines) < 2 || strings.Join(lines, " ") != "THE BEST STREET FOOD" {
		t.Fatalf("wrapped lines = %#v", lines)
	}
	if got := thumbOutputNames("street-food.jpg", 2); len(got) != 2 || got[0] != "street-food-001.jpg" || got[1] != "street-food-002.jpg" {
		t.Fatalf("batch output names = %#v", got)
	}
	if got := thumbOutputNames("one.jpg", 1); len(got) != 1 || got[0] != "one.jpg" {
		t.Fatalf("single output name = %#v", got)
	}
}

func TestThumbToolHelpListAndHeadlineValidation(t *testing.T) {
	// Help and list expose the module; headline selection and template limits are checked before ImageMagick runs.
	var stdout, stderr bytes.Buffer
	if err := (thumbTool{}).Run([]string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "--headlines-file") || !strings.Contains(stdout.String(), "street-food") {
		t.Fatalf("help output is incomplete: %s", stdout.String())
	}
	stdout.Reset()
	if err := (thumbTool{}).Run([]string{"--list"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "street-food") {
		t.Fatalf("template list is incomplete: %s", stdout.String())
	}
	err := (thumbTool{}).Run([]string{"--template", "street-food", "--platform", "youtube", "--file", "dish.jpg", "--headline", "THE BEST STREET FOOD TOUR", "--headlines-file", "headlines.txt", "--output", "out.jpg"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("conflicting headline sources error = %v", err)
	}
	err = (thumbTool{}).Run([]string{"--template", "street-food", "--platform", "youtube", "--file", "dish.jpg", "--headline", "THE BEST STREET FOOD TOUR RIGHT NOW", "--output", "out.jpg"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "allows at most 5") {
		t.Fatalf("headline word limit error = %v", err)
	}
}

func TestReadThumbHeadlines(t *testing.T) {
	// Batch headline files trim whitespace and ignore blank lines while preserving line order.
	path := filepath.Join(t.TempDir(), "headlines.txt")
	if err := os.WriteFile(path, []byte(" First headline \n\nSecond headline\n  \n"), 0600); err != nil {
		t.Fatal(err)
	}
	headlines, err := readThumbHeadlines("", path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(headlines, "|") != "First headline|Second headline" {
		t.Fatalf("headlines = %#v", headlines)
	}
}

func TestThumbRenderingInvokesImageMagickLayers(t *testing.T) {
	// A fake ImageMagick executable verifies cover crop, gradient composition, headline drawing, and export settings.
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "magick-args.txt")
	magick := "#!/bin/sh\nif [ \"$1\" = \"-version\" ]; then echo 'ImageMagick 7 fake'; exit 0; fi\nfor arg do printf '%s\\n' \"$arg\" >> \"$EVUTHUMB_TEST_ARG_LOG\"; last=\"$arg\"; if [ \"$arg\" = \"info:\" ]; then printf '300 100'; exit 0; fi; done\n: > \"$last\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "magick"), []byte(magick), 0700); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "dish.png")
	if err := os.WriteFile(input, []byte("fake source"), 0600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)
	t.Setenv("EVUTHUMB_TEST_ARG_LOG", logPath)
	t.Setenv("EVU_IMAGEMAGICK_PATH", "")
	var stdout, stderr bytes.Buffer
	err := (thumbTool{}).Run([]string{"--template", "street-food", "--platform", "youtube", "--file", input, "--headline", "THE BEST STREET FOOD", "--output", "thumbnail.jpg"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("thumbnail render error: %v, stderr: %s", err, stderr.String())
	}
	output := filepath.Join(home, "evueditout", "thumbnail.jpg")
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("thumbnail output missing: %v", err)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"1280x720^", "-extent", "1280x720", "-compose", "over", "THE BEST", "-quality", "92", "sRGB", output} {
		if !strings.Contains(string(args), expected) {
			t.Errorf("ImageMagick invocation missing %q: %s", expected, args)
		}
	}
	if !strings.Contains(stdout.String(), "Wrote "+output) {
		t.Errorf("success output missing output path: %s", stdout.String())
	}
}
