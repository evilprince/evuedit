package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Tool owns its own flags and execution logic. Add future editing features as
// separate Tool implementations instead of adding their options to the global CLI.
type Tool interface {
	Name() string
	Description() string
	Run(args []string, stdout, stderr io.Writer) error
}

var tools = []Tool{
	presetTool{},
	imageFilterTool{},
	thumbTool{},
	motionTool{},
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "evuedit:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	toolName, toolArgs, err := extractTool(args)
	if err != nil {
		printUsage(stderr)
		return err
	}
	if toolName == "" {
		if hasHelpFlag(args) {
			printUsage(stdout)
			return nil
		}
		return errors.New("--tool or --module is required")
	}
	toolName = canonicalToolName(toolName)
	for _, tool := range tools {
		if tool.Name() == toolName {
			return tool.Run(toolArgs, stdout, stderr)
		}
	}
	return fmt.Errorf("unknown tool %q; available tools: %s", toolName, toolNames())
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

// extractTool consumes the global --tool or --module selector. Every other
// argument belongs to the selected tool and is parsed by that tool's FlagSet.
func extractTool(args []string) (string, []string, error) {
	var toolName string
	toolArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			if toolName == "" {
				return "", nil, nil
			}
			toolArgs = append(toolArgs, arg)
		case arg == "--tool" || arg == "--module":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s requires a name", arg)
			}
			if toolName != "" {
				return "", nil, errors.New("--tool and --module may only be specified once")
			}
			i++
			toolName = args[i]
		case strings.HasPrefix(arg, "--tool="):
			if toolName != "" {
				return "", nil, errors.New("--tool and --module may only be specified once")
			}
			toolName = strings.TrimPrefix(arg, "--tool=")
		case strings.HasPrefix(arg, "--module="):
			if toolName != "" {
				return "", nil, errors.New("--tool and --module may only be specified once")
			}
			toolName = strings.TrimPrefix(arg, "--module=")
		default:
			toolArgs = append(toolArgs, arg)
		}
	}
	return toolName, toolArgs, nil
}

func canonicalToolName(name string) string {
	if name == "presets" {
		return "preset"
	}
	return name
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: evuedit --tool <name> [tool options]")
	fmt.Fprintln(w, "       evuedit --module <name> [module options]")
	fmt.Fprintln(w, "\nAvailable tools:")
	for _, tool := range tools {
		fmt.Fprintf(w, "  %-12s %s\n", tool.Name(), tool.Description())
	}
	fmt.Fprintln(w, "\nRun evuedit --tool <name> --help or --module <name> --help for module details.")
}

func toolNames() string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return strings.Join(names, ", ")
}
