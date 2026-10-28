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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed presets.json use_cases.json
var presetConfig embed.FS

type presetTool struct{}

func (presetTool) Name() string        { return "preset" }
func (presetTool) Description() string { return "apply a built-in color preset to a video" }

type presetCatalog struct {
	Version       int           `json:"version"`
	InputProfile  string        `json:"inputProfile"`
	WorkingOutput string        `json:"workingOutput"`
	Presets       []colorPreset `json:"presets"`
}

type colorPreset struct {
	ID           string   `json:"id"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	ExposureEV   float64  `json:"exposureEV"`
	Contrast     float64  `json:"contrast"`
	Saturation   float64  `json:"saturation"`
	Highlights   float64  `json:"highlights"`
	Shadows      float64  `json:"shadows"`
	Warmth       float64  `json:"warmth"`
	LUTIntensity float64  `json:"lutIntensity"`
}

type useCaseCatalog struct {
	Version  int             `json:"version"`
	Tags     []presetTag     `json:"tags"`
	UseCases []presetUseCase `json:"useCases"`
}

type presetTag struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type presetUseCase struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

func loadPresetCatalog() (presetCatalog, error) {
	data, err := presetConfig.ReadFile("presets.json")
	if err != nil {
		return presetCatalog{}, fmt.Errorf("read embedded preset catalog: %w", err)
	}
	var catalog presetCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return presetCatalog{}, fmt.Errorf("parse preset catalog: %w", err)
	}
	if catalog.Version != 2 {
		return presetCatalog{}, fmt.Errorf("unsupported preset catalog version %d", catalog.Version)
	}
	if catalog.InputProfile != "samsung-log" && catalog.InputProfile != "rec709" {
		return presetCatalog{}, fmt.Errorf("unsupported input profile %q", catalog.InputProfile)
	}
	if catalog.WorkingOutput != "rec709-sdr" {
		return presetCatalog{}, fmt.Errorf("unsupported working output %q", catalog.WorkingOutput)
	}
	if len(catalog.Presets) == 0 {
		return presetCatalog{}, errors.New("preset catalog must define at least one preset")
	}
	seen := make(map[string]bool, len(catalog.Presets))
	for _, item := range catalog.Presets {
		if strings.TrimSpace(item.ID) == "" || seen[item.ID] {
			return presetCatalog{}, fmt.Errorf("preset catalog contains an empty or duplicate id %q", item.ID)
		}
		if strings.TrimSpace(item.Description) == "" {
			return presetCatalog{}, fmt.Errorf("preset %q: description is required", item.ID)
		}
		if err := validateTagList("preset "+item.ID, item.Tags); err != nil {
			return presetCatalog{}, err
		}
		seen[item.ID] = true
		if err := validatePreset(item); err != nil {
			return presetCatalog{}, fmt.Errorf("preset %q: %w", item.ID, err)
		}
	}
	return catalog, nil
}

func loadUseCaseCatalog() (useCaseCatalog, error) {
	data, err := presetConfig.ReadFile("use_cases.json")
	if err != nil {
		return useCaseCatalog{}, fmt.Errorf("read embedded use-case catalog: %w", err)
	}
	var catalog useCaseCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return useCaseCatalog{}, fmt.Errorf("parse use-case catalog: %w", err)
	}
	if catalog.Version != 1 {
		return useCaseCatalog{}, fmt.Errorf("unsupported use-case catalog version %d", catalog.Version)
	}
	tagIDs := make(map[string]bool, len(catalog.Tags))
	for _, tag := range catalog.Tags {
		if strings.TrimSpace(tag.ID) == "" || tagIDs[tag.ID] {
			return useCaseCatalog{}, fmt.Errorf("use-case catalog contains an empty or duplicate tag id %q", tag.ID)
		}
		if strings.TrimSpace(tag.Description) == "" {
			return useCaseCatalog{}, fmt.Errorf("tag %q: description is required", tag.ID)
		}
		tagIDs[tag.ID] = true
	}
	caseIDs := make(map[string]bool, len(catalog.UseCases))
	for _, item := range catalog.UseCases {
		if strings.TrimSpace(item.ID) == "" || caseIDs[item.ID] {
			return useCaseCatalog{}, fmt.Errorf("use-case catalog contains an empty or duplicate use-case id %q", item.ID)
		}
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Description) == "" {
			return useCaseCatalog{}, fmt.Errorf("use case %q: name and description are required", item.ID)
		}
		if err := validateTagList("use case "+item.ID, item.Tags); err != nil {
			return useCaseCatalog{}, err
		}
		for _, tag := range item.Tags {
			if !tagIDs[tag] {
				return useCaseCatalog{}, fmt.Errorf("use case %q: unknown tag %q", item.ID, tag)
			}
		}
		caseIDs[item.ID] = true
	}
	if len(catalog.Tags) == 0 || len(catalog.UseCases) == 0 {
		return useCaseCatalog{}, errors.New("use-case catalog must define at least one tag and one use case")
	}
	return catalog, nil
}

func validateTagList(owner string, tags []string) error {
	if len(tags) == 0 {
		return fmt.Errorf("%s: at least one tag is required", owner)
	}
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if strings.TrimSpace(tag) == "" || seen[tag] {
			return fmt.Errorf("%s: tags must be non-empty and unique (invalid tag %q)", owner, tag)
		}
		seen[tag] = true
	}
	return nil
}

func validatePresetTagReferences(presets presetCatalog, useCases useCaseCatalog) error {
	tagIDs := make(map[string]bool, len(useCases.Tags))
	for _, tag := range useCases.Tags {
		tagIDs[tag.ID] = true
	}
	for _, item := range presets.Presets {
		for _, tag := range item.Tags {
			if !tagIDs[tag] {
				return fmt.Errorf("preset %q: unknown tag %q", item.ID, tag)
			}
		}
	}
	for _, item := range useCases.UseCases {
		if len(presetsForUseCase(presets, item)) == 0 {
			return fmt.Errorf("use case %q: no preset matches all tags %q", item.ID, strings.Join(item.Tags, ", "))
		}
	}
	return nil
}

func validatePreset(item colorPreset) error {
	if item.ExposureEV < -5 || item.ExposureEV > 5 {
		return errors.New("exposureEV must be between -5 and 5")
	}
	if item.Contrast <= 0 || item.Contrast > 3 {
		return errors.New("contrast must be greater than 0 and at most 3")
	}
	if item.Saturation < 0 || item.Saturation > 3 {
		return errors.New("saturation must be between 0 and 3")
	}
	if item.Highlights < -1 || item.Highlights > 1 || item.Shadows < -1 || item.Shadows > 1 {
		return errors.New("highlights and shadows must be between -1 and 1")
	}
	if item.Warmth < -1 || item.Warmth > 1 {
		return errors.New("warmth must be between -1 and 1")
	}
	if item.LUTIntensity < 0 || item.LUTIntensity > 1 {
		return errors.New("lutIntensity must be between 0 and 1")
	}
	if item.LUTIntensity != 0 {
		return errors.New("creative LUT blending is not available yet; lutIntensity must be 0")
	}
	return nil
}

func (presetTool) Run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("preset", flag.ContinueOnError)
	fs.SetOutput(stderr)
	apply := fs.String("apply", "", "preset id to apply")
	input := fs.String("file", "", "input video file")
	output := fs.String("output", "", "output filename (written to ~/evueditout)")
	inputProfileOverride := fs.String("input-profile", "", "override the catalog input profile (samsung-log or rec709)")
	profileLUT := fs.String("profile-lut", "", "Samsung Log to Rec.709 3D LUT file (required for samsung-log input)")
	list := fs.Bool("list", false, "list available presets")
	listUseCases := fs.Bool("list-use-cases", false, "list available use cases and their tags")
	useCaseID := fs.String("use-case", "", "show presets matching a use-case id")
	fs.Usage = func() { printPresetHelp(stdout) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	catalog, err := loadPresetCatalog()
	if err != nil {
		return err
	}
	useCases, err := loadUseCaseCatalog()
	if err != nil {
		return err
	}
	if err := validatePresetTagReferences(catalog, useCases); err != nil {
		return err
	}
	if (*list && (*listUseCases || *useCaseID != "")) || (*listUseCases && *useCaseID != "") {
		return errors.New("use only one of --list, --list-use-cases, or --use-case")
	}
	if (*list || *listUseCases || *useCaseID != "") && (*apply != "" || *input != "" || *output != "" || *inputProfileOverride != "" || *profileLUT != "") {
		return errors.New("preset discovery options cannot be combined with preset application options")
	}
	if *list {
		printPresetList(stdout, catalog)
		return nil
	}
	if *listUseCases {
		printUseCaseList(stdout, useCases)
		return nil
	}
	if *useCaseID != "" {
		item, ok := findUseCase(useCases, *useCaseID)
		if !ok {
			return fmt.Errorf("unknown use case %q; available use cases: %s", *useCaseID, strings.Join(useCaseNames(useCases), ", "))
		}
		printUseCasePresets(stdout, catalog, item)
		return nil
	}
	selected, ok := findPreset(catalog, *apply)
	if !ok {
		return fmt.Errorf("unknown preset %q; available presets: %s", *apply, strings.Join(presetNames(catalog), ", "))
	}
	if *input == "" || *output == "" {
		return errors.New("--file and --output are required")
	}
	inputProfile := catalog.InputProfile
	if *inputProfileOverride != "" {
		inputProfile = *inputProfileOverride
	}
	if inputProfile != "samsung-log" && inputProfile != "rec709" {
		return fmt.Errorf("unsupported input profile %q; available: samsung-log, rec709", inputProfile)
	}
	if inputProfile == "samsung-log" && *profileLUT == "" {
		return errors.New("the preset catalog expects samsung-log input; provide Samsung's Log to Rec.709 LUT with --profile-lut")
	}

	inPath, err := filepath.Abs(*input)
	if err != nil {
		return fmt.Errorf("resolve input path: %w", err)
	}
	if *output == "." || *output == ".." || strings.ContainsAny(*output, `/\\`) || filepath.Base(*output) != *output {
		return errors.New("--output must be a filename only; files are written to ~/evueditout")
	}
	if strings.ToLower(filepath.Ext(*output)) != ".mp4" {
		return errors.New("--output must use the .mp4 extension for YouTube Shorts and Instagram Reels")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory for output: %w", err)
	}
	outputDir := filepath.Join(homeDir, "evueditout")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory %q: %w", outputDir, err)
	}
	outPath := filepath.Join(outputDir, *output)
	inPath = filepath.Clean(inPath)
	if inPath == outPath {
		return errors.New("input and output must be different paths")
	}
	info, err := os.Stat(inPath)
	if err != nil {
		return fmt.Errorf("read input %q: %w", *input, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("input %q is not a regular file", *input)
	}
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("output %q already exists; choose another path", *output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check output %q: %w", *output, err)
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return errors.New("ffprobe was not found on PATH; install FFmpeg (which includes ffprobe) and try again")
	}
	dimensions, err := probeVideoDimensions(ffprobe, inPath)
	if err != nil {
		return fmt.Errorf("inspect input video: %w", err)
	}
	if dimensions.width >= dimensions.height {
		return fmt.Errorf("invalid format: input video is horizontal (%dx%d); use a vertical phone recording for Shorts", dimensions.width, dimensions.height)
	}
	var lutPath string
	if *profileLUT != "" {
		lutPath, err = filepath.Abs(*profileLUT)
		if err != nil {
			return fmt.Errorf("resolve profile LUT path: %w", err)
		}
		lutInfo, err := os.Stat(lutPath)
		if err != nil {
			return fmt.Errorf("read profile LUT %q: %w", *profileLUT, err)
		}
		if !lutInfo.Mode().IsRegular() {
			return fmt.Errorf("profile LUT %q is not a regular file", *profileLUT)
		}
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return errors.New("FFmpeg was not found on PATH; install FFmpeg and try again")
	}
	filtergraph, err := buildPresetFilter(selected, inputProfile, catalog.WorkingOutput, lutPath)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg,
		"-nostdin", "-hide_banner", "-n", "-stats_period", "1", "-i", inPath,
		"-vf", filtergraph,
		"-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart",
		"-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709",
		outPath,
	)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	fmt.Fprintf(stderr, "Applying %q preset to %s input (%s)...\n", selected.ID, inputProfile, selected.Description)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("video processing cancelled")
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("FFmpeg failed with exit code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("run FFmpeg: %w", err)
	}
	fmt.Fprintf(stdout, "Wrote %s\n", outPath)
	return nil
}

type videoDimensions struct{ width, height int }

func probeVideoDimensions(ffprobe, path string) (videoDimensions, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "csv=s=x:p=0", path).Output()
	if err != nil {
		return videoDimensions{}, fmt.Errorf("ffprobe could not read video dimensions: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(output)), "x")
	if len(parts) != 2 {
		return videoDimensions{}, errors.New("ffprobe returned invalid video dimensions")
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil || width <= 0 {
		return videoDimensions{}, errors.New("ffprobe returned invalid video width")
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil || height <= 0 {
		return videoDimensions{}, errors.New("ffprobe returned invalid video height")
	}
	return videoDimensions{width: width, height: height}, nil
}

func buildPresetFilter(item colorPreset, inputProfile, workingOutput, lutPath string) (string, error) {
	if err := validatePreset(item); err != nil {
		return "", err
	}
	if workingOutput != "rec709-sdr" {
		return "", fmt.Errorf("unsupported working output %q", workingOutput)
	}
	filters := make([]string, 0, 5)
	switch inputProfile {
	case "samsung-log":
		if lutPath == "" {
			return "", errors.New("Samsung Log input requires a Samsung Log to Rec.709 LUT")
		}
		filters = append(filters, "lut3d=file='"+escapeFilterPath(lutPath)+"'")
	case "rec709":
		if lutPath != "" {
			return "", errors.New("--profile-lut is only valid when inputProfile is samsung-log")
		}
	default:
		return "", fmt.Errorf("unsupported input profile %q", inputProfile)
	}
	if item.ExposureEV != 0 {
		filters = append(filters, fmt.Sprintf("exposure=exposure=%s", filterNumber(item.ExposureEV)))
	}
	filters = append(filters, fmt.Sprintf("eq=contrast=%s:saturation=%s", filterNumber(item.Contrast), filterNumber(item.Saturation)))
	shadowPoint := 0.25 + item.Shadows*0.25
	highlightPoint := 0.75 + item.Highlights*0.25
	filters = append(filters, fmt.Sprintf("curves=master='0/0 0.25/%s 0.75/%s 1/1'", filterNumber(shadowPoint), filterNumber(highlightPoint)))
	if item.Warmth != 0 {
		warmth := item.Warmth * 0.15
		filters = append(filters, fmt.Sprintf("colorbalance=rs=%s:bs=%s:rm=%s:bm=%s:rh=%s:bh=%s",
			filterNumber(warmth), filterNumber(-warmth), filterNumber(warmth), filterNumber(-warmth), filterNumber(warmth), filterNumber(-warmth)))
	}
	return strings.Join(filters, ","), nil
}

func escapeFilterPath(path string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "'", "\\'", ":", "\\:", ",", "\\,")
	return replacer.Replace(filepath.ToSlash(path))
}

func filterNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func findPreset(catalog presetCatalog, id string) (colorPreset, bool) {
	for _, item := range catalog.Presets {
		if item.ID == id {
			return item, true
		}
	}
	return colorPreset{}, false
}

func presetNames(catalog presetCatalog) []string {
	names := make([]string, 0, len(catalog.Presets))
	for _, item := range catalog.Presets {
		names = append(names, item.ID)
	}
	sort.Strings(names)
	return names
}

func useCaseNames(catalog useCaseCatalog) []string {
	names := make([]string, 0, len(catalog.UseCases))
	for _, item := range catalog.UseCases {
		names = append(names, item.ID)
	}
	sort.Strings(names)
	return names
}

func findUseCase(catalog useCaseCatalog, id string) (presetUseCase, bool) {
	for _, item := range catalog.UseCases {
		if item.ID == id {
			return item, true
		}
	}
	return presetUseCase{}, false
}

func presetsForUseCase(catalog presetCatalog, item presetUseCase) []colorPreset {
	wanted := make(map[string]bool, len(item.Tags))
	for _, tag := range item.Tags {
		wanted[tag] = true
	}
	matched := make([]colorPreset, 0)
	for _, preset := range catalog.Presets {
		matchedTags := make(map[string]bool, len(preset.Tags))
		for _, presetTag := range preset.Tags {
			matchedTags[presetTag] = true
		}
		matchesAll := true
		for tag := range wanted {
			if !matchedTags[tag] {
				matchesAll = false
				break
			}
		}
		if matchesAll {
			matched = append(matched, preset)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	return matched
}

func printUseCaseList(w io.Writer, catalog useCaseCatalog) {
	fmt.Fprintln(w, "Use cases:")
	for _, id := range useCaseNames(catalog) {
		item, _ := findUseCase(catalog, id)
		fmt.Fprintf(w, "  %-20s %s (tags: %s)\n", item.ID, item.Name, strings.Join(item.Tags, ", "))
	}
}

func printUseCasePresets(w io.Writer, presets presetCatalog, item presetUseCase) {
	fmt.Fprintf(w, "Use case: %s\n%s\nTags: %s\n\nMatching presets:\n", item.Name, item.Description, strings.Join(item.Tags, ", "))
	for _, preset := range presetsForUseCase(presets, item) {
		fmt.Fprintf(w, "  %-18s %s\n", preset.ID, preset.Description)
	}
}

func printPresetList(w io.Writer, catalog presetCatalog) {
	fmt.Fprintf(w, "Input profile: %s\nWorking output: %s\n\nPresets:\n", catalog.InputProfile, catalog.WorkingOutput)
	for _, id := range presetNames(catalog) {
		item, _ := findPreset(catalog, id)
		fmt.Fprintf(w, "  %-18s %s (tags: %s)\n", item.ID, item.Description, strings.Join(item.Tags, ", "))
	}
}

func printPresetHelp(w io.Writer) {
	catalog, err := loadPresetCatalog()
	useCases, useCaseErr := loadUseCaseCatalog()
	fmt.Fprintln(w, "Usage: evuedit --module presets --apply <presetName> --file <inputFile> --output <outputFile.mp4> [--input-profile <profile>] [--profile-lut <file>]")
	fmt.Fprintln(w, "       evuedit --tool preset --apply <presetName> --file <inputFile> --output <outputFile.mp4> [--input-profile <profile>] [--profile-lut <file>]")
	fmt.Fprintln(w, "Apply a color preset and encode vertical Shorts/Reels video (H.264/AAC MP4).")
	fmt.Fprintln(w, "Input must be portrait (vertical); landscape and square videos are rejected.")
	fmt.Fprintln(w, "Use --list to see preset names and descriptions.")
	fmt.Fprintln(w, "Use --list-use-cases or --use-case <id> to find presets by use case.")
	if err != nil {
		fmt.Fprintf(w, "\nCould not load preset catalog: %v\n", err)
		return
	}
	fmt.Fprintf(w, "\nInput profile: %s\nWorking output: %s\n\nBuilt-in presets and parameters:\n", catalog.InputProfile, catalog.WorkingOutput)
	for _, id := range presetNames(catalog) {
		item, _ := findPreset(catalog, id)
		fmt.Fprintf(w, "\n  %s — %s\n", item.ID, item.Description)
		fmt.Fprintf(w, "    tags: %s\n", strings.Join(item.Tags, ", "))
		fmt.Fprintf(w, "    exposureEV=%s contrast=%s saturation=%s highlights=%s shadows=%s warmth=%s lutIntensity=%s\n",
			filterNumber(item.ExposureEV), filterNumber(item.Contrast), filterNumber(item.Saturation),
			filterNumber(item.Highlights), filterNumber(item.Shadows), filterNumber(item.Warmth), filterNumber(item.LUTIntensity))
	}
	if useCaseErr != nil {
		fmt.Fprintf(w, "\nCould not load use-case catalog: %v\n", useCaseErr)
	} else {
		fmt.Fprintf(w, "\nUse cases: %s\n", strings.Join(useCaseNames(useCases), ", "))
	}
	fmt.Fprintln(w, "\nPresets are starting points; results vary with the source footage.")
}
