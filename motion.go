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
	"path/filepath"
	"strings"
	"time"
)

//go:embed motion_templates.json
var motionTemplateConfig embed.FS

type motionTool struct{}

func (motionTool) Name() string { return "evumotion" }
func (motionTool) Description() string {
	return "create short animated videos from image and text templates"
}

type motionCatalog struct {
	Version   int              `json:"version"`
	Templates []motionTemplate `json:"templates"`
}
type motionTemplate struct {
	ID          string         `json:"id"`
	Version     int            `json:"version"`
	Description string         `json:"description"`
	Canvas      motionCanvas   `json:"canvas"`
	Duration    float64        `json:"durationSeconds"`
	FPS         int            `json:"fps"`
	Headline    motionHeadline `json:"headline"`
	Subject     motionSubject  `json:"subject"`
	Export      motionExport   `json:"export"`
}
type motionCanvas struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type motionHeadline struct {
	Font      string  `json:"font"`
	FontSize  int     `json:"fontSize"`
	Color     string  `json:"color"`
	Position  string  `json:"position"`
	Animation string  `json:"animation"`
	Start     float64 `json:"startSeconds"`
	Duration  float64 `json:"durationSeconds"`
}
type motionSubject struct {
	MaxWidth  float64 `json:"maxWidthRatio"`
	MaxHeight float64 `json:"maxHeightRatio"`
	Position  string  `json:"position"`
	Animation string  `json:"animation"`
}
type motionExport struct {
	Format      string `json:"format"`
	VideoCodec  string `json:"videoCodec"`
	PixelFormat string `json:"pixelFormat"`
	CRF         int    `json:"crf"`
}

func loadMotionCatalog() (motionCatalog, error) {
	data, err := motionTemplateConfig.ReadFile("motion_templates.json")
	if err != nil {
		return motionCatalog{}, err
	}
	var c motionCatalog
	if err = json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("parse motion template catalog: %w", err)
	}
	if c.Version != 1 || len(c.Templates) == 0 {
		return c, errors.New("motion template catalog must have version 1 and at least one template")
	}
	seen := map[string]bool{}
	for _, t := range c.Templates {
		if t.ID == "" || seen[t.ID] {
			return c, fmt.Errorf("motion template catalog contains empty or duplicate id %q", t.ID)
		}
		seen[t.ID] = true
		if t.Version != 1 || t.Description == "" || t.Canvas.Width < 1 || t.Canvas.Height < 1 || t.Duration <= 0 || t.FPS < 1 || t.FPS > 60 {
			return c, fmt.Errorf("template %q has invalid version, description, canvas, duration, or frame rate", t.ID)
		}
		if t.Headline.Font == "" || t.Headline.FontSize < 1 || !strings.HasPrefix(t.Headline.Color, "#") && t.Headline.Color != "white" || (t.Headline.Position != "bottom-center" && t.Headline.Position != "center") || (t.Headline.Animation != "fade" && t.Headline.Animation != "slide-up" && t.Headline.Animation != "pop") || t.Headline.Start < 0 || t.Headline.Duration <= 0 || t.Headline.Start+t.Headline.Duration > t.Duration {
			return c, fmt.Errorf("template %q has invalid headline settings", t.ID)
		}
		if t.Subject.MaxWidth <= 0 || t.Subject.MaxWidth > 1 || t.Subject.MaxHeight <= 0 || t.Subject.MaxHeight > 1 || t.Subject.Position != "center" || (t.Subject.Animation != "none" && t.Subject.Animation != "float" && t.Subject.Animation != "slide-up") {
			return c, fmt.Errorf("template %q has invalid subject settings", t.ID)
		}
		if t.Export.Format != "mp4" || t.Export.VideoCodec != "libx264" || t.Export.PixelFormat != "yuv420p" || t.Export.CRF < 0 || t.Export.CRF > 51 {
			return c, fmt.Errorf("template %q has invalid MP4 export settings", t.ID)
		}
	}
	return c, nil
}

