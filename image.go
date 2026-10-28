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
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

type imageFilterTool struct{}

func (imageFilterTool) Name() string        { return "evufilters" }
func (imageFilterTool) Description() string { return "apply a named filter to a still image" }

type imageFilter struct {
	name        string
	description string
	filterGraph string
	tags        []string
	properties  *imageFilterProperties
}

type imageFilterProperties struct {
	ExposureEV float64 `json:"exposureEV"`
	Contrast   float64 `json:"contrast"`
	Saturation float64 `json:"saturation"`
	Warmth     float64 `json:"warmth"`
	Texture    float64 `json:"texture"`
}

type imageResizeSize struct {
	width  int
	height int
}

var imageResizeSizes = map[string]imageResizeSize{
	"square":    {width: 1080, height: 1080},
	"portrait":  {width: 1080, height: 1350},
	"story":     {width: 1080, height: 1920},
	"landscape": {width: 1920, height: 1080},
	"youtube":   {width: 1280, height: 720},
	"instagram": {width: 1080, height: 1920},
}

//go:embed image_filters.json
var imageFilterConfig embed.FS

type imageFilterJSON struct {
	ID          string                 `json:"id"`
	Description string                 `json:"description"`
	FilterGraph string                 `json:"filterGraph"`
	Tags        []string               `json:"tags"`
	Properties  *imageFilterProperties `json:"properties"`
}

