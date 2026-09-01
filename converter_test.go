package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGraphLinksAndFallbacks(t *testing.T) {
	graph, err := parseGraph(strings.NewReader(`{
		"nodes":[{"id":"one"},{"id":"two","label":"Two","file_type":"function","community":0}],
		"links":[{"source":"one","target":"two","relation":"calls","confidence":"EXTRACTED"}]
	}`))
	if err != nil {
		t.Fatalf("parseGraph() error = %v", err)
	}
	if graph.Nodes[0].Label != "one" || graph.Nodes[0].FileType != "concept" {
		t.Fatalf("fallback node = %#v", graph.Nodes[0])
	}
	if graph.Nodes[1].Community == nil || *graph.Nodes[1].Community != 0 {
		t.Fatalf("community = %v, want pointer to 0", graph.Nodes[1].Community)
	}
	if len(graph.Links) != 1 || graph.Links[0].Relation != "calls" {
		t.Fatalf("links = %#v", graph.Links)
	}
}

func intPointer(value int) *int { return &value }

func TestPlanBundleGroupingPaths(t *testing.T) {
	graph := Graph{Nodes: []Node{
		{ID: "api/user", SourceFile: `src\api\user.go`},
		{ID: "root", SourceFile: "main.go"},
		{ID: "missing"},
		{ID: "absolute", SourceFile: "/tmp/file.go"},
		{ID: "traversal", SourceFile: "src/../secret.go"},
	}}
	var warnings bytes.Buffer
	plan, err := planBundle(graph, filepath.Join(t.TempDir(), "graph.json"), filepath.Join(t.TempDir(), "bundle"), "directory", &warnings)
	if err != nil {
		t.Fatalf("planBundle() error = %v", err)
	}
	want := []string{"_ungrouped/absolute.md", "_ungrouped/missing.md", "_ungrouped/traversal.md", "root.md", "src/api/api_user.md"}
	for index, destination := range want {
		if plan.Nodes[index].Destination != destination {
			t.Errorf("destination[%d] = %q, want %q", index, plan.Nodes[index].Destination, destination)
		}
	}
	if got := strings.Count(warnings.String(), "warning:"); got != 3 {
		t.Errorf("warnings = %q, want 3 warnings", warnings.String())
	}
}

func TestPlanBundleCommunityPaths(t *testing.T) {
	graph := Graph{Nodes: []Node{{ID: "zero", Community: intPointer(0)}, {ID: "none"}, {ID: "negative", Community: intPointer(-2)}}}
	plan, err := planBundle(graph, filepath.Join(t.TempDir(), "graph.json"), filepath.Join(t.TempDir(), "bundle"), "community", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"zero": "community-0/zero.md", "none": "_ungrouped/none.md", "negative": "community--2/negative.md"}
	for _, node := range plan.Nodes {
		if node.Destination != want[node.Node.ID] {
			t.Errorf("destination for %q = %q, want %q", node.Node.ID, node.Destination, want[node.Node.ID])
		}
	}
}