func (motionTool) Run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("evumotion", flag.ContinueOnError)
	fs.SetOutput(stderr)
	templateID := fs.String("template", "", "motion template id")
	platform := fs.String("platform", "", "youtube, instagram, or facebook (vertical short-form output)")
	input := fs.String("file", "", "background image (JPEG, PNG, or WebP)")
	foreground := fs.String("foreground", "", "optional transparent subject cutout (PNG or WebP)")
	headline := fs.String("headline", "", "headline text")
	output := fs.String("output", "", "MP4 filename written to ~/evueditout")
	list := fs.Bool("list", false, "list motion templates")
	fs.Usage = func() { printMotionHelp(stdout) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	catalog, err := loadMotionCatalog()
	if err != nil {
		return err
	}
	if *list {
		if *templateID != "" || *platform != "" || *input != "" || *foreground != "" || *headline != "" || *output != "" {
			return errors.New("--list cannot be combined with generation options")
		}
		for _, t := range catalog.Templates {
			fmt.Fprintf(stdout, "%-16s %s\n", t.ID, t.Description)
		}
		return nil
	}
	if *templateID == "" || *platform == "" || *input == "" || strings.TrimSpace(*headline) == "" || *output == "" {
		return errors.New("--template, --platform, --file, --headline, and --output are required")
	}
	if *platform != "youtube" && *platform != "instagram" && *platform != "facebook" {
		return errors.New("--platform must be youtube, instagram, or facebook")
	}
	var tmpl *motionTemplate
	for i := range catalog.Templates {
		if catalog.Templates[i].ID == *templateID {
			tmpl = &catalog.Templates[i]
			break
		}
	}
	if tmpl == nil {
		return fmt.Errorf("unknown motion template %q", *templateID)
	}
	in, err := filepath.Abs(*input)
	if err != nil {
		return err
	}
	if err = validateMotionInput(in, ".jpg", ".jpeg", ".png", ".webp"); err != nil {
		return err
	}
	fg := ""
	if *foreground != "" {
		fg, err = filepath.Abs(*foreground)
		if err != nil {
			return err
		}
		if err = validateMotionInput(fg, ".png", ".webp"); err != nil {
			return fmt.Errorf("foreground: %w", err)
		}
	}
	if filepath.Ext(*output) != ".mp4" {
		return errors.New("--output must end in .mp4")
	}
	if filepath.Base(*output) != *output || *output == "." || *output == ".." {
		return errors.New("--output must be a filename only; files are written to ~/evueditout")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	outDir := filepath.Join(home, "evueditout")
	if err = os.MkdirAll(outDir, 0755); err != nil {
		return err
	}
	out := filepath.Join(outDir, *output)
	if filepath.Clean(in) == filepath.Clean(out) || filepath.Clean(fg) == filepath.Clean(out) {
		return errors.New("input and output must be different paths")
	}
	if _, err = os.Stat(out); err == nil {
		return fmt.Errorf("output %q already exists; choose another filename", *output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return errors.New("ffmpeg was not found on PATH; install FFmpeg and try again")
	}
	textFile, err := os.CreateTemp("", "evuedit-headline-*.txt")
	if err != nil {
		return err
	}
	textPath := textFile.Name()
	defer os.Remove(textPath)
	if _, err = textFile.WriteString(*headline); err != nil {
		textFile.Close()
		return err
	}
	if err = textFile.Close(); err != nil {
		return err
	}
	argsFF := []string{"-hide_banner", "-loglevel", "error", "-y", "-loop", "1", "-framerate", fmt.Sprint(tmpl.FPS), "-i", in}
	if fg != "" {
		argsFF = append(argsFF, "-loop", "1", "-framerate", fmt.Sprint(tmpl.FPS), "-i", fg)
	}
	graph := motionFilterGraph(*tmpl, textPath, fg != "")
	argsFF = append(argsFF, "-filter_complex", graph, "-map", "[outv]", "-t", fmt.Sprintf("%.3f", tmpl.Duration), "-r", fmt.Sprint(tmpl.FPS), "-an", "-c:v", tmpl.Export.VideoCodec, "-pix_fmt", tmpl.Export.PixelFormat, "-color_range", "tv", "-crf", fmt.Sprint(tmpl.Export.CRF), "-movflags", "+faststart", out)
	reservation, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output %q already exists; choose another filename", *output)
		}
		return fmt.Errorf("create output %q: %w", *output, err)
	}
	if err = reservation.Close(); err != nil {
		os.Remove(out)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, argsFF...)
	cmd.Stderr = stderr
	fmt.Fprintf(stdout, "Creating %s %s video...\n", tmpl.ID, *platform)
	if err = cmd.Run(); err != nil {
		os.Remove(out)
		return fmt.Errorf("FFmpeg could not create the video: %w", err)
	}
	fmt.Fprintf(stdout, "Created %s\n", out)
	return nil
}