func loadImageFilterCatalog(data []byte) ([]imageFilter, error) {
	var catalog struct {
		Version int               `json:"version"`
		Filters []imageFilterJSON `json:"filters"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse image filter catalog: %w", err)
	}
	if catalog.Version != 1 {
		return nil, fmt.Errorf("unsupported image filter catalog version %d", catalog.Version)
	}
	if len(catalog.Filters) == 0 {
		return nil, errors.New("image filter catalog must define at least one filter")
	}
	filters := make([]imageFilter, 0, len(catalog.Filters))
	seen := make(map[string]bool, len(catalog.Filters))
	for _, item := range catalog.Filters {
		if strings.TrimSpace(item.ID) == "" || seen[item.ID] {
			return nil, fmt.Errorf("image filter catalog contains an empty or duplicate id %q", item.ID)
		}
		if strings.TrimSpace(item.Description) == "" || (strings.TrimSpace(item.FilterGraph) == "" && item.Properties == nil) {
			return nil, fmt.Errorf("image filter %q: description and either filterGraph or properties are required", item.ID)
		}
		if item.Properties != nil {
			if item.FilterGraph != "" {
				return nil, fmt.Errorf("image filter %q: define filterGraph or properties, not both", item.ID)
			}
			if err := validateImageFilterProperties(*item.Properties); err != nil {
				return nil, fmt.Errorf("image filter %q: %w", item.ID, err)
			}
		}
		if err := validateTagList("image filter "+item.ID, item.Tags); err != nil {
			return nil, err
		}
		seen[item.ID] = true
		filters = append(filters, imageFilter{name: item.ID, description: item.Description, filterGraph: item.FilterGraph, tags: item.Tags, properties: item.Properties})
	}
	return filters, nil
}

func validateImageFilterProperties(properties imageFilterProperties) error {
	if properties.ExposureEV < -5 || properties.ExposureEV > 5 {
		return errors.New("exposureEV must be between -5 and 5")
	}
	if properties.Contrast <= 0 || properties.Contrast > 3 {
		return errors.New("contrast must be greater than 0 and at most 3")
	}
	if properties.Saturation < 0 || properties.Saturation > 3 {
		return errors.New("saturation must be between 0 and 3")
	}
	if properties.Warmth < -1 || properties.Warmth > 1 {
		return errors.New("warmth must be between -1 and 1")
	}
	if properties.Texture < -1.5 || properties.Texture > 1.5 {
		return errors.New("texture must be between -1.5 and 1.5")
	}
	return nil
}

func (item imageFilter) graph() string {
	if item.properties == nil {
		return item.filterGraph
	}
	p := item.properties
	parts := []string{
		fmt.Sprintf("exposure=exposure=%g", p.ExposureEV),
		fmt.Sprintf("eq=contrast=%g:saturation=%g", p.Contrast, p.Saturation),
	}
	if p.Warmth != 0 {
		warmth := p.Warmth * 0.4
		parts = append(parts, fmt.Sprintf("colorbalance=rs=%g:gs=%g:bs=%g:rm=%g:gm=%g:bm=%g:rh=%g:gh=%g:bh=%g", warmth, -warmth*0.15, -warmth, warmth, -warmth*0.15, -warmth, warmth, -warmth*0.15, -warmth))
	}
	if p.Texture != 0 {
		parts = append(parts, fmt.Sprintf("unsharp=5:5:%g:5:5:0", p.Texture))
	}
	return strings.Join(parts, ",")
}

func loadEmbeddedImageFilters() ([]imageFilter, error) {
	data, err := imageFilterConfig.ReadFile("image_filters.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded image filter catalog: %w", err)
	}
	return loadImageFilterCatalog(data)
}

func (imageFilterTool) Run(args []string, stdout, stderr io.Writer) error {
	filters, err := loadEmbeddedImageFilters()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("evufilters", flag.ContinueOnError)
	fs.SetOutput(stderr)
	filterName := fs.String("filter", "", "image filter to apply, or all to generate every filter")
	input := fs.String("file", "", "input image file")
	output := fs.String("output", "", "output image filename (written to ~/evueditout)")
	resize := fs.String("resize", "", "resize preset: square, portrait, story, or landscape (center-crops to exact dimensions)")
	format := fs.String("format", "", "output format: jpg, png, or webp (can add extension to --output)")
	quality := fs.Int("quality", -1, "JPEG/WebP quality from 1 to 100 (default: 90; PNG is lossless)")
	list := fs.Bool("list", false, "list available image filters")
	fs.Usage = func() { printImageFilterHelp(stdout) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if *list {
		if *filterName != "" || *input != "" || *output != "" || *resize != "" || *format != "" || *quality != -1 {
			return errors.New("--list cannot be combined with filter application options")
		}
		printImageFilterList(stdout, filters)
		return nil
	}
	allFilters := *filterName == "all"
	selected, ok := findImageFilter(filters, *filterName)
	if !ok && !allFilters {
		return fmt.Errorf("unknown image filter %q; available filters: %s", *filterName, strings.Join(imageFilterNames(filters), ", "))
	}
	if *input == "" || *output == "" {
		return errors.New("--file and --output are required")
	}
	resizeSize, resizing := imageResizeSizes[*resize]
	if *resize != "" && !resizing {
		return fmt.Errorf("unknown resize size %q; available sizes: %s", *resize, strings.Join(imageResizeSizeNames(), ", "))
	}
	if *quality != -1 && (*quality < 1 || *quality > 100) {
		return errors.New("--quality must be between 1 and 100")
	}
	inputExt := strings.ToLower(filepath.Ext(*input))
	if !supportedImageExtension(inputExt) {
		return fmt.Errorf("unsupported input image format %q; supported formats: .jpg, .jpeg, .png, .webp", inputExt)
	}
	outputName := *output
	outputExt := strings.ToLower(filepath.Ext(outputName))
	if *format != "" {
		formatExt, ok := imageFormatExtension(*format)
		if !ok {
			return errors.New("--format must be jpg, png, or webp")
		}
		if outputExt != "" && !sameImageFormat(outputExt, formatExt) {
			return fmt.Errorf("output filename extension %q conflicts with --format %q", outputExt, *format)
		}
		if outputExt == "" {
			outputName += formatExt
		}
		outputExt = formatExt
	}
	if !supportedImageExtension(outputExt) {
		return fmt.Errorf("unsupported output image format %q; supported formats: .jpg, .jpeg, .png, .webp", outputExt)
	}
	if outputExt == ".png" && *quality != -1 {
		return errors.New("--quality applies to JPEG and WebP; PNG is lossless")
	}
	if outputName == "." || outputName == ".." || strings.ContainsAny(outputName, `/\\`) || filepath.Base(outputName) != outputName {
		return errors.New("--output must be a filename only; files are written to ~/evueditout")
	}
	inPath, err := filepath.Abs(*input)
	if err != nil {
		return fmt.Errorf("resolve input path: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory for output: %w", err)
	}
	outputDir := filepath.Join(homeDir, "evueditout")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory %q: %w", outputDir, err)
	}
	outputNames := []string{outputName}
	selectedFilters := []imageFilter{selected}
	if allFilters {
		selectedFilters = filters
		outputNames = make([]string, 0, len(filters))
		ext := filepath.Ext(outputName)
		base := strings.TrimSuffix(outputName, ext)
		for _, item := range filters {
			outputNames = append(outputNames, base+"-"+item.name+ext)
		}
	}
	outPaths := make([]string, len(outputNames))
	inPath = filepath.Clean(inPath)
	for i, name := range outputNames {
		outPaths[i] = filepath.Join(outputDir, name)
		if inPath == outPaths[i] {
			return errors.New("input and output must be different paths")
		}
	}
	info, err := os.Stat(inPath)
	if err != nil {
		return fmt.Errorf("read input image %q: %w", *input, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("input image %q is not a regular file", *input)
	}
	for _, name := range outputNames {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err == nil {
			return fmt.Errorf("output %q already exists; choose another filename", name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("check output %q: %w", name, err)
		}
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return errors.New("ffprobe was not found on PATH; install FFmpeg and try again")
	}
	if _, err := probeVideoDimensions(ffprobe, inPath); err != nil {
		return fmt.Errorf("inspect input image: %w", err)
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return errors.New("FFmpeg was not found on PATH; install FFmpeg and try again")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	for i, item := range selectedFilters {
		graph := item.graph()
		if resizing {
			graph = fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,%s", resizeSize.width, resizeSize.height, resizeSize.width, resizeSize.height, graph)
		}
		argsFFmpeg := []string{"-nostdin", "-hide_banner", "-n", "-i", inPath, "-vf", graph, "-frames:v", "1"}
		switch outputExt {
		case ".jpg", ".jpeg":
			qualityValue := *quality
			if qualityValue == -1 {
				qualityValue = 90
			}
			q := 31 - int(float64(qualityValue-1)*29/99+0.5)
			argsFFmpeg = append(argsFFmpeg, "-c:v", "mjpeg", "-q:v", fmt.Sprint(q))
		case ".png":
			argsFFmpeg = append(argsFFmpeg, "-c:v", "png", "-compression_level", "6")
		case ".webp":
			q := 90
			if *quality != -1 {
				q = *quality
			}
			argsFFmpeg = append(argsFFmpeg, "-c:v", "libwebp", "-quality", fmt.Sprint(q), "-compression_level", "6")
		}
		argsFFmpeg = append(argsFFmpeg, "-update", "1", outPaths[i])
		cmd := exec.CommandContext(ctx, ffmpeg, argsFFmpeg...)
		cmd.Stdout, cmd.Stderr = stdout, stderr
		fmt.Fprintf(stderr, "Applying %q image filter...\n", item.name)
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return errors.New("image processing cancelled")
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				return fmt.Errorf("FFmpeg failed for filter %q with exit code %d", item.name, exitErr.ExitCode())
			}
			return fmt.Errorf("run FFmpeg for filter %q: %w", item.name, err)
		}
		fmt.Fprintf(stdout, "Wrote %s\n", outPaths[i])
	}
	return nil
}

func findImageFilter(filters []imageFilter, name string) (imageFilter, bool) {
	for _, item := range filters {
		if item.name == name {
			return item, true
		}
	}
	return imageFilter{}, false
}

func imageFilterNames(filters []imageFilter) []string {
	names := make([]string, 0, len(filters)+1)
	names = append(names, "all")
	for _, item := range filters {
		names = append(names, item.name)
	}
	return names
}

func supportedImageExtension(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	default:
		return false
	}
}

func imageFormatExtension(format string) (string, bool) {
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		return ".jpg", true
	case "png":
		return ".png", true
	case "webp":
		return ".webp", true
	default:
		return "", false
	}
}

func sameImageFormat(ext, canonical string) bool {
	if ext == ".jpeg" {
		ext = ".jpg"
	}
	return ext == canonical
}

func imageResizeSizeNames() []string {
	return []string{"instagram", "landscape", "portrait", "square", "story", "youtube"}
}

func printImageFilterList(w io.Writer, filters []imageFilter) {
	fmt.Fprintln(w, "Available image filters:")
	fmt.Fprintln(w, "  all            Generate one output for each available filter.")
	for _, item := range filters {
		fmt.Fprintf(w, "  %-14s %s\n", item.name, item.description)
		if p := item.properties; p != nil {
			fmt.Fprintf(w, "                 exposureEV=%+.2f contrast=%.2f saturation=%.2f warmth=%+.2f texture=%+.2f\n", p.ExposureEV, p.Contrast, p.Saturation, p.Warmth, p.Texture)
		}
	}
}

func printImageFilterHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: evuedit --module evufilters --filter <name|all> --file <inputImage> --output <outputImage>")
	fmt.Fprintln(w, "Apply a named filter to a still image. Output is written to ~/evueditout.")
	fmt.Fprintln(w, "Supported formats: JPEG, PNG, and WebP. Without --resize, output preserves the image dimensions.")
	fmt.Fprintln(w, "Resize presets: square (1080x1080), portrait (1080x1350), story/instagram (1080x1920), landscape (1920x1080), youtube (1280x720); images are proportionally scaled and center-cropped.")
	fmt.Fprintln(w, "--format selects jpg, png, or webp. --quality sets JPEG/WebP quality from 1 to 100; PNG is lossless.")
	fmt.Fprintln(w, "Use --list to list the available filters.")
	fmt.Fprintln(w, "With --filter all, --output is a filename base; outputs are named <base>-<filter>.<ext>.")
	fmt.Fprintln(w)
	filters, err := loadEmbeddedImageFilters()
	if err != nil {
		fmt.Fprintf(w, "Unable to load image filter catalog: %v\n", err)
		return
	}
	printImageFilterList(w, filters)
}
