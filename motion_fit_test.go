package main

import (
	"strings"
	"testing"
)

func TestFitMotionHeadlineWrapsAndPreservesWords(t *testing.T) {
	tmpl, err := loadMotionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	headline := "A LONG HEADLINE THAT SHOULD WRAP CLEANLY AT WORD BOUNDARIES"
	fitted, size, err := fitMotionHeadline(headline, &tmpl.Templates[0])
	if err != nil {
		t.Fatalf("fitMotionHeadline returned error: %v", err)
	}
	if !strings.Contains(fitted, "\n") {
		t.Fatalf("headline was not wrapped: %q", fitted)
	}
	if size < tmpl.Templates[0].Headline.FontSize/2 || size > tmpl.Templates[0].Headline.FontSize {
		t.Fatalf("fitted font size %d outside supported range", size)
	}
	if strings.Join(strings.Fields(fitted), " ") != strings.Join(strings.Fields(headline), " ") {
		t.Fatalf("wrapped text changed headline words: got %q", fitted)
	}
	maxWidth := float64(tmpl.Templates[0].Canvas.Width) * 0.80
	for _, line := range strings.Split(fitted, "\n") {
		if got := motionTextWidthUnits(line) * float64(size) * 1.15; got > maxWidth+0.01 {
			t.Errorf("line %q estimated width %.1f exceeds safe width %.1f", line, got, maxWidth)
		}
	}
}

func TestFitMotionHeadlineReducesFontSizeForTallCopy(t *testing.T) {
	catalog, err := loadMotionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	words := make([]string, 0, 72)
	for i := 0; i < 36; i++ {
		words = append(words, "FRESH", "FOOD")
	}
	fitted, size, err := fitMotionHeadline(strings.Join(words, " "), &catalog.Templates[0])
	if err != nil {
		t.Fatalf("fitMotionHeadline returned error: %v", err)
	}
	if size >= catalog.Templates[0].Headline.FontSize {
		t.Fatalf("font size remained %d; expected reduction for long headline", size)
	}
	lines := strings.Split(fitted, "\n")
	if float64(len(lines))*float64(size)*1.35 > float64(catalog.Templates[0].Canvas.Height)*0.82-180 {
		t.Fatalf("fitted text block exceeds available height: %d lines at %d px", len(lines), size)
	}
}

func TestFitMotionHeadlineRejectsTextThatCannotFit(t *testing.T) {
	catalog, err := loadMotionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tmpl := catalog.Templates[0]
	tmpl.Canvas = motionCanvas{Width: 320, Height: 240}
	_, _, err = fitMotionHeadline(strings.Repeat("VERY LONG ", 100), &tmpl)
	if err == nil || !strings.Contains(err.Error(), "shorten the headline") {
		t.Fatalf("error = %v, want an actionable overflow error", err)
	}
}

func TestMotionFilterGraphUsesFittedSizeAndBoundsCorrection(t *testing.T) {
	catalog, err := loadMotionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tmpl := catalog.Templates[0]
	tmpl.Headline.Animation = "fade"
	graph := motionFilterGraphAtSize(tmpl, "/tmp/headline.txt", false, 64)
	if !strings.Contains(graph, "fontsize=64:") || !strings.Contains(graph, ":fix_bounds=1") {
		t.Fatalf("filter graph missing fitted font size or bounds correction: %s", graph)
	}
	tmpl.Headline.Animation = "pop"
	graph = motionFilterGraphAtSize(tmpl, "/tmp/headline.txt", false, 64)
	if !strings.Contains(graph, "fontsize=64*min(") {
		t.Fatalf("pop animation did not use fitted font size: %s", graph)
	}
}
