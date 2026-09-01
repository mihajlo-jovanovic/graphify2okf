package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

type relationshipView struct {
	Label      string
	Relation   string
	Confidence string
	Link       string
}

type conceptView struct {
	Type, Title, Description, Resource string
	Tags                               []string
	Source, Community                  string
	Outbound, Inbound                  []relationshipView
}

var conceptTemplate = template.Must(template.New("concept").Funcs(template.FuncMap{
	"yaml": yamlValue,
	"md":   markdownText,
}).Parse(`---
type: {{ yaml .Type }}
title: {{ yaml .Title }}
description: {{ yaml .Description }}
{{- if .Resource }}
resource: {{ yaml .Resource }}
{{- end }}
tags: {{ yaml .Tags }}
---

# {{ md .Title }}

## Node Details

- **Source:** {{ if .Source }}{{ md .Source }}{{ else }}None.{{ end }}
- **Community:** {{ md .Community }}

## Outbound Relationships

{{- if .Outbound }}
{{ range .Outbound -}}
- **{{ md .Relation }}**{{ if .Confidence }} ({{ md .Confidence }}){{ end }}: [{{ md .Label }}]({{ .Link }})
{{ end -}}
{{ else }}
- None.
{{ end }}
## Inbound Relationships

{{- if .Inbound }}
{{ range .Inbound -}}
- **{{ md .Relation }}**{{ if .Confidence }} ({{ md .Confidence }}){{ end }}: [{{ md .Label }}]({{ .Link }})
{{ end -}}
{{ else }}
- None.
{{ end }}`))

// renderConcepts resolves valid relationships and renders every concept without
// touching the filesystem. Malformed and dangling edges are skipped.
func renderConcepts(plan BundlePlan, edges []Edge, warnings io.Writer) ([]RenderedFile, error) {
	byID := make(map[string]PlannedNode, len(plan.Nodes))
	outbound := make(map[string][]relationshipView)
	inbound := make(map[string][]relationshipView)
	for _, planned := range plan.Nodes {
		byID[planned.Node.ID] = planned
	}
	for index, edge := range edges {
		source, sourceOK := byID[edge.Source]
		target, targetOK := byID[edge.Target]
		if !sourceOK || !targetOK {
			if warnings != nil {
				fmt.Fprintf(warnings, "warning: skipping edge %d (%q -> %q): source or target node does not exist\n", index, edge.Source, edge.Target)
			}
			continue
		}
		outbound[edge.Source] = append(outbound[edge.Source], relationship(source, target, edge))
		inbound[edge.Target] = append(inbound[edge.Target], relationship(target, source, edge))
	}

	files := make([]RenderedFile, 0, len(plan.Nodes))
	for _, planned := range plan.Nodes {
		sortRelationships(outbound[planned.Node.ID])
		sortRelationships(inbound[planned.Node.ID])
		view := conceptView{
			Type: planned.Node.FileType, Title: planned.Node.Label,
			Description: "Graphify extracted node for " + planned.Node.Label,
			Resource:    planned.Node.SourceFile, Source: planned.Node.SourceFile,
			Community: "None.", Outbound: outbound[planned.Node.ID], Inbound: inbound[planned.Node.ID],
			Tags: []string{"graphify_extracted"},
		}
		if planned.Node.Community != nil {
			community := fmt.Sprintf("community-%d", *planned.Node.Community)
			view.Community = community
			view.Tags = []string{community, "graphify_extracted"}
		}
		var content bytes.Buffer
		if err := conceptTemplate.Execute(&content, view); err != nil {
			return nil, fmt.Errorf("render node %q: %w", planned.Node.ID, err)
		}
		files = append(files, RenderedFile{Destination: planned.Destination, Content: content.Bytes()})
	}
	return files, nil
}

func relationship(from, other PlannedNode, edge Edge) relationshipView {
	link, err := filepath.Rel(filepath.Dir(from.Destination), other.Destination)
	if err != nil { // Destinations were already validated, so this is defensive.
		link = other.Destination
	}
	return relationshipView{Label: other.Node.Label, Relation: edge.Relation,
		Confidence: edge.Confidence, Link: filepath.ToSlash(link)}
}

func sortRelationships(relationships []relationshipView) {
	sort.SliceStable(relationships, func(i, j int) bool {
		left, right := relationships[i], relationships[j]
		if left.Label != right.Label {
			return left.Label < right.Label
		}
		if left.Relation != right.Relation {
			return left.Relation < right.Relation
		}
		if left.Link != right.Link {
			return left.Link < right.Link
		}
		return left.Confidence < right.Confidence
	})
}

func yamlValue(value any) (string, error) {
	encoded, err := json.Marshal(value)
	return string(encoded), err
}

func markdownText(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`, "*", `\*`, "_", `\_`)
	return replacer.Replace(value)
}
