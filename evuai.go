package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

//go:embed tools/*/tools.json
var plannerSchemas embed.FS

const moduleSelectionInstructions = `Select the single evuedit module that best matches the user's request. Use the select_evuedit_module function and briefly explain the match. Choose "none" if no module can perform the request. Do not create CLI arguments in this step.`
const argumentPlanningInstructions = `You are the evuedit CLI planner. Return arguments for the selected module that satisfy its function schema. Do not invent options or execute commands. If needed information is missing, explain that in plain text.`

type plannerModule struct {
	Name        string
	Description string
}

var plannerModules = []plannerModule{
	{Name: "preset", Description: "Apply a built-in color look to portrait videos, one file or a folder of videos."},
	{Name: "evufilters", Description: "Apply a named filter to still images, optionally resizing or converting the output; supports one image or a folder."},
	{Name: "evuthumb", Description: "Create YouTube, Instagram, or Facebook thumbnails from an image and a headline using a layout template."},
	{Name: "evumotion", Description: "Create a short animated portrait video from a still image, headline, and motion template."},
	{Name: "evuads", Description: "Generate image assets for Google Ads campaign types, with optional logo assets."},
}

type plannerTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict"`
}

type plannerResponse struct {
	Output     []plannerOutput `json:"output"`
	OutputText string          `json:"output_text"`
}

type plannerOutput struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type plannerRequest struct {
	Model             string        `json:"model"`
	Instructions      string        `json:"instructions"`
	Input             string        `json:"input"`
	Tools             []plannerTool `json:"tools"`
	ToolChoice        string        `json:"tool_choice"`
	ParallelToolCalls bool          `json:"parallel_tool_calls"`
}

func runEvuai(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("evuai", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: evuai [options] <natural-language query>")
		fmt.Fprintln(stderr, "       evuai --query <natural-language query>")
		fmt.Fprintln(stderr, "Options:")
		fs.PrintDefaults()
	}
	query := fs.String("query", "", "Natural-language editing request (or pass it as a positional argument)")
	modules := fs.String("module", "", "Comma-separated modules to consider (default: all modules)")
	model := fs.String("model", "", "OpenAI model (default: OPENAI_MODEL or gpt-4.1-mini)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *query == "" && fs.NArg() > 0 {
		*query = strings.Join(fs.Args(), " ")
	} else if fs.NArg() > 0 {
		return errors.New("provide the query either positionally or with --query, not both")
	}
	if strings.TrimSpace(*query) == "" {
		return errors.New("a natural-language query is required")
	}
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return errors.New("OPENAI_API_KEY is required")
	}
	availableModules, err := selectPlannerModules(*modules)
	if err != nil {
		return err
	}
	chosenModel := strings.TrimSpace(*model)
	if chosenModel == "" {
		chosenModel = strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	}
	if chosenModel == "" {
		chosenModel = "gpt-4.1-mini"
	}

	var moduleList []string
	var moduleDescriptions strings.Builder
	for _, module := range availableModules {
		moduleList = append(moduleList, module.Name)
		fmt.Fprintf(&moduleDescriptions, "- %s: %s\n", module.Name, module.Description)
	}
	selectionInput := "Available evuedit modules:\n" + moduleDescriptions.String() + "\nUser request:\n" + *query
	selectionResponse, err := callPlannerAPI(apiKey, plannerRequest{
		Model: chosenModel, Instructions: moduleSelectionInstructions, Input: selectionInput,
		Tools: []plannerTool{moduleSelectorTool(moduleList)}, ToolChoice: "auto", ParallelToolCalls: false,
	})
	if err != nil {
		return fmt.Errorf("select module: %w", err)
	}
	selection, ok, err := selectedModule(selectionResponse, availableModules)
	if err != nil {
		return err
	}
	if !ok {
		message := responseText(selectionResponse)
		if message == "" {
			message = "No module matched the request."
		}
		fmt.Fprintln(stdout, message)
		return nil
	}
	if selection.Module == "none" {
		if selection.Reason == "" {
			selection.Reason = "No available module fits this request."
		}
		fmt.Fprintln(stdout, selection.Reason)
		return nil
	}

	selectedTools, err := loadPlannerTools(selection.Module)
	if err != nil {
		return err
	}
	argumentResponse, err := callPlannerAPI(apiKey, plannerRequest{
		Model: chosenModel, Instructions: argumentPlanningInstructions, Input: *query,
		Tools: selectedTools, ToolChoice: "auto", ParallelToolCalls: false,
	})
	if err != nil {
		return fmt.Errorf("plan %s arguments: %w", selection.Module, err)
	}
	fmt.Fprintf(stdout, "Selected module: %s — %s\n", selection.Module, selection.Reason)
	if err := printPlannerResult(stdout, argumentResponse); err != nil {
		return err
	}
	return nil
}

