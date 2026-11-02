package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type folderBatchTarget struct {
	output string
	names  []string
}

// runFolderBatch handles the shared input contract and leaves each module's
// processing and validation in its existing single-file Run implementation.
func runFolderBatch(args []string, stdout, stderr io.Writer, extensions []string, plan func([]string, string, string) (folderBatchTarget, error), invoke func([]string, io.Writer, io.Writer) error) (bool, error) {
	dir, clean, requested, err := takeLongOption(args, "--input-dir")
	if err != nil || !requested {
		return requested, err
	}
	if _, _, hasFile, fileErr := takeLongOption(clean, "--file"); fileErr != nil {
		return true, fileErr
	} else if hasFile {
		return true, errors.New("use either --file or --input-dir, not both")
	}
	outputBase, clean, found, err := takeLongOption(clean, "--output")
	if err != nil {
		return true, err
	}
	if !found || outputBase == "" {
		return true, errors.New("--output is required in folder mode; it selects the output format")
	}
	if filepath.Base(outputBase) != outputBase || outputBase == "." || outputBase == ".." {
		return true, errors.New("--output must be a filename only; batch outputs are written to ~/evueditout")
	}

	folder, err := filepath.Abs(dir)
	if err != nil {
		return true, fmt.Errorf("resolve input folder: %w", err)
	}
	info, err := os.Stat(folder)
	if err != nil {
		return true, fmt.Errorf("read input folder %q: %w", dir, err)
	}
	if !info.IsDir() {
		return true, fmt.Errorf("input %q is not a folder", dir)
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return true, fmt.Errorf("list input folder %q: %w", dir, err)
	}
	extSet := make(map[string]bool, len(extensions))
	for _, ext := range extensions {
		extSet[ext] = true
	}
	var files []string
	var skipped []string
	for _, entry := range entries {
		path := filepath.Join(folder, entry.Name())
		fileInfo, infoErr := entry.Info()
		if infoErr != nil {
			return true, fmt.Errorf("inspect %q: %w", entry.Name(), infoErr)
		}
		if !fileInfo.Mode().IsRegular() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if strings.HasPrefix(entry.Name(), "._") || !extSet[ext] {
			skipped = append(skipped, entry.Name())
			continue
		}
		files = append(files, path)
	}
	sort.Slice(files, func(i, j int) bool {
		a, b := strings.ToLower(filepath.Base(files[i])), strings.ToLower(filepath.Base(files[j]))
		if a == b {
			return filepath.Base(files[i]) < filepath.Base(files[j])
		}
		return a < b
	})
	for _, name := range skipped {
		fmt.Fprintf(stderr, "Skipping unsupported file %q\n", name)
	}
	if len(files) == 0 {
		return true, fmt.Errorf("no supported input files found in %q", dir)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return true, fmt.Errorf("find home directory for output: %w", err)
	}
	outputDir := filepath.Join(home, "evueditout")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return true, fmt.Errorf("create output directory %q: %w", outputDir, err)
	}
	type job struct {
		input  string
		target folderBatchTarget
	}
	jobs := make([]job, 0, len(files))
	planned := make(map[string]string)
	for _, file := range files {
		target, planErr := plan(clean, file, outputBase)
		if planErr != nil {
			return true, fmt.Errorf("plan output for %q: %w", filepath.Base(file), planErr)
		}
		if target.output == "" || filepath.Base(target.output) != target.output || target.output == "." || target.output == ".." {
			return true, fmt.Errorf("unsafe batch output filename %q", target.output)
		}
		if len(target.names) == 0 {
			target.names = []string{target.output}
		}
		for _, name := range target.names {
			if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
				return true, fmt.Errorf("unsafe batch output filename %q", name)
			}
			path := filepath.Join(outputDir, name)
			key := strings.ToLower(filepath.Clean(path))
			if previous, exists := planned[key]; exists {
				return true, fmt.Errorf("batch output collision: %q and %q both produce %q", filepath.Base(previous), filepath.Base(file), name)
			}
			planned[key] = file
			if _, statErr := os.Stat(path); statErr == nil {
				return true, fmt.Errorf("output %q already exists; choose another output name", name)
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return true, fmt.Errorf("check output %q: %w", name, statErr)
			}
		}
		jobs = append(jobs, job{input: file, target: target})
	}

	fmt.Fprintf(stdout, "Batch: %d supported input file(s) in %s\n", len(jobs), folder)
	succeeded := 0
	var failures []string
	for _, item := range jobs {
		oneArgs := append([]string(nil), clean...)
		oneArgs = append(oneArgs, "--file", item.input, "--output", item.target.output)
		if runErr := invoke(oneArgs, stdout, stderr); runErr != nil {
			failures = append(failures, filepath.Base(item.input)+": "+runErr.Error())
			fmt.Fprintf(stderr, "Failed %s: %v\n", filepath.Base(item.input), runErr)
			continue
		}
		succeeded++
	}
	fmt.Fprintf(stdout, "Batch complete: %d succeeded, %d failed, %d unsupported file(s) skipped.\n", succeeded, len(failures), len(skipped))
	if len(failures) > 0 {
		return true, fmt.Errorf("folder batch finished with %d failure(s)", len(failures))
	}
	return true, nil
}

