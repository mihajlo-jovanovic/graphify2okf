package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Graph is the subset of Graphify's persisted graph used by the converter.
// Edges is accepted as a compatibility alias for Links.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Links []Edge `json:"links"`
	Edges []Edge `json:"edges"`
}

// PlannedNode records the bundle-relative Markdown destination for a node.
type PlannedNode struct {
	Node        Node
	Destination string
}

// BundlePlan is a fully validated, deterministic plan for a future generation.
// Planning never creates, removes, or modifies files.
type BundlePlan struct {
	InputPath  string
	OutputPath string
	Nodes      []PlannedNode
}

// RenderedFile is a bundle-relative concept file rendered entirely in memory.
type RenderedFile struct {
	Destination string
	Content     []byte
}

// renderBundle completes all rendering before any filesystem changes occur.
func renderBundle(plan BundlePlan, edges []Edge, warnings io.Writer) ([]RenderedFile, error) {
	concepts, err := renderConcepts(plan, edges, warnings)
	if err != nil {
		return nil, err
	}
	indexes, err := renderIndexes(plan)
	if err != nil {
		return nil, err
	}
	files := append(concepts, indexes...)
	sort.Slice(files, func(i, j int) bool { return files[i].Destination < files[j].Destination })
	return files, nil
}

// writeBundle replaces the output only after the complete bundle has passed
// parsing, path planning, relationship resolution, and rendering.
func writeBundle(plan BundlePlan, files []RenderedFile) error {
	if err := os.RemoveAll(plan.OutputPath); err != nil {
		return fmt.Errorf("clear output directory: %w", err)
	}
	if err := os.MkdirAll(plan.OutputPath, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	for _, file := range files {
		destination := filepath.Join(plan.OutputPath, filepath.FromSlash(file.Destination))
		if !pathContains(plan.OutputPath, destination) {
			return fmt.Errorf("unsafe rendered destination %q", file.Destination)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("create directory for %q: %w", file.Destination, err)
		}
		if err := os.WriteFile(destination, file.Content, 0o644); err != nil {
			return fmt.Errorf("write %q: %w", file.Destination, err)
		}
	}
	return nil
}

// planBundle validates the output configuration and computes all node paths.
func planBundle(graph Graph, input, output, groupBy string, warnings io.Writer) (BundlePlan, error) {
	inputPath, err := filepath.Abs(input)
	if err != nil {
		return BundlePlan{}, fmt.Errorf("resolve input path: %w", err)
	}
	outputPath, err := filepath.Abs(output)
	if err != nil {
		return BundlePlan{}, fmt.Errorf("resolve output path: %w", err)
	}
	inputPath = filepath.Clean(inputPath)
	outputPath = filepath.Clean(outputPath)
	if err := validateOutputPath(inputPath, outputPath); err != nil {
		return BundlePlan{}, err
	}

	plan := BundlePlan{InputPath: inputPath, OutputPath: outputPath}
	seen := make(map[string]string, len(graph.Nodes))
	for _, node := range graph.Nodes {
		name, err := sanitizePathComponent(node.ID)
		if err != nil {
			return BundlePlan{}, fmt.Errorf("node %q filename: %w", node.ID, err)
		}
		directory, unsafeSource, err := groupingDirectory(node, groupBy)
		if err != nil {
			return BundlePlan{}, fmt.Errorf("node %q: %w", node.ID, err)
		}
		if unsafeSource && warnings != nil {
			fmt.Fprintf(warnings, "warning: node %q has missing or unsafe source_file; using _ungrouped\n", node.ID)
		}
		destination := filepath.Join(directory, name+".md")
		key := strings.ToLower(filepath.ToSlash(destination))
		if previous, exists := seen[key]; exists {
			return BundlePlan{}, fmt.Errorf("destination collision between nodes %q and %q at %q", previous, node.ID, filepath.ToSlash(destination))
		}
		base := strings.ToLower(filepath.Base(destination))
		if base == "index.md" || base == "log.md" {
			return BundlePlan{}, fmt.Errorf("node %q destination %q uses reserved filename", node.ID, filepath.ToSlash(destination))
		}
		seen[key] = node.ID
		plan.Nodes = append(plan.Nodes, PlannedNode{Node: node, Destination: filepath.ToSlash(destination)})
	}
	sort.Slice(plan.Nodes, func(i, j int) bool {
		if plan.Nodes[i].Destination == plan.Nodes[j].Destination {
			return plan.Nodes[i].Node.ID < plan.Nodes[j].Node.ID
		}
		return plan.Nodes[i].Destination < plan.Nodes[j].Destination
	})
	return plan, nil
}

func validateOutputPath(inputPath, outputPath string) error {
	volume := filepath.VolumeName(outputPath)
	if outputPath == filepath.Clean(volume+string(filepath.Separator)) {
		return fmt.Errorf("unsafe output path %q: filesystem root is not allowed", outputPath)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve current directory: %w", err)
	}
	cwd, _ = filepath.Abs(cwd)
	if samePath(outputPath, cwd) {
		return fmt.Errorf("unsafe output path %q: current directory is not allowed", outputPath)
	}
	if root := repositoryRoot(cwd); root != "" && samePath(outputPath, root) {
		return fmt.Errorf("unsafe output path %q: repository root is not allowed", outputPath)
	}
	if pathContains(outputPath, inputPath) {
		return fmt.Errorf("unsafe output path %q contains input file %q", outputPath, inputPath)
	}
	return nil
}

func repositoryRoot(start string) string {
	for directory := filepath.Clean(start); ; directory = filepath.Dir(directory) {
		if info, err := os.Stat(filepath.Join(directory, ".git")); err == nil && info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return ""
		}
	}
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func groupingDirectory(node Node, groupBy string) (string, bool, error) {
	switch groupBy {
	case "community":
		if node.Community == nil {
			return "_ungrouped", false, nil
		}
		return fmt.Sprintf("community-%d", *node.Community), false, nil
	case "directory":
		return sourceDirectory(node.SourceFile)
	default:
		return "", false, fmt.Errorf("invalid grouping strategy %q", groupBy)
	}
}

func sourceDirectory(source string) (string, bool, error) {
	if source == "" {
		return "_ungrouped", true, nil
	}
	normalized := strings.ReplaceAll(source, `\`, "/")
	// filepath.IsAbs does not recognize Windows paths on Unix.
	if strings.HasPrefix(normalized, "/") || (len(normalized) >= 2 && normalized[1] == ':') {
		return "_ungrouped", true, nil
	}
	parts := strings.Split(normalized, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "_ungrouped", true, nil
		}
	}
	if len(parts) == 1 {
		return "", false, nil
	}
	directories := make([]string, 0, len(parts)-1)
	for _, part := range parts[:len(parts)-1] {
		clean, err := sanitizePathComponent(part)
		if err != nil {
			return "_ungrouped", true, nil
		}
		directories = append(directories, clean)
	}
	return filepath.Join(directories...), false, nil
}

func sanitizePathComponent(value string) (string, error) {
	value = strings.Trim(value, ". ")
	var builder strings.Builder
	for _, character := range value {
		if unicode.IsLetter(character) && character <= unicode.MaxASCII ||
			unicode.IsDigit(character) && character <= unicode.MaxASCII ||
			character == '.' || character == '_' || character == '-' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), ". ")
	if result == "" {
		return "", fmt.Errorf("sanitization produced an empty name")
	}
	return result, nil
}

type Node struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	FileType      string `json:"file_type"`
	SourceFile    string `json:"source_file"`
	Community     *int   `json:"community"`
	CommunityName string `json:"community_name,omitempty"`
}

type Edge struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	Relation   string `json:"relation"`
	Confidence string `json:"confidence"`
}

func parseGraph(reader io.Reader) (Graph, error) {
	var graph Graph
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&graph); err != nil {
		return Graph{}, fmt.Errorf("decode JSON: %w", err)
	}
	// Reject trailing JSON values instead of silently accepting a malformed file.
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Graph{}, fmt.Errorf("decode JSON: multiple JSON values")
		}
		return Graph{}, fmt.Errorf("decode JSON: %w", err)
	}

	if len(graph.Links) > 0 && len(graph.Edges) > 0 {
		return Graph{}, fmt.Errorf("input contains both non-empty links and edges arrays")
	}
	if len(graph.Links) == 0 && len(graph.Edges) > 0 {
		graph.Links = graph.Edges
	}
	graph.Edges = nil

	ids := make(map[string]struct{}, len(graph.Nodes))
	for index := range graph.Nodes {
		node := &graph.Nodes[index]
		if strings.TrimSpace(node.ID) == "" {
			return Graph{}, fmt.Errorf("node at index %d has an empty id", index)
		}
		if _, exists := ids[node.ID]; exists {
			return Graph{}, fmt.Errorf("duplicate node id %q", node.ID)
		}
		ids[node.ID] = struct{}{}
		if node.Label == "" {
			node.Label = node.ID
		}
		if node.FileType == "" {
			node.FileType = "concept"
		}
	}

	return graph, nil
}
