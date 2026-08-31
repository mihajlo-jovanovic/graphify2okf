package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Graph is the subset of Graphify's persisted graph used by the converter.
// Edges is accepted as a compatibility alias for Links.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Links []Edge `json:"links"`
	Edges []Edge `json:"edges"`
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