// takeLongOption removes a flag and its value from args. It accepts --name value
// and --name=value, and rejects duplicates or a missing value.
func takeLongOption(args []string, name string) (value string, rest []string, found bool, err error) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == name {
			if found {
				return "", nil, true, fmt.Errorf("%s may only be specified once", name)
			}
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", nil, true, fmt.Errorf("%s requires a value", name)
			}
			value, found = args[i+1], true
			i++
			continue
		}
		if strings.HasPrefix(arg, name+"=") {
			if found {
				return "", nil, true, fmt.Errorf("%s may only be specified once", name)
			}
			value, found = strings.TrimPrefix(arg, name+"="), true
			continue
		}
		rest = append(rest, arg)
	}
	return value, rest, found, nil
}

func batchOutputName(input, operation, extension string) (string, error) {
	if operation == "" || extension == "" || strings.Trim(extension, ".") == "" {
		return "", errors.New("could not determine output name")
	}
	for _, r := range operation {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return "", fmt.Errorf("invalid output label %q", operation)
		}
	}
	stem := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	if stem == "" || stem == "." || stem == ".." {
		return "", fmt.Errorf("cannot make an output name from %q", filepath.Base(input))
	}
	return stem + "-" + operation + extension, nil
}

func batchFlagValue(args []string, name string) (string, error) {
	value, _, found, err := takeLongOption(args, name)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("%s is required in folder mode", name)
	}
	return value, nil
}

func runPresetFolderBatch(args []string, stdout, stderr io.Writer) (bool, error) {
	return runFolderBatch(args, stdout, stderr, []string{".mp4", ".mov", ".m4v", ".mkv"}, func(options []string, input, outputArg string) (folderBatchTarget, error) {
		preset, err := batchFlagValue(options, "--apply")
		if err != nil {
			return folderBatchTarget{}, err
		}
		if strings.ToLower(filepath.Ext(outputArg)) != ".mp4" {
			return folderBatchTarget{}, errors.New("--output must end in .mp4")
		}
		name, err := batchOutputName(input, preset, ".mp4")
		return folderBatchTarget{output: name}, err
	}, func(oneArgs []string, stdout, stderr io.Writer) error {
		return (presetTool{}).Run(oneArgs, stdout, stderr)
	})
}

