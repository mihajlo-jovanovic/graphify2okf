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

type indexEntry struct {
	Label, Description, Link string
}

type indexView struct {
	Root                  bool
	Concepts, Directories []indexEntry
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

var indexTemplate = template.Must(template.New("index").Funcs(template.FuncMap{
	"yaml": yamlValue,
	"md":   markdownText,
}).Parse(`{{ if .Root -}}
---
okf_version: {{ yaml "0.2" }}
---

{{ end -}}
# Knowledge Index

## Concepts

{{- if .Concepts }}
{{ range .Concepts -}}
- [{{ md .Label }}]({{ .Link }}) — {{ md .Description }}
{{ end -}}
{{ else }}
- None.
{{ end }}
## Directories

{{- if .Directories }}
{{ range .Directories -}}
- [{{ md .Label }}]({{ .Link }})
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

// renderIndexes builds a progressive index for the root and each directory
// containing concepts or other generated directories.
func renderIndexes(plan BundlePlan) ([]RenderedFile, error) {
	directories := map[string]struct{}{"": {}}
	concepts := make(map[string][]indexEntry)
	children := make(map[string]map[string]struct{})
	for _, planned := range plan.Nodes {
		directory := filepath.ToSlash(filepath.Dir(planned.Destination))
		if directory == "." {
			directory = ""
		}
		concepts[directory] = append(concepts[directory], indexEntry{
			Label: planned.Node.Label, Description: "Graphify extracted node for " + planned.Node.Label,
			Link: filepath.Base(planned.Destination),
		})
		registerDirectory(directory, directories, children)
	}
	paths := make([]string, 0, len(directories))
	for directory := range directories {
		paths = append(paths, directory)
	}
	sort.Strings(paths)
	files := make([]RenderedFile, 0, len(paths))
	for _, directory := range paths {
		sort.Slice(concepts[directory], func(i, j int) bool {
			left, right := concepts[directory][i], concepts[directory][j]
			if left.Label != right.Label {
				return left.Label < right.Label
			}
			return left.Link < right.Link
		})
		var childEntries []indexEntry
		for child := range children[directory] {
			childEntries = append(childEntries, indexEntry{Label: child, Link: child + "/index.md"})
		}
		sort.Slice(childEntries, func(i, j int) bool { return childEntries[i].Label < childEntries[j].Label })
		var content bytes.Buffer
		view := indexView{Root: directory == "", Concepts: concepts[directory], Directories: childEntries}
		if err := indexTemplate.Execute(&content, view); err != nil {
			return nil, fmt.Errorf("render index %q: %w", directory, err)
		}
		files = append(files, RenderedFile{Destination: filepath.ToSlash(filepath.Join(directory, "index.md")), Content: content.Bytes()})
	}
	return files, nil
}

func registerDirectory(directory string, directories map[string]struct{}, children map[string]map[string]struct{}) {
	if directory == "" {
		return
	}
	parts := strings.Split(directory, "/")
	parent := ""
	for _, part := range parts {
		if children[parent] == nil {
			children[parent] = make(map[string]struct{})
		}
		children[parent][part] = struct{}{}
		parent = filepath.ToSlash(filepath.Join(parent, part))
		directories[parent] = struct{}{}
	}
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
