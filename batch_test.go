package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFolderBatchSortsFilesSkipsUnsupportedAndContinuesFailures(t *testing.T) {
	// Shared folder mode must be nonrecursive, alphabetically stable, report skipped files, and process later inputs after a failure.
	dir := t.TempDir()
	for name, content := range map[string]string{"z.mov": "", "B.mp4": "", "a.mkv": "", "notes.txt": ""} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	var stdout, stderr bytes.Buffer
	var processed []string
	handled, err := runFolderBatch([]string{"--input-dir=" + dir, "--output", "base.mp4", "--apply", "masala-gold"}, &stdout, &stderr, []string{".mp4", ".mov", ".mkv"}, func(_ []string, input, _ string) (folderBatchTarget, error) {
		name, err := batchOutputName(input, "masala-gold", ".mp4")
		return folderBatchTarget{output: name}, err
	}, func(args []string, _, _ io.Writer) error {
		file, err := batchFlagValue(args, "--file")
		if err != nil {
			return err
		}
		processed = append(processed, filepath.Base(file))
		if filepath.Base(file) == "B.mp4" {
			return errors.New("simulated failure")
		}
		return nil
	})
	if !handled || err == nil || !strings.Contains(err.Error(), "1 failure") {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if want := []string{"a.mkv", "B.mp4", "z.mov"}; !reflect.DeepEqual(processed, want) {
		t.Fatalf("processed=%v want=%v", processed, want)
	}
	if !strings.Contains(stderr.String(), `Skipping unsupported file "notes.txt"`) || !strings.Contains(stdout.String(), "2 succeeded, 1 failed, 1 unsupported") {
		t.Fatalf("batch reporting incomplete: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestFolderBatchPreflightsAllOutputsAndRejectsMixedInputModes(t *testing.T) {
	// Batch mode must detect duplicate stems and file/folder ambiguity before invoking a module.
	dir := t.TempDir()
	for _, name := range []string{"dish.jpg", "dish.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	invoked := false
	plan := func(_ []string, input, _ string) (folderBatchTarget, error) {
		name, err := batchOutputName(input, "warm", ".jpg")
		return folderBatchTarget{output: name}, err
	}
	invoke := func([]string, io.Writer, io.Writer) error { invoked = true; return nil }
	handled, err := runFolderBatch([]string{"--input-dir", dir, "--output", "unused.jpg"}, &stdout, &stderr, []string{".jpg", ".png"}, plan, invoke)
	if !handled || err == nil || !strings.Contains(err.Error(), "collision") || invoked {
		t.Fatalf("collision preflight: handled=%v err=%v invoked=%v", handled, err, invoked)
	}
	_, err = runFolderBatch([]string{"--input-dir", dir, "--file", "x.jpg", "--output", "unused.jpg"}, &stdout, &stderr, []string{".jpg"}, plan, invoke)
	if err == nil || !strings.Contains(err.Error(), "either --file or --input-dir") {
		t.Fatalf("mixed input error=%v", err)
	}
}