func runImageFilterFolderBatch(args []string, stdout, stderr io.Writer) (bool, error) {
	return runFolderBatch(args, stdout, stderr, []string{".jpg", ".jpeg", ".png", ".webp"}, func(options []string, input, outputArg string) (folderBatchTarget, error) {
		filter, err := batchFlagValue(options, "--filter")
		if err != nil {
			return folderBatchTarget{}, err
		}
		ext := strings.ToLower(filepath.Ext(outputArg))
		format, _, _, err := takeLongOption(options, "--format")
		if err != nil {
			return folderBatchTarget{}, err
		}
		if format != "" {
			formatExt, ok := imageFormatExtension(format)
			if !ok {
				return folderBatchTarget{}, errors.New("--format must be jpg, png, or webp")
			}
			if ext != "" && !sameImageFormat(ext, formatExt) {
				return folderBatchTarget{}, fmt.Errorf("output filename extension %q conflicts with --format %q", ext, format)
			}
			ext = formatExt
		}
		if !supportedImageExtension(ext) {
			return folderBatchTarget{}, errors.New("--output must have a supported image extension or use --format")
		}
		if filter != "all" {
			name, err := batchOutputName(input, filter, ext)
			return folderBatchTarget{output: name}, err
		}
		filters, err := loadEmbeddedImageFilters()
		if err != nil {
			return folderBatchTarget{}, err
		}
		names := make([]string, 0, len(filters))
		for _, item := range filters {
			name, nameErr := batchOutputName(input, item.name, ext)
			if nameErr != nil {
				return folderBatchTarget{}, nameErr
			}
			names = append(names, name)
		}
		stem := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
		return folderBatchTarget{output: stem + ext, names: names}, nil
	}, func(oneArgs []string, stdout, stderr io.Writer) error {
		return (imageFilterTool{}).Run(oneArgs, stdout, stderr)
	})
}

func runThumbFolderBatch(args []string, stdout, stderr io.Writer) (bool, error) {
	return runFolderBatch(args, stdout, stderr, []string{".jpg", ".jpeg", ".png", ".webp"}, func(options []string, input, outputArg string) (folderBatchTarget, error) {
		template, err := batchFlagValue(options, "--template")
		if err != nil {
			return folderBatchTarget{}, err
		}
		headline, _, hasHeadline, err := takeLongOption(options, "--headline")
		if err != nil {
			return folderBatchTarget{}, err
		}
		headlinesPath, _, hasFile, err := takeLongOption(options, "--headlines-file")
		if err != nil {
			return folderBatchTarget{}, err
		}
		if hasHeadline == hasFile {
			return folderBatchTarget{}, errors.New("provide exactly one of --headline or --headlines-file")
		}
		headlines, err := readThumbHeadlines(headline, headlinesPath)
		if err != nil {
			return folderBatchTarget{}, err
		}
		if strings.ToLower(filepath.Ext(outputArg)) != ".jpg" && strings.ToLower(filepath.Ext(outputArg)) != ".jpeg" {
			return folderBatchTarget{}, errors.New("--output must end in .jpg or .jpeg")
		}
		base, err := batchOutputName(input, template, strings.ToLower(filepath.Ext(outputArg)))
		if err != nil {
			return folderBatchTarget{}, err
		}
		return folderBatchTarget{output: base, names: thumbOutputNames(base, len(headlines))}, nil
	}, func(oneArgs []string, stdout, stderr io.Writer) error {
		return (thumbTool{}).Run(oneArgs, stdout, stderr)
	})
}

func runMotionFolderBatch(args []string, stdout, stderr io.Writer) (bool, error) {
	return runFolderBatch(args, stdout, stderr, []string{".jpg", ".jpeg", ".png", ".webp"}, func(options []string, input, outputArg string) (folderBatchTarget, error) {
		template, err := batchFlagValue(options, "--template")
		if err != nil {
			return folderBatchTarget{}, err
		}
		if strings.ToLower(filepath.Ext(outputArg)) != ".mp4" {
			return folderBatchTarget{}, errors.New("--output must end in .mp4")
		}
		name, err := batchOutputName(input, template, ".mp4")
		return folderBatchTarget{output: name}, err
	}, func(oneArgs []string, stdout, stderr io.Writer) error {
		return (motionTool{}).Run(oneArgs, stdout, stderr)
	})
}
