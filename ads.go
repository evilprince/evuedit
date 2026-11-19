package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed ads_templates.json
var adsConfig embed.FS

type adsAsset struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type adsCampaign struct {
	Name   string     `json:"name"`
	Assets []adsAsset `json:"assets"`
}
type adsCatalog struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Output   struct {
		Format           string `json:"format"`
		Quality          int    `json:"quality"`
		ColorSpace       string `json:"colorSpace"`
		MaxFileSizeMB    int    `json:"maxFileSizeMB"`
		PreserveOriginal bool   `json:"preserveOriginal"`
	} `json:"output"`
	Campaigns    map[string]adsCampaign `json:"campaigns"`
	FixedDisplay struct {
		MaxFileSizeKB int        `json:"maxFileSizeKB"`
		Sizes         []adsAsset `json:"sizes"`
	} `json:"fixed_display"`
}
type adsTool struct{}

func (adsTool) Name() string { return "evuads" }
func (adsTool) Description() string {
	return "create Google Ads image assets from JSON campaign size definitions"
}

func loadAdsCatalog() (adsCatalog, error) {
	b, err := adsConfig.ReadFile("ads_templates.json")
	if err != nil {
		return adsCatalog{}, err
	}
	var c adsCatalog
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse ads_templates.json: %w", err)
	}
	if c.Version != "1.0" || c.Platform != "google_ads" || c.Output.Format != "jpg" || c.Output.Quality < 1 || c.Output.Quality > 100 || c.Output.ColorSpace != "sRGB" || c.Output.MaxFileSizeMB < 1 || len(c.Campaigns) == 0 {
		return c, errors.New("ads_templates.json has an unsupported version or invalid output/campaign configuration")
	}
	for id, campaign := range c.Campaigns {
		if len(campaign.Assets) == 0 {
			return c, fmt.Errorf("campaign %q has no assets", id)
		}
		seen := map[string]bool{}
		for _, a := range campaign.Assets {
			if a.ID == "" || seen[a.ID] || (a.Type != "image" && a.Type != "logo") || a.Width < 1 || a.Height < 1 {
				return c, fmt.Errorf("campaign %q has an invalid or duplicate asset %q", id, a.ID)
			}
			seen[a.ID] = true
		}
	}
	return c, nil
}

func (adsTool) Run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("evuads", flag.ContinueOnError)
	fs.SetOutput(stderr)
	platform := fs.String("platform", "", "ad platform (google)")
	adtype := fs.String("adtype", "", "campaign/ad type from ads_templates.json")
	input := fs.String("file", "", "source image path")
	logo := fs.String("logo", "", "optional logo image path; renders logo asset sizes")
	outDir := fs.String("output-dir", "", "output directory (default ~/evueditout)")
	list := fs.Bool("list", false, "list supported Google Ads types")
	fs.Usage = func() { printAdsHelp(stdout) }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	catalog, err := loadAdsCatalog()
	if err != nil {
		return err
	}
	if *list {
		for id, c := range catalog.Campaigns {
			fmt.Fprintf(stdout, "%s\t%s\n", id, c.Name)
		}
		fmt.Fprintln(stdout, "fixed_display\tFixed-size Display Ads")
		return nil
	}
	if *platform != "google" {
		return errors.New("--platform must be google")
	}
	if *adtype == "" || *input == "" {
		return errors.New("--platform google, --adtype, and --file are required")
	}
	assets, ok := catalog.Campaigns[*adtype]
	if *adtype == "fixed_display" {
		assets = adsCampaign{Name: "Fixed-size Display Ads"}
		for _, size := range catalog.FixedDisplay.Sizes {
			size.Type = "image"
			assets.Assets = append(assets.Assets, size)
		}
		ok = len(assets.Assets) > 0
	}
	if !ok {
		return fmt.Errorf("unknown --adtype %q; use --list to see available types", *adtype)
	}
	inputPath, err := filepath.Abs(*input)
	if err != nil {
		return err
	}
	if err = checkAdsInput(inputPath); err != nil {
		return err
	}
	logoPath := ""
	if *logo != "" {
		logoPath, err = filepath.Abs(*logo)
		if err != nil {
			return err
		}
		if err = checkAdsInput(logoPath); err != nil {
			return err
		}
	}
	stem := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	dir := *outDir
	if dir == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		dir = filepath.Join(home, "evueditout")
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create output directory %q: %w", dir, err)
	}
	type job struct {
		asset         adsAsset
		input, output string
	}
	jobs := []job{}
	for _, a := range assets.Assets {
		src := inputPath
		if a.Type == "logo" {
			if logoPath == "" {
				continue
			}
			src = logoPath
		}
		out := filepath.Join(dir, fmt.Sprintf("%s_%s_%s.%s", stem, a.ID, *adtype, catalog.Output.Format))
		if filepath.Clean(src) == filepath.Clean(out) {
			return errors.New("input and output paths must differ")
		}
		if _, e := os.Stat(out); e == nil {
			return fmt.Errorf("output %q already exists", out)
		} else if !os.IsNotExist(e) {
			return fmt.Errorf("check output %q: %w", out, e)
		}
		jobs = append(jobs, job{a, src, out})
	}
	if logoPath == "" {
		for _, a := range assets.Assets {
			if a.Type == "logo" {
				fmt.Fprintln(stderr, "Logo asset sizes skipped; pass --logo to generate them.")
				break
			}
		}
	}
	if len(jobs) == 0 {
		return errors.New("no assets selected for rendering")
	}
	magick, err := findImageMagick()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	for _, j := range jobs {
		args := []string{j.input, "-auto-orient", "-resize", fmt.Sprintf("%dx%d^", j.asset.Width, j.asset.Height), "-gravity", "center", "-extent", fmt.Sprintf("%dx%d", j.asset.Width, j.asset.Height), "-colorspace", "sRGB", "-quality", fmt.Sprint(catalog.Output.Quality), j.output}
		if err := runThumbCommand(ctx, magick, args...); err != nil {
			return fmt.Errorf("render %s: %w", j.asset.ID, err)
		}
		info, e := os.Stat(j.output)
		if e != nil {
			return e
		}
		maxSize := int64(catalog.Output.MaxFileSizeMB) * 1024 * 1024
		if *adtype == "fixed_display" {
			maxSize = int64(catalog.FixedDisplay.MaxFileSizeKB) * 1024
		}
		if info.Size() > maxSize {
			os.Remove(j.output)
			return fmt.Errorf("%s exceeds configured file size limit; reduce quality or dimensions", filepath.Base(j.output))
		}
		fmt.Fprintf(stdout, "Wrote %s\n", j.output)
	}
	return nil
}

func checkAdsInput(path string) error {
	if !supportedImageExtension(strings.ToLower(filepath.Ext(path))) {
		return fmt.Errorf("unsupported image format %q; use .jpg, .jpeg, .png, or .webp", filepath.Ext(path))
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read input image %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("input image %q is not a regular file", path)
	}
	return nil
}
func printAdsHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: evuedit --module evuads --platform google --adtype <campaign> --file <image> [--logo <image>] [--output-dir <dir>]")
	fmt.Fprintln(w, "Create configured Google Ads image assets. Output names: <source>_<size>_<adtype>.jpg")
	fmt.Fprintln(w, "Logo sizes are generated when --logo is supplied. Default output directory: ~/evueditout.")
	fmt.Fprintln(w, "Use --list to list campaign types.")
}