func TestPlanBundleRejectsDestinationConflicts(t *testing.T) {
	tests := []struct {
		name  string
		nodes []Node
		want  string
	}{
		{"sanitized", []Node{{ID: "a/b", SourceFile: "x.go"}, {ID: "a?b", SourceFile: "y.go"}}, "collision"},
		{"case folded", []Node{{ID: "Name", SourceFile: "x.go"}, {ID: "name", SourceFile: "y.go"}}, "collision"},
		{"index reserved", []Node{{ID: "INDEX", SourceFile: "x.go"}}, "reserved"},
		{"log reserved", []Node{{ID: "log", SourceFile: "x.go"}}, "reserved"},
		{"empty filename", []Node{{ID: "...", SourceFile: "x.go"}}, "empty name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := planBundle(Graph{Nodes: test.nodes}, filepath.Join(t.TempDir(), "graph.json"), filepath.Join(t.TempDir(), "bundle"), "directory", nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("planBundle() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestPlanBundleRejectsUnsafeOutputs(t *testing.T) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	tests := []struct{ name, input, output, want string }{
		{"root", filepath.Join(temp, "graph.json"), string(filepath.Separator), "filesystem root"},
		{"current", filepath.Join(temp, "graph.json"), cwd, "current directory"},
		{"contains input", filepath.Join(temp, "bundle", "graph.json"), filepath.Join(temp, "bundle"), "contains input"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := planBundle(Graph{}, test.input, test.output, "directory", nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("planBundle() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestParseGraphEdgesAlias(t *testing.T) {
	graph, err := parseGraph(strings.NewReader(`{
		"nodes":[{"id":"one"},{"id":"two"}],
		"edges":[{"source":"one","target":"two","relation":"uses"}]
	}`))
	if err != nil {
		t.Fatalf("parseGraph() error = %v", err)
	}
	if len(graph.Links) != 1 || len(graph.Edges) != 0 {
		t.Fatalf("links = %#v, edges = %#v", graph.Links, graph.Edges)
	}
}

func TestParseGraphValidation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"missing ID", `{"nodes":[{"label":"no id"}]}`, "empty id"},
		{"blank ID", `{"nodes":[{"id":"  "}]}`, "empty id"},
		{"duplicate ID", `{"nodes":[{"id":"same"},{"id":"same"}]}`, "duplicate node id"},
		{"both edge arrays", `{"nodes":[],"links":[{}],"edges":[{}]}`, "both non-empty"},
		{"invalid JSON", `{`, "decode JSON"},
		{"trailing value", `{} {}`, "multiple JSON values"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseGraph(strings.NewReader(test.input))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseGraph() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRenderConceptsRelationshipsAndWarnings(t *testing.T) {
	graph := Graph{Nodes: []Node{
		{ID: "caller", Label: "Caller", FileType: "function", SourceFile: "src/api/caller.go", Community: intPointer(2)},
		{ID: "target", Label: "Target [value]", FileType: "class", SourceFile: "lib/target.go"},
	}, Links: []Edge{
		{Source: "caller", Target: "target", Relation: "uses", Confidence: "EXTRACTED"},
		{Source: "missing", Target: "target", Relation: "bad"},
		{Source: "caller", Target: "absent", Relation: "bad"},
	}}
	plan, err := planBundle(graph, filepath.Join(t.TempDir(), "graph.json"), filepath.Join(t.TempDir(), "bundle"), "directory", nil)
	if err != nil {
		t.Fatal(err)
	}
	var warnings bytes.Buffer
	files, err := renderConcepts(plan, graph.Links, &warnings)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Destination != "lib/target.md" || files[1].Destination != "src/api/caller.md" {
		t.Fatalf("files = %#v", files)
	}
	target := string(files[0].Content)
	caller := string(files[1].Content)
	if !strings.Contains(caller, `[Target \[value\]](../../lib/target.md)`) {
		t.Errorf("caller relationship missing:\n%s", caller)
	}
	if !strings.Contains(target, `[Caller](../src/api/caller.md)`) {
		t.Errorf("target relationship missing:\n%s", target)
	}
	if got := strings.Count(warnings.String(), "warning:"); got != 2 {
		t.Errorf("warnings = %q, want 2", warnings.String())
	}
}

func TestRenderConceptFrontmatterAndEmptyRelationships(t *testing.T) {
	graph := Graph{Nodes: []Node{{
		ID: "quoted", Label: `A: "quoted" value`, FileType: "concept",
	}}}
	plan, err := planBundle(graph, filepath.Join(t.TempDir(), "graph.json"), filepath.Join(t.TempDir(), "bundle"), "directory", nil)
	if err != nil {
		t.Fatal(err)
	}
	files, err := renderConcepts(plan, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	content := string(files[0].Content)
	checks := []string{
		`type: "concept"`, `title: "A: \"quoted\" value"`,
		`description: "Graphify extracted node for A: \"quoted\" value"`,
		`tags: ["graphify_extracted"]`, "- **Source:** None.", "- **Community:** None.",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("content missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "resource:") {
		t.Errorf("missing resource should be omitted:\n%s", content)
	}
	if got := strings.Count(content, "- None."); got != 2 {
		t.Errorf("empty relationship markers = %d, want 2:\n%s", got, content)
	}
}

func TestRenderConceptsSortsRelationships(t *testing.T) {
	graph := Graph{Nodes: []Node{
		{ID: "root", Label: "Root", FileType: "function", SourceFile: "root.go"},
		{ID: "z", Label: "Zulu", FileType: "function", SourceFile: "z.go"},
		{ID: "a", Label: "Alpha", FileType: "function", SourceFile: "a.go"},
	}, Links: []Edge{
		{Source: "root", Target: "z", Relation: "calls"},
		{Source: "root", Target: "a", Relation: "uses"},
		{Source: "root", Target: "a", Relation: "calls"},
	}}
	plan, err := planBundle(graph, filepath.Join(t.TempDir(), "graph.json"), filepath.Join(t.TempDir(), "bundle"), "directory", nil)
	if err != nil {
		t.Fatal(err)
	}
	files, err := renderConcepts(plan, graph.Links, nil)
	if err != nil {
		t.Fatal(err)
	}
	var content string
	for _, file := range files {
		if file.Destination == "root.md" {
			content = string(file.Content)
		}
	}
	positions := []int{strings.Index(content, "**calls**: [Alpha]"), strings.Index(content, "**uses**: [Alpha]"), strings.Index(content, "**calls**: [Zulu]")}
	if positions[0] < 0 || positions[0] >= positions[1] || positions[1] >= positions[2] {
		t.Errorf("relationships not sorted by label then relation:\n%s", content)
	}
}
