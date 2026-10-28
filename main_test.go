package main

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

type stubTool struct {
	name   string
	args   []string
	called bool
}

func (s *stubTool) Name() string        { return s.name }
func (s *stubTool) Description() string { return "test tool" }
func (s *stubTool) Run(args []string, _, _ io.Writer) error {
	s.called = true
	s.args = append([]string(nil), args...)
	return nil
}

func TestExtractToolKeepsToolOptionsAndSupportsEqualsSyntax(t *testing.T) {
	// The dispatcher must remove only --tool and pass all tool-specific options through unchanged.
	name, args, err := extractTool([]string{"--apply", "cool", "--tool=preset", "--file", "clip.mov"})
	if err != nil {
		t.Fatalf("extractTool returned error: %v", err)
	}
	if name != "preset" {
		t.Fatalf("tool name = %q, want preset", name)
	}
	want := []string{"--apply", "cool", "--file", "clip.mov"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("tool args = %#v, want %#v", args, want)
	}
}

func TestRunSupportsPresetsModuleAliasAndDetailedHelp(t *testing.T) {
	// The plural module alias should select preset help and retain module-owned flags.
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--module", "presets", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	for _, expected := range []string{"Built-in presets and parameters", "masala-gold", "tags: food, warm, natural", "exposureEV=0 contrast=1.08 saturation=1.08", "lutIntensity=0"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("module help does not contain %q", expected)
		}
	}
}

func TestRunDispatchesToEvufiltersModule(t *testing.T) {
	// The image-processing module should be available through global --module dispatch.
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--module", "evufilters", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "--filter <name|all>") || !strings.Contains(stdout.String(), "monochrome") {
		t.Fatalf("evufilters help was not displayed: %s", stdout.String())
	}
}

func TestRunDispatchesToEvuthumbModule(t *testing.T) {
	// Thumbnail module help should be reachable through the shared tool registry.
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--module", "evuthumb", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "--template") || !strings.Contains(stdout.String(), "--headlines-file") {
		t.Fatalf("evuthumb help was not displayed: %s", stdout.String())
	}
}

func TestExtractToolRejectsDuplicateSelectors(t *testing.T) {
	// Rejecting duplicate global selectors prevents ambiguous tool selection.
	_, _, err := extractTool([]string{"--tool", "preset", "--tool=resize"})
	if err == nil || !strings.Contains(err.Error(), "only be specified once") {
		t.Fatalf("error = %v, want duplicate --tool error", err)
	}
}

func TestRunDispatchesToSelectedTool(t *testing.T) {
	// A selected tool should receive its private flags without the global selector.
	stub := &stubTool{name: "stub"}
	oldTools := tools
	tools = []Tool{stub}
	t.Cleanup(func() { tools = oldTools })

	var stdout, stderr bytes.Buffer
	if err := run([]string{"--tool", "stub", "--mode", "fast"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !stub.called {
		t.Fatal("selected tool was not called")
	}
	if !reflect.DeepEqual(stub.args, []string{"--mode", "fast"}) {
		t.Fatalf("tool received args %#v, want private options", stub.args)
	}
}

func TestRunRejectsUnknownToolAndRequiresSelector(t *testing.T) {
	// The CLI should explain both an unsupported tool and an omitted tool selector.
	var stdout, stderr bytes.Buffer
	err := run([]string{"--tool", "missing"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("unknown tool error = %v", err)
	}
	err = run([]string{"--file", "clip.mov"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--module is required") {
		t.Fatalf("missing selector error = %v", err)
	}
}
