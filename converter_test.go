package main

import (
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