func validateMotionInput(path string, exts ...string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read input %q: %w", path, err)
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("input %q is not a regular file", path)
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, allowed := range exts {
		if ext == allowed {
			return nil
		}
	}
	return fmt.Errorf("unsupported input format %q", ext)
}
func motionFilterGraph(t motionTemplate, textFile string, hasForeground bool) string {
	w, h := t.Canvas.Width, t.Canvas.Height
	bg := fmt.Sprintf("[0:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1[bg]", w, h, w, h)
	base := "[bg]"
	if hasForeground {
		fgw := int(float64(w) * t.Subject.MaxWidth)
		fgh := int(float64(h) * t.Subject.MaxHeight)
		moveY := "(H-h)/2"
		if t.Subject.Animation == "float" {
			moveY = "(H-h)/2+18*sin(t*2)"
		}
		if t.Subject.Animation == "slide-up" {
			moveY = "if(lt(t,1),H,(H-h)/2)"
		}
		bg += fmt.Sprintf(";[1:v]scale=%d:%d:force_original_aspect_ratio=decrease,format=rgba[subject];[bg][subject]overlay=x='(W-w)/2':y='%s':shortest=1[composed]", fgw, fgh, moveY)
		base = "[composed]"
	}
	anim := "if(lt(t\\," + fmt.Sprintf("%.3f", t.Headline.Start) + ")\\,0\\,if(lt(t\\," + fmt.Sprintf("%.3f", t.Headline.Start+t.Headline.Duration) + ")\\,(t-" + fmt.Sprintf("%.3f", t.Headline.Start) + ")/" + fmt.Sprintf("%.3f", t.Headline.Duration) + "\\,1))"
	fontsize := fmt.Sprint(t.Headline.FontSize)
	x := "(w-text_w)/2"
	y := "(h-text_h)/2"
	if t.Headline.Position == "bottom-center" {
		y = "h-text_h-180"
	}
	switch t.Headline.Animation {
	case "slide-up":
		y = "h-text_h-180+120*(1-min(1\\,max(0\\,(t-" + fmt.Sprintf("%.3f", t.Headline.Start) + ")/" + fmt.Sprintf("%.3f", t.Headline.Duration) + ")))"
	case "pop":
		fontsize = "" + fmt.Sprint(t.Headline.FontSize) + "*min(1\\,max(0\\,(t-" + fmt.Sprintf("%.3f", t.Headline.Start) + ")/" + fmt.Sprintf("%.3f", t.Headline.Duration) + "))"
	}
	return bg + fmt.Sprintf(";%sdrawtext=font='%s':textfile='%s':fontsize=%s:fontcolor=%s:x='%s':y='%s':alpha='%s',format=yuv420p[outv]", base, t.Headline.Font, textFile, fontsize, t.Headline.Color, x, y, anim)
}
func printMotionHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: evuedit --module evumotion --template <id> --platform <youtube|instagram|facebook> --file <image> [--foreground <transparent-subject.png>] --headline \"text\" --output <video.mp4>")
	fmt.Fprintln(w, "Create a silent portrait MP4 using a built-in template. Output is written to ~/evueditout.")
	fmt.Fprintln(w, "Use --list to show templates. A transparent foreground image enables separate subject motion.")
}
