package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed thumb_templates.json templates/*.json
var thumbTemplateConfig embed.FS

type thumbTool struct{}

func (thumbTool) Name() string { return "evuthumb" }
func (thumbTool) Description() string {
	return "create YouTube and Instagram thumbnails from JSON templates"
}

type thumbCanvas struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type thumbLayout struct {
	ImageMode        string  `json:"imageMode"`
	SubjectPosition  string  `json:"subjectPosition"`
	Gradient         string  `json:"gradient"`
	HeadlinePosition string  `json:"headlinePosition"`
	HeadlineMaxLines int     `json:"headlineMaxLines"`
	HeadlineMaxWords int     `json:"headlineMaxWords"`
	Font             string  `json:"font"`
	FontSizeRatio    float64 `json:"fontSizeRatio"`
	TextColor        string  `json:"textColor"`
	AccentColor      string  `json:"accentColor"`
	Shadow           bool    `json:"shadow"`
	LabelText        string  `json:"labelText"`
	LabelPosition    string  `json:"labelPosition"`
	LabelBackground  string  `json:"labelBackground"`
	StickerStyle     string  `json:"stickerStyle"`
	Border           bool    `json:"border"`
	BorderColor      string  `json:"borderColor"`
	BorderWidthRatio float64 `json:"borderWidthRatio"`
	PanelColor       string  `json:"panelColor"`
	PanelOpacity     float64 `json:"panelOpacity"`
	PriceBadge       struct {
		Enabled    bool   `json:"enabled"`
		Position   string `json:"position"`
		Background string `json:"background"`
		TextColor  string `json:"textColor"`
		Prefix     string `json:"prefix"`
	} `json:"priceBadge"`
	LetterSpacingRatio float64 `json:"letterSpacingRatio"`
	SafeArea           struct {
		TopRatio    float64 `json:"topRatio"`
		BottomRatio float64 `json:"bottomRatio"`
		LeftRatio   float64 `json:"leftRatio"`
		RightRatio  float64 `json:"rightRatio"`
	} `json:"safeArea"`
	OutlineText           bool    `json:"outlineText"`
	OutlineColor          string  `json:"outlineColor"`
	OutlineWidthRatio     float64 `json:"outlineWidthRatio"`
	SecondaryTextPosition string  `json:"secondaryTextPosition"`
	SecondaryTextStyle    string  `json:"secondaryTextStyle"`
	SplitDirection        string  `json:"splitDirection"`
	SplitRatio            float64 `json:"splitRatio"`
	LeftLabel             string  `json:"leftLabel"`
	RightLabel            string  `json:"rightLabel"`
	LeftLabelColor        string  `json:"leftLabelColor"`
	RightLabelColor       string  `json:"rightLabelColor"`
	Divider               bool    `json:"divider"`
	DividerColor          string  `json:"dividerColor"`
	DividerWidthRatio     float64 `json:"dividerWidthRatio"`
}

type thumbExport struct {
	Format     string `json:"format"`
	Quality    int    `json:"quality"`
	ColorSpace string `json:"colorSpace"`
}

type thumbTemplate struct {
	ID          string                 `json:"id"`
	Version     int                    `json:"version"`
	Description string                 `json:"description"`
	Canvas      map[string]thumbCanvas `json:"canvas"`
	Layout      thumbLayout            `json:"layout"`
	Export      thumbExport            `json:"export"`
}

type thumbTemplateCatalog struct {
	Version   int             `json:"version"`
	Templates []thumbTemplate `json:"templates"`
}

type fittedThumbHeadline struct {
	lines      []string
	pointSize  int
	textWidth  int
	textHeight int
}

var thumbHexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func loadThumbTemplateCatalog(data []byte) (thumbTemplateCatalog, error) {
	var catalog thumbTemplateCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return thumbTemplateCatalog{}, fmt.Errorf("parse thumbnail template catalog: %w", err)
	}
	if catalog.Version != 1 {
		return thumbTemplateCatalog{}, fmt.Errorf("unsupported thumbnail template catalog version %d", catalog.Version)
	}
	if len(catalog.Templates) == 0 {
		return thumbTemplateCatalog{}, errors.New("thumbnail template catalog must define at least one template")
	}
	seen := make(map[string]bool, len(catalog.Templates))
	for _, item := range catalog.Templates {
		if strings.TrimSpace(item.ID) == "" || seen[item.ID] {
			return thumbTemplateCatalog{}, fmt.Errorf("thumbnail template catalog contains an empty or duplicate id %q", item.ID)
		}
		seen[item.ID] = true
		if err := validateThumbTemplate(item); err != nil {
			return thumbTemplateCatalog{}, err
		}
	}
	return catalog, nil
}

func validateThumbTemplate(item thumbTemplate) error {
	if item.Version != 1 {
		return fmt.Errorf("template %q: unsupported version %d", item.ID, item.Version)
	}
	if strings.TrimSpace(item.Description) == "" {
		return fmt.Errorf("template %q: description is required", item.ID)
	}
	for _, platform := range []string{"youtube", "instagram"} {
		canvas, ok := item.Canvas[platform]
		if !ok || canvas.Width < 1 || canvas.Height < 1 || canvas.Width > 10000 || canvas.Height > 10000 {
			return fmt.Errorf("template %q: canvas for %s must have width and height between 1 and 10000", item.ID, platform)
		}
	}
	layout := item.Layout
	if layout.ImageMode != "cover" && layout.ImageMode != "split-cover" {
		return fmt.Errorf("template %q: imageMode must be cover or split-cover", item.ID)
	}
	if layout.ImageMode == "split-cover" {
		if layout.SplitDirection != "vertical" || layout.SplitRatio < 0.2 || layout.SplitRatio > 0.8 {
			return fmt.Errorf("template %q: split-cover requires vertical splitDirection and splitRatio between 0.2 and 0.8", item.ID)
		}
	} else if layout.SubjectPosition != "left" && layout.SubjectPosition != "center" && layout.SubjectPosition != "right" {
		return fmt.Errorf("template %q: subjectPosition must be left, center, or right", item.ID)
	}
	if !oneOf(layout.Gradient, "none", "left-to-right", "left-to-right-soft", "bottom-up", "bottom-up-soft") {
		return fmt.Errorf("template %q: unsupported gradient %q", item.ID, layout.Gradient)
	}
	if !oneOf(layout.HeadlinePosition, "left-center", "center", "right-center", "bottom-left", "top-center") {
		return fmt.Errorf("template %q: unsupported headlinePosition %q", item.ID, layout.HeadlinePosition)
	}
	if strings.TrimSpace(layout.Font) == "" || layout.HeadlineMaxLines < 1 || layout.HeadlineMaxLines > 10 || layout.HeadlineMaxWords < 1 || layout.HeadlineMaxWords > 30 || layout.FontSizeRatio < 0.01 || layout.FontSizeRatio > 0.5 {
		return fmt.Errorf("template %q: font, headline limits, and fontSizeRatio must be valid", item.ID)
	}
	for field, value := range map[string]string{"textColor": layout.TextColor, "accentColor": layout.AccentColor} {
		if !thumbHexColor.MatchString(value) {
			return fmt.Errorf("template %q: %s must be a #RRGGBB color", item.ID, field)
		}
	}
	for field, value := range map[string]string{"labelBackground": layout.LabelBackground, "borderColor": layout.BorderColor, "outlineColor": layout.OutlineColor, "panelColor": layout.PanelColor, "priceBadge.background": layout.PriceBadge.Background, "priceBadge.textColor": layout.PriceBadge.TextColor, "leftLabelColor": layout.LeftLabelColor, "rightLabelColor": layout.RightLabelColor, "dividerColor": layout.DividerColor} {
		if value != "" && !thumbHexColor.MatchString(value) {
			return fmt.Errorf("template %q: %s must be a #RRGGBB color", item.ID, field)
		}
	}
	if layout.LabelPosition != "" && !oneOf(layout.LabelPosition, "top-left", "center-top") {
		return fmt.Errorf("template %q: unsupported labelPosition %q", item.ID, layout.LabelPosition)
	}
	if layout.SecondaryTextPosition != "" && !oneOf(layout.SecondaryTextPosition, "bottom-left", "bottom-right", "bottom-center") {
		return fmt.Errorf("template %q: unsupported secondaryTextPosition %q", item.ID, layout.SecondaryTextPosition)
	}
	if layout.PanelOpacity < 0 || layout.PanelOpacity > 1 || layout.BorderWidthRatio < 0 || layout.BorderWidthRatio > 0.05 || layout.OutlineWidthRatio < 0 || layout.OutlineWidthRatio > 0.05 || layout.DividerWidthRatio < 0 || layout.DividerWidthRatio > 0.05 || layout.LetterSpacingRatio < 0 || layout.LetterSpacingRatio > 0.05 {
		return fmt.Errorf("template %q: opacity and width ratios are outside supported ranges", item.ID)
	}
	if layout.SafeArea.TopRatio < 0 || layout.SafeArea.TopRatio > 0.45 || layout.SafeArea.BottomRatio < 0 || layout.SafeArea.BottomRatio > 0.45 || layout.SafeArea.LeftRatio < 0 || layout.SafeArea.LeftRatio > 0.45 || layout.SafeArea.RightRatio < 0 || layout.SafeArea.RightRatio > 0.45 {
		return fmt.Errorf("template %q: safeArea ratios must be between 0 and 0.45", item.ID)
	}
	if layout.PriceBadge.Enabled && (layout.PriceBadge.Position != "bottom-left" || layout.PriceBadge.Background == "" || layout.PriceBadge.TextColor == "" || layout.PriceBadge.Prefix == "") {
		return fmt.Errorf("template %q: enabled priceBadge requires bottom-left position, colors, and prefix", item.ID)
	}
	if item.Export.Format != "jpeg" || item.Export.Quality < 1 || item.Export.Quality > 100 || item.Export.ColorSpace != "sRGB" {
		return fmt.Errorf("template %q: export must use jpeg, quality 1-100, and sRGB", item.ID)
	}
	return nil
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func loadEmbeddedThumbTemplateCatalog() (thumbTemplateCatalog, error) {
	data, err := thumbTemplateConfig.ReadFile("thumb_templates.json")
	if err != nil {
		return thumbTemplateCatalog{}, fmt.Errorf("read embedded thumbnail template catalog: %w", err)
	}
	catalog, err := loadThumbTemplateCatalog(data)
	if err != nil {
		return thumbTemplateCatalog{}, err
	}
	files, err := thumbTemplateConfig.ReadDir("templates")
	if err != nil {
		return thumbTemplateCatalog{}, fmt.Errorf("read embedded templates directory: %w", err)
	}
	for _, file := range files {
		if file.IsDir() || strings.HasPrefix(file.Name(), "._") || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		data, err := thumbTemplateConfig.ReadFile(filepath.Join("templates", file.Name()))
		if err != nil {
			return thumbTemplateCatalog{}, fmt.Errorf("read embedded template %q: %w", file.Name(), err)
		}
		var item thumbTemplate
		if err := json.Unmarshal(data, &item); err != nil {
			return thumbTemplateCatalog{}, fmt.Errorf("parse embedded template %q: %w", file.Name(), err)
		}
		if item.Layout.Gradient == "" {
			item.Layout.Gradient = "none"
		}
		if err := validateThumbTemplate(item); err != nil {
			return thumbTemplateCatalog{}, err
		}
		replaced := false
		for i := range catalog.Templates {
			if catalog.Templates[i].ID == item.ID {
				catalog.Templates[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			catalog.Templates = append(catalog.Templates, item)
		}
	}
	sort.Slice(catalog.Templates, func(i, j int) bool { return catalog.Templates[i].ID < catalog.Templates[j].ID })
	return catalog, nil
}

func (thumbTool) Run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("evuthumb", flag.ContinueOnError)
	fs.SetOutput(stderr)
	templateID := fs.String("template", "", "thumbnail template id")
	platform := fs.String("platform", "", "target platform: youtube or instagram")
	input := fs.String("file", "", "source still image")
	compareInput := fs.String("compare-file", "", "second source image required by food-comparison")
	headline := fs.String("headline", "", "one headline, for example: \"THE BEST STREET FOOD\"")
	headlinesFile := fs.String("headlines-file", "", "UTF-8 file with one headline per line")
	price := fs.String("price", "", "price text used by the food-deal badge")
	secondaryText := fs.String("secondary-text", "", "subtitle or location tag for templates that support it")
	output := fs.String("output", "", "output image filename (written to ~/evueditout)")
	list := fs.Bool("list", false, "list available thumbnail templates")
	fs.Usage = func() { printThumbHelp(stdout) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	catalog, err := loadEmbeddedThumbTemplateCatalog()
	if err != nil {
		return err
	}
	if *list {
		if *templateID != "" || *platform != "" || *input != "" || *compareInput != "" || *headline != "" || *headlinesFile != "" || *price != "" || *secondaryText != "" || *output != "" {
			return errors.New("--list cannot be combined with thumbnail generation options")
		}
		printThumbTemplateList(stdout, catalog)
		return nil
	}
	if (*headline == "") == (*headlinesFile == "") {
		return errors.New("provide exactly one of --headline or --headlines-file")
	}
	if *templateID == "" || *platform == "" || *input == "" || *output == "" {
		return errors.New("--template, --platform, --file, and --output are required")
	}
	if *platform != "youtube" && *platform != "instagram" {
		return errors.New("--platform must be youtube or instagram")
	}
	template, ok := findThumbTemplate(catalog, *templateID)
	if !ok {
		return fmt.Errorf("unknown thumbnail template %q; available templates: %s", *templateID, strings.Join(thumbTemplateIDs(catalog), ", "))
	}
	if (template.Layout.ImageMode == "split-cover") != (*compareInput != "") {
		if template.Layout.ImageMode == "split-cover" {
			return errors.New("template food-comparison requires --compare-file with the second source image")
		}
		return errors.New("--compare-file is only supported by the food-comparison template")
	}
	if template.Layout.PriceBadge.Enabled != (*price != "") {
		if template.Layout.PriceBadge.Enabled {
			return errors.New("template food-deal requires --price for its price badge")
		}
		return errors.New("--price is only supported by the food-deal template")
	}
	if (template.Layout.SecondaryTextStyle != "") != (*secondaryText != "") {
		if template.Layout.SecondaryTextStyle != "" {
			return fmt.Errorf("template %q requires --secondary-text", template.ID)
		}
		return fmt.Errorf("--secondary-text is not supported by template %q", template.ID)
	}
	inputExt := strings.ToLower(filepath.Ext(*input))
	if !supportedImageExtension(inputExt) {
		return fmt.Errorf("unsupported input image format %q; supported formats: .jpg, .jpeg, .png, .webp", inputExt)
	}
	outputExt := strings.ToLower(filepath.Ext(*output))
	if outputExt != ".jpg" && outputExt != ".jpeg" {
		return errors.New("--output must use a .jpg or .jpeg extension because the selected template exports JPEG")
	}
	if strings.ContainsAny(*output, `/\\`) || filepath.Base(*output) != *output || *output == "." || *output == ".." {
		return errors.New("--output must be a filename only; files are written to ~/evueditout")
	}
	headlines, err := readThumbHeadlines(*headline, *headlinesFile)
	if err != nil {
		return err
	}
	if len(headlines) == 0 {
		return errors.New("headline input contains no non-empty headlines")
	}
	for index, text := range headlines {
		if countHeadlineWords(text) > template.Layout.HeadlineMaxWords {
			return fmt.Errorf("headline %d contains %d words; template %q allows at most %d", index+1, countHeadlineWords(text), template.ID, template.Layout.HeadlineMaxWords)
		}
	}
	inPath, err := filepath.Abs(*input)
	if err != nil {
		return fmt.Errorf("resolve input image path: %w", err)
	}
	info, err := os.Stat(inPath)
	if err != nil {
		return fmt.Errorf("read input image %q: %w", *input, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("input image %q is not a regular file", *input)
	}
	comparePath := ""
	if *compareInput != "" {
		compareExt := strings.ToLower(filepath.Ext(*compareInput))
		if !supportedImageExtension(compareExt) {
			return fmt.Errorf("unsupported comparison image format %q; supported formats: .jpg, .jpeg, .png, .webp", compareExt)
		}
		comparePath, err = filepath.Abs(*compareInput)
		if err != nil {
			return fmt.Errorf("resolve comparison image path: %w", err)
		}
		compareInfo, statErr := os.Stat(comparePath)
		if statErr != nil {
			return fmt.Errorf("read comparison image %q: %w", *compareInput, statErr)
		}
		if !compareInfo.Mode().IsRegular() {
			return fmt.Errorf("comparison image %q is not a regular file", *compareInput)
		}
	}
	outputNames := thumbOutputNames(*output, len(headlines))
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory for output: %w", err)
	}
	outputDir := filepath.Join(home, "evueditout")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory %q: %w", outputDir, err)
	}
	outputPaths := make([]string, len(outputNames))
	for i, name := range outputNames {
		outputPaths[i] = filepath.Join(outputDir, name)
		if filepath.Clean(outputPaths[i]) == filepath.Clean(inPath) {
			return errors.New("input and output must be different paths")
		}
		if comparePath != "" && filepath.Clean(outputPaths[i]) == filepath.Clean(comparePath) {
			return errors.New("comparison input and output must be different paths")
		}
		if _, err := os.Stat(outputPaths[i]); err == nil {
			return fmt.Errorf("output %q already exists; choose another filename", name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("check output %q: %w", name, err)
		}
	}
	imageMagick, err := findImageMagick()
	if err != nil {
		return err
	}
	font, err := resolveThumbFont(template.Layout.Font)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	canvas := template.Canvas[*platform]
	fitted := make([]fittedThumbHeadline, len(headlines))
	for i, text := range headlines {
		lines, pointSize, textWidth, err := fitThumbHeadline(ctx, imageMagick, font, text, template.Layout, canvas)
		if err != nil {
			return fmt.Errorf("headline %d: %w", i+1, err)
		}
		_, textHeight, err := measureThumbText(ctx, imageMagick, font, pointSize, strings.Join(lines, "\n"))
		if err != nil {
			return fmt.Errorf("headline %d measurement: %w", i+1, err)
		}
		fitted[i] = fittedThumbHeadline{lines: lines, pointSize: pointSize, textWidth: textWidth, textHeight: textHeight}
	}
	for i := range headlines {
		fmt.Fprintf(stderr, "Creating %s thumbnail %d/%d...\n", *platform, i+1, len(headlines))
		if err := renderThumb(ctx, imageMagick, inPath, comparePath, outputPaths[i], fitted[i], font, template, canvas, *price, *secondaryText); err != nil {
			return fmt.Errorf("render headline %d: %w", i+1, err)
		}
		fmt.Fprintf(stdout, "Wrote %s\n", outputPaths[i])
	}
	return nil
}

func findImageMagick() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("EVU_IMAGEMAGICK_PATH")); configured != "" {
		path, err := exec.LookPath(configured)
		if err != nil {
			return "", fmt.Errorf("EVU_IMAGEMAGICK_PATH %q was not found: %w", configured, err)
		}
		if err := checkImageMagick(path); err != nil {
			return "", err
		}
		return path, nil
	}
	for _, name := range []string{"magick", "convert"} {
		path, err := exec.LookPath(name)
		if err == nil && checkImageMagick(path) == nil {
			return path, nil
		}
	}
	return "", errors.New("ImageMagick was not found on PATH; install it with scripts/install-imagemagick.sh or set EVU_IMAGEMAGICK_PATH")
}

func checkImageMagick(path string) error {
	output, err := exec.Command(path, "-version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("run ImageMagick %q: %w: %s", path, err, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(string(output), "ImageMagick") {
		return fmt.Errorf("command %q is not ImageMagick", path)
	}
	return nil
}

func renderThumb(ctx context.Context, magick, input, compareInput, output string, fitted fittedThumbHeadline, font string, template thumbTemplate, canvas thumbCanvas, price, secondaryText string) error {
	tempDir, err := os.MkdirTemp("", "evuthumb-*")
	if err != nil {
		return fmt.Errorf("create temporary render directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	basePath := filepath.Join(tempDir, "base.png")
	gradientPath := filepath.Join(tempDir, "gradient.png")
	composedPath := filepath.Join(tempDir, "composed.png")
	if template.Layout.ImageMode == "split-cover" {
		leftPath := filepath.Join(tempDir, "left.png")
		rightPath := filepath.Join(tempDir, "right.png")
		leftWidth := int(math.Round(float64(canvas.Width) * template.Layout.SplitRatio))
		rightWidth := canvas.Width - leftWidth
		for _, side := range []struct {
			input, output string
			width         int
			gravity       string
		}{{input, leftPath, leftWidth, "West"}, {compareInput, rightPath, rightWidth, "East"}} {
			args := []string{side.input, "-auto-orient", "-resize", fmt.Sprintf("%dx%d^", side.width, canvas.Height), "-gravity", side.gravity, "-extent", fmt.Sprintf("%dx%d", side.width, canvas.Height), side.output}
			if err := runThumbCommand(ctx, magick, args...); err != nil {
				return fmt.Errorf("prepare split comparison image: %w", err)
			}
		}
		if err := runThumbCommand(ctx, magick, leftPath, rightPath, "+append", basePath); err != nil {
			return fmt.Errorf("combine comparison images: %w", err)
		}
	} else {
		baseArgs := []string{input, "-auto-orient", "-resize", fmt.Sprintf("%dx%d^", canvas.Width, canvas.Height)}
		gravity := map[string]string{"left": "West", "center": "Center", "right": "East"}[template.Layout.SubjectPosition]
		baseArgs = append(baseArgs, "-gravity", gravity, "-extent", fmt.Sprintf("%dx%d", canvas.Width, canvas.Height), basePath)
		if err := runThumbCommand(ctx, magick, baseArgs...); err != nil {
			return fmt.Errorf("prepare source image: %w", err)
		}
	}
	backgroundPath := basePath
	if template.Layout.Gradient != "none" {
		if err := writeThumbGradient(gradientPath, canvas.Width, canvas.Height, template.Layout.Gradient); err != nil {
			return err
		}
		if err := runThumbCommand(ctx, magick, basePath, gradientPath, "-compose", "over", "-composite", composedPath); err != nil {
			return fmt.Errorf("apply gradient: %w", err)
		}
		backgroundPath = composedPath
	}
	layout := template.Layout
	args := []string{backgroundPath, "-gravity", "NorthWest"}
	if layout.PanelColor != "" && layout.PanelOpacity > 0 {
		panelWidth := int(float64(canvas.Width) * 0.58)
		args = append(args, "-fill", rgbaThumb(layout.PanelColor, layout.PanelOpacity), "-draw", fmt.Sprintf("rectangle 0,0 %d,%d", panelWidth, canvas.Height))
	}
	if layout.LabelText != "" {
		args, err = appendThumbBadge(ctx, magick, args, font, layout.LabelText, layout.LabelBackground, layout.TextColor, layout.LabelPosition, canvas, layout.StickerStyle == "rounded-pill")
		if err != nil {
			return fmt.Errorf("measure template label: %w", err)
		}
	}
	if layout.ImageMode == "split-cover" {
		args, err = appendThumbBadge(ctx, magick, args, font, layout.LeftLabel, layout.LabelBackground, layout.LeftLabelColor, "top-left", canvas, true)
		if err != nil {
			return fmt.Errorf("measure left comparison label: %w", err)
		}
		args, err = appendThumbBadge(ctx, magick, args, font, layout.RightLabel, layout.LabelBackground, layout.RightLabelColor, "top-right", canvas, true)
		if err != nil {
			return fmt.Errorf("measure right comparison label: %w", err)
		}
		if layout.Divider {
			divider := maxInt(2, int(float64(canvas.Width)*layout.DividerWidthRatio))
			args = append(args, "-fill", layout.DividerColor, "-draw", fmt.Sprintf("rectangle %d,0 %d,%d", canvas.Width/2-divider/2, canvas.Width/2+divider/2, canvas.Height))
		}
	}
	if price != "" {
		badge := layout.PriceBadge
		args, err = appendThumbBadge(ctx, magick, args, font, badge.Prefix+price, badge.Background, badge.TextColor, badge.Position, canvas, true)
		if err != nil {
			return fmt.Errorf("measure price badge: %w", err)
		}
	}
	if secondaryText != "" {
		position := layout.SecondaryTextPosition
		if position == "" {
			position = "bottom-left"
		}
		args, err = appendThumbBadge(ctx, magick, args, font, secondaryText, layout.LabelBackground, layout.TextColor, position, canvas, true)
		if err != nil {
			return fmt.Errorf("measure secondary text: %w", err)
		}
	}
	if layout.Border {
		border := maxInt(2, int(float64(canvas.Width)*layout.BorderWidthRatio))
		inset := maxInt(5, int(float64(canvas.Width)*0.015))
		borderColor := layout.BorderColor
		if borderColor == "" {
			borderColor = layout.AccentColor
		}
		args = append(args, "-fill", "none", "-stroke", borderColor, "-strokewidth", strconv.Itoa(border), "-draw", fmt.Sprintf("rectangle %d,%d %d,%d", inset, inset, canvas.Width-inset, canvas.Height-inset), "-stroke", "none")
	}
	text := strings.Join(fitted.lines, "\n")
	x, y := thumbHeadlinePosition(layout, fitted, canvas)
	barWidth := maxInt(8, int(float64(canvas.Width)*0.012))
	barHeight := int(float64(fitted.pointSize) * float64(len(fitted.lines)) * 0.9)
	if layout.HeadlinePosition == "left-center" || layout.HeadlinePosition == "bottom-left" {
		barLeft := maxInt(0, x-barWidth-int(float64(canvas.Width)*0.02))
		barTop := y + (fitted.textHeight-barHeight)/2
		args = append(args, "-fill", layout.AccentColor, "-draw", fmt.Sprintf("rectangle %d,%d %d,%d", barLeft, barTop, barLeft+barWidth, barTop+barHeight))
	}
	args = append(args, "-font", font, "-pointsize", strconv.Itoa(fitted.pointSize))
	if layout.LetterSpacingRatio > 0 {
		args = append(args, "-kerning", strconv.FormatFloat(float64(fitted.pointSize)*layout.LetterSpacingRatio, 'f', 2, 64))
	}
	if layout.Shadow {
		args = append(args, "-fill", "rgba(0,0,0,0.68)", "-annotate", fmt.Sprintf("+%d+%d", x+2, y+2), text)
	}
	if layout.OutlineText {
		outline := layout.OutlineColor
		if outline == "" {
			outline = "#171717"
		}
		strokeWidth := maxInt(1, int(float64(fitted.pointSize)*layout.OutlineWidthRatio))
		args = append(args, "-fill", "none", "-stroke", outline, "-strokewidth", strconv.Itoa(strokeWidth), "-annotate", fmt.Sprintf("+%d+%d", x, y), text, "-stroke", "none")
	}
	args = append(args, "-fill", layout.TextColor, "-annotate", fmt.Sprintf("+%d+%d", x, y), text, "-colorspace", "sRGB", "-quality", strconv.Itoa(template.Export.Quality), output)
	if err := runThumbCommand(ctx, magick, args...); err != nil {
		return fmt.Errorf("draw thumbnail headline: %w", err)
	}
	return nil
}

func thumbHeadlinePosition(layout thumbLayout, fitted fittedThumbHeadline, canvas thumbCanvas) (int, int) {
	left, right := int(float64(canvas.Width)*0.055), int(float64(canvas.Width)*0.055)
	top, bottom := int(float64(canvas.Height)*0.05), int(float64(canvas.Height)*0.05)
	if layout.SafeArea.LeftRatio > 0 {
		left = int(float64(canvas.Width) * layout.SafeArea.LeftRatio)
	}
	if layout.SafeArea.RightRatio > 0 {
		right = int(float64(canvas.Width) * layout.SafeArea.RightRatio)
	}
	if layout.SafeArea.TopRatio > 0 {
		top = int(float64(canvas.Height) * layout.SafeArea.TopRatio)
	}
	if layout.SafeArea.BottomRatio > 0 {
		bottom = int(float64(canvas.Height) * layout.SafeArea.BottomRatio)
	}
	switch layout.HeadlinePosition {
	case "right-center":
		return canvas.Width - right - fitted.textWidth, (canvas.Height - fitted.textHeight) / 2
	case "center":
		return (canvas.Width - fitted.textWidth) / 2, (canvas.Height - fitted.textHeight) / 2
	case "top-center":
		if layout.LabelText != "" {
			top = maxInt(top, int(float64(canvas.Height)*0.14))
		}
		return (canvas.Width - fitted.textWidth) / 2, top
	case "bottom-left":
		return left, canvas.Height - bottom - fitted.textHeight
	default:
		return left, (canvas.Height - fitted.textHeight) / 2
	}
}

func appendThumbBadge(ctx context.Context, magick string, args []string, font, text, background, textColor, position string, canvas thumbCanvas, rounded bool) ([]string, error) {
	if text == "" {
		return args, nil
	}
	if background == "" {
		background = "#171717"
	}
	if textColor == "" {
		textColor = "#FFFFFF"
	}
	pointSize := maxInt(16, int(float64(canvas.Height)*0.032))
	w, h, err := measureThumbText(ctx, magick, font, pointSize, text)
	if err != nil {
		return nil, err
	}
	padX, padY := maxInt(10, pointSize/2), maxInt(6, pointSize/3)
	boxW, boxH := w+2*padX, h+2*padY
	margin := maxInt(18, int(float64(canvas.Width)*0.035))
	x, y := margin, margin
	switch position {
	case "center-top":
		x = (canvas.Width - boxW) / 2
	case "top-right":
		x = canvas.Width - margin - boxW
	case "bottom-left":
		y = canvas.Height - margin - boxH
	case "bottom-right":
		x, y = canvas.Width-margin-boxW, canvas.Height-margin-boxH
	case "bottom-center":
		x, y = (canvas.Width-boxW)/2, canvas.Height-margin-boxH
	}
	shape := "rectangle"
	if rounded {
		shape = "roundrectangle"
	}
	draw := fmt.Sprintf("%s %d,%d %d,%d", shape, x, y, x+boxW, y+boxH)
	if rounded {
		draw += fmt.Sprintf(" %d,%d", pointSize/2, pointSize/2)
	}
	args = append(args, "-fill", background, "-draw", draw, "-font", font, "-pointsize", strconv.Itoa(pointSize), "-fill", textColor, "-annotate", fmt.Sprintf("+%d+%d", x+padX, y+padY), text)
	return args, nil
}

func rgbaThumb(hex string, alpha float64) string {
	return fmt.Sprintf("rgba(%d,%d,%d,%.3f)", mustHexByte(hex[1:3]), mustHexByte(hex[3:5]), mustHexByte(hex[5:7]), alpha)
}

func mustHexByte(value string) int {
	n, _ := strconv.ParseInt(value, 16, 32)
	return int(n)
}

func fitThumbHeadline(ctx context.Context, magick, font, headline string, layout thumbLayout, canvas thumbCanvas) ([]string, int, int, error) {
	words := strings.Fields(headline)
	maxWidth := thumbHeadlineMaxWidth(layout, canvas)
	maxHeight := int(float64(canvas.Height) * 0.72)
	maxSize := int(math.Round(float64(canvas.Height) * layout.FontSizeRatio))
	minSize := maxInt(12, int(math.Round(float64(canvas.Height)*0.035)))
	for size := maxInt(12, maxSize); size >= minSize; size -= 2 {
		lines := wrapThumbWords(words, maxWidth, size)
		if len(lines) == 0 || len(lines) > layout.HeadlineMaxLines {
			continue
		}
		w, h, err := measureThumbText(ctx, magick, font, size, strings.Join(lines, "\n"))
		if err != nil {
			return nil, 0, 0, err
		}
		if w <= maxWidth && h <= maxHeight {
			return lines, size, w, nil
		}
	}
	return nil, 0, 0, fmt.Errorf("headline cannot fit within %d lines at the minimum font size", layout.HeadlineMaxLines)
}

func thumbHeadlineMaxWidth(layout thumbLayout, canvas thumbCanvas) int {
	left, right := int(float64(canvas.Width)*0.055), int(float64(canvas.Width)*0.055)
	if layout.SafeArea.LeftRatio > 0 {
		left = int(float64(canvas.Width) * layout.SafeArea.LeftRatio)
	}
	if layout.SafeArea.RightRatio > 0 {
		right = int(float64(canvas.Width) * layout.SafeArea.RightRatio)
	}
	available := canvas.Width - left - right
	if layout.HeadlinePosition == "left-center" || layout.HeadlinePosition == "right-center" {
		return minInt(available, int(float64(canvas.Width)*0.43))
	}
	return minInt(available, int(float64(canvas.Width)*0.84))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func wrapThumbWords(words []string, maxWidth, size int) []string {
	lines := make([]string, 0)
	line := ""
	for _, word := range words {
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if line != "" && estimateThumbTextWidth(candidate, size) > maxWidth {
			lines = append(lines, line)
			line = word
		} else {
			line = candidate
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func estimateThumbTextWidth(text string, size int) int {
	units := 0.0
	for _, r := range text {
		switch {
		case r == ' ':
			units += 0.34
		case strings.ContainsRune("I!.,'|", r):
			units += 0.32
		case strings.ContainsRune("MW@#%", r):
			units += 0.9
		default:
			units += 0.62
		}
	}
	return int(math.Ceil(units * float64(size)))
}

func measureThumbText(ctx context.Context, magick, font string, size int, text string) (int, int, error) {
	output, err := exec.CommandContext(ctx, magick, "-font", font, "-pointsize", strconv.Itoa(size), "-background", "none", "label:"+text, "-format", "%w %h", "info:").CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("measure headline with ImageMagick: %w: %s", err, strings.TrimSpace(string(output)))
	}
	parts := strings.Fields(string(output))
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected ImageMagick text measurement %q", output)
	}
	w, errW := strconv.Atoi(parts[0])
	h, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil {
		return 0, 0, fmt.Errorf("invalid ImageMagick text measurement %q", output)
	}
	return w, h, nil
}

func resolveThumbFont(font string) (string, error) {
	if font == "sans-serif" {
		if path := findSystemThumbFont(); path != "" {
			return path, nil
		}
	}
	if !strings.ContainsAny(font, `/\\`) {
		return font, nil
	}
	path := font
	if !filepath.IsAbs(path) {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve template font %q: %w", font, err)
		}
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		if strings.HasPrefix(filepath.ToSlash(font), "assets/fonts/") {
			if fallback := findSystemThumbFont(); fallback != "" {
				return fallback, nil
			}
		}
		return "", fmt.Errorf("template font %q was not found; use a font family name or install the referenced font", font)
	}
	return path, nil
}

func findSystemThumbFont() string {
	candidates := []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation2/LiberationSans-Regular.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
		"/System/Library/Fonts/HelveticaNeue.ttc",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
	}
	if runtime.GOOS == "darwin" {
		candidates = append([]string{
			"/System/Library/Fonts/HelveticaNeue.ttc",
			"/System/Library/Fonts/Supplemental/Arial.ttf",
		}, candidates...)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	return ""
}

func writeThumbGradient(path string, width, height int, direction string) error {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	soft := strings.HasSuffix(direction, "-soft")
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var t float64
			if strings.HasPrefix(direction, "bottom-up") {
				t = float64(height-1-y) / float64(maxInt(1, height-1))
			} else {
				t = float64(x) / float64(maxInt(1, width-1))
			}
			falloff := 0.65
			power := 1.5
			if soft {
				falloff, power = 0.9, 2.2
			}
			alpha := uint8(math.Round(190 * math.Pow(maxFloat(0, 1-t/falloff), power)))
			img.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: alpha})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create gradient layer: %w", err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return fmt.Errorf("write gradient layer: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close gradient layer: %w", err)
	}
	return nil
}

func runThumbCommand(ctx context.Context, magick string, args ...string) error {
	cmd := exec.CommandContext(ctx, magick, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("thumbnail rendering cancelled or timed out")
		}
		return fmt.Errorf("ImageMagick failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func readThumbHeadlines(single, path string) ([]string, error) {
	if path == "" {
		return []string{strings.TrimSpace(single)}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open headlines file %q: %w", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var headlines []string
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			headlines = append(headlines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read headlines file %q: %w", path, err)
	}
	return headlines, nil
}

func thumbOutputNames(output string, count int) []string {
	if count == 1 {
		return []string{output}
	}
	ext := filepath.Ext(output)
	base := strings.TrimSuffix(output, ext)
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("%s-%03d%s", base, i+1, ext)
	}
	return names
}

func findThumbTemplate(catalog thumbTemplateCatalog, id string) (thumbTemplate, bool) {
	for _, item := range catalog.Templates {
		if item.ID == id {
			return item, true
		}
	}
	return thumbTemplate{}, false
}

func thumbTemplateIDs(catalog thumbTemplateCatalog) []string {
	ids := make([]string, 0, len(catalog.Templates))
	for _, item := range catalog.Templates {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	return ids
}

func countHeadlineWords(text string) int { return len(strings.Fields(text)) }

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func printThumbTemplateList(w io.Writer, catalog thumbTemplateCatalog) {
	fmt.Fprintln(w, "Available thumbnail templates:")
	for _, item := range catalog.Templates {
		fmt.Fprintf(w, "  %-16s %s\n", item.ID, item.Description)
	}
}

func printThumbHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: evuedit --module evuthumb --template <id> --platform <youtube|instagram> --file <image> (--headline \"text\" | --headlines-file <path>) --output <filename.jpg> [template options]")
	fmt.Fprintln(w, "Create deterministic image thumbnails from JSON templates. Outputs are written to ~/evueditout.")
	fmt.Fprintln(w, "Use --list to show templates. Headline files use one headline per non-empty line.")
	fmt.Fprintln(w, "Headline text wraps and shrinks to fit the template's word and line limits.")
	fmt.Fprintln(w, "Template options: --compare-file <image> (food-comparison), --price <text> (food-deal), --secondary-text <text> (restaurant-review).")
	fmt.Fprintln(w)
	if catalog, err := loadEmbeddedThumbTemplateCatalog(); err == nil {
		printThumbTemplateList(w, catalog)
	}
}