type moduleSelection struct {
	Module string `json:"module"`
	Reason string `json:"reason"`
}

func selectPlannerModules(moduleList string) ([]plannerModule, error) {
	wanted := map[string]bool{}
	if strings.TrimSpace(moduleList) != "" {
		for _, name := range strings.Split(moduleList, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil, errors.New("--module contains an empty module name")
			}
			wanted[name] = true
		}
	}
	var selected []plannerModule
	for _, module := range plannerModules {
		if len(wanted) == 0 || wanted[module.Name] {
			selected = append(selected, module)
			delete(wanted, module.Name)
		}
	}
	for name := range wanted {
		return nil, fmt.Errorf("unknown module %q", name)
	}
	if len(selected) == 0 {
		return nil, errors.New("no modules are available")
	}
	return selected, nil
}

func moduleSelectorTool(moduleNames []string) plannerTool {
	enums := append(append([]string(nil), moduleNames...), "none")
	parameters, _ := json.Marshal(struct {
		Type       string `json:"type"`
		Properties struct {
			Module struct {
				Type string   `json:"type"`
				Enum []string `json:"enum"`
			} `json:"module"`
			Reason struct {
				Type string `json:"type"`
			} `json:"reason"`
		} `json:"properties"`
		Required             []string `json:"required"`
		AdditionalProperties bool     `json:"additionalProperties"`
	}{
		Type: "object",
		Properties: struct {
			Module struct {
				Type string   `json:"type"`
				Enum []string `json:"enum"`
			} `json:"module"`
			Reason struct {
				Type string `json:"type"`
			} `json:"reason"`
		}{
			Module: struct {
				Type string   `json:"type"`
				Enum []string `json:"enum"`
			}{Type: "string", Enum: enums},
			Reason: struct {
				Type string `json:"type"`
			}{Type: "string"},
		},
		Required:             []string{"module", "reason"},
		AdditionalProperties: false,
	})
	return plannerTool{
		Type: "function", Name: "select_evuedit_module",
		Description: "Select the best matching evuedit module from the available module list, or none if no module fits.",
		Parameters:  parameters, Strict: true,
	}
}

func selectedModule(response plannerResponse, available []plannerModule) (moduleSelection, bool, error) {
	for _, item := range response.Output {
		if item.Type != "function_call" || item.Name != "select_evuedit_module" {
			continue
		}
		var selection moduleSelection
		if err := json.Unmarshal([]byte(item.Arguments), &selection); err != nil {
			return moduleSelection{}, false, fmt.Errorf("decode module selection: %w", err)
		}
		if selection.Module != "none" {
			valid := false
			for _, module := range available {
				if module.Name == selection.Module {
					valid = true
					break
				}
			}
			if !valid {
				return moduleSelection{}, false, fmt.Errorf("model selected invalid module %q", selection.Module)
			}
		}
		return selection, true, nil
	}
	return moduleSelection{}, false, nil
}

func printPlannerResult(w io.Writer, result plannerResponse) error {
	printed := false
	for _, item := range result.Output {
		if item.Type != "function_call" {
			continue
		}
		if err := printSuggestion(w, item); err != nil {
			return err
		}
		printed = true
	}
	if !printed {
		message := responseText(result)
		if message == "" {
			message = "The selected module could not prepare a command from this request."
		}
		fmt.Fprintln(w, message)
	}
	return nil
}

func responseText(result plannerResponse) string {
	message := strings.TrimSpace(result.OutputText)
	if message != "" {
		return message
	}
	var parts []string
	for _, item := range result.Output {
		for _, content := range item.Content {
			if content.Type == "output_text" || content.Type == "text" {
				parts = append(parts, content.Text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func callPlannerAPI(apiKey string, request plannerRequest) (plannerResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return plannerResponse{}, fmt.Errorf("encode request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(body))
	if err != nil {
		return plannerResponse{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return plannerResponse{}, fmt.Errorf("call OpenAI Responses API: %w", err)
	}
	defer resp.Body.Close()
	var responseBody bytes.Buffer
	if _, err := io.Copy(&responseBody, io.LimitReader(resp.Body, 8<<20)); err != nil {
		return plannerResponse{}, fmt.Errorf("read API response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return plannerResponse{}, fmt.Errorf("OpenAI API returned %s: %s", resp.Status, strings.TrimSpace(responseBody.String()))
	}
	var result plannerResponse
	if err := json.Unmarshal(responseBody.Bytes(), &result); err != nil {
		return plannerResponse{}, fmt.Errorf("decode API response: %w", err)
	}
	return result, nil
}

func loadPlannerTools(moduleList string) ([]plannerTool, error) {
	entries, err := plannerSchemas.ReadDir("tools")
	if err != nil {
		return nil, fmt.Errorf("list embedded module schemas: %w", err)
	}
	wanted := map[string]bool{}
	if strings.TrimSpace(moduleList) != "" {
		for _, name := range strings.Split(moduleList, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil, errors.New("--module contains an empty module name")
			}
			wanted[name] = true
		}
	}
	var tools []plannerTool
	found := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		module := entry.Name()
		if len(wanted) > 0 && !wanted[module] {
			continue
		}
		data, err := plannerSchemas.ReadFile(path.Join("tools", module, "tools.json"))
		if err != nil {
			return nil, fmt.Errorf("read %s schema: %w", module, err)
		}
		var definitions []plannerTool
		if err := json.Unmarshal(data, &definitions); err != nil {
			return nil, fmt.Errorf("parse %s schema: %w", module, err)
		}
		for _, definition := range definitions {
			tools = append(tools, definition)
		}
		found[module] = true
	}
	for module := range wanted {
		if !found[module] {
			return nil, fmt.Errorf("unknown module %q", module)
		}
	}
	if len(tools) == 0 {
		return nil, errors.New("no module schemas are available")
	}
	return tools, nil
}

func printSuggestion(w io.Writer, call plannerOutput) error {
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return fmt.Errorf("decode arguments for %s: %w", call.Name, err)
	}
	module := ""
	for _, candidate := range []string{"preset", "evufilters", "evuthumb", "evumotion", "evuads"} {
		if strings.Contains(call.Name, map[string]string{"preset": "preset", "evufilters": "image_filters", "evuthumb": "thumbnail", "evumotion": "motion_video", "evuads": "google_ads"}[candidate]) {
			module = candidate
			break
		}
	}
	if module == "" {
		return fmt.Errorf("API returned an unknown module tool %q", call.Name)
	}
	pretty, err := json.MarshalIndent(args, "", "  ")
	if err != nil {
		return fmt.Errorf("format tool arguments: %w", err)
	}
	fmt.Fprintf(w, "Suggested module: %s\nArguments:\n%s\nSuggested command:\n", module, pretty)
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	command := []string{"evuedit", "--module", module}
	for _, key := range keys {
		value := args[key]
		if value == nil {
			continue
		}
		flag := "--" + strings.ReplaceAll(key, "_", "-")
		switch typed := value.(type) {
		case bool:
			command = append(command, flag+"="+fmt.Sprint(typed))
		case string:
			command = append(command, flag, shellQuote(typed))
		default:
			encoded, err := json.Marshal(typed)
			if err != nil {
				return fmt.Errorf("format %s argument: %w", key, err)
			}
			command = append(command, flag, shellQuote(string(encoded)))
		}
	}
	fmt.Fprintln(w, strings.Join(command, " "))
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
