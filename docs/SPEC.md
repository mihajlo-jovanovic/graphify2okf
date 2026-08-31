# Graphify to OKF (Open Knowledge Format) Converter

**Objective**: Build a CLI tool that parses Graphify's graph.json output and generates an Open Knowledge Format (OKF) compliant directory of Markdown files.

## Tech Stack
  - Language: Go
  - CLI Framework: Cobra
  - Templating: Go text/template
  - Data parsing: encoding/json
 
1. Input Specification (graph.json)
  The CLI will ingest a Graphify JSON graph. The agent must define Go structs to unmarshal this schema:

  - Nodes: Contain *id*, *label*, *source_file*, *community*, *type* (e.g., function, class).
  - Edges: Contain *source_id*, *target_id*, *relationship* (e.g., uses, imports), *confidence* (EXTRACTED, INFERRED).

2. Output Specification (OKF Bundle)

    The tool must output a directory containing Markdown files with YAML frontmatter.

  - **File Naming**: Each node becomes a file named *<sanitized_node_id>.md* .

  - **Directory Structure**: Group files by their *source_file* directory path or *community* (configurable via CLI flag). Generate an *index.md* in each subdirectory listing its contents.

  - **Frontmatter**: Must be valid YAML fenced by ---. Strictly requires a *type* field.

  - **Markdown Body**: Must contain human-readable Node details and a section for Edges formatted as relative Markdown links.

3. Core Requirements & Logic
    
    A. CLI Commands (Cobra)
    
    Implement a root command *graphify2okf* with a *generate* subcommand:

    --input, -i: Path to *graph.json* (default: *./graphify-out/graph.json*).

    --output, -o: Path to output directory (default: *./okf-bundle*).

    --group-by, -g: Grouping strategy for folders: *directory* or *community* (default: *directory*).

  B. Graph Processing & Mapping
    
  1. Parse & Index: Unmarshal the JSON. Create an in-memory map of *NodeID -> Node* to quickly resolve target names and file paths when generating edge links.
  1. Path Generation: Compute the relative output path for every node based on the --group-by flag. (e.g., Node_A in src/api/ goes to okf-bundle/src/api/Node_A.md).
  1. Link Resolution: When processing edges for a node, look up the target node's destination path and compute the relative Markdown link from the source file to the target file.

  C. File Generation (Go Templates)
  Use text/template to generate the Markdown files. The template should look like this:

```Markdown

type: {{ .NodeType }}
title: {{ .Label }}
description: Graphify extracted node for {{ .Label }}
resource: {{ .SourceFile }}
tags: [{{ .Community }}, graphify_extracted]
timestamp: {{ .Timestamp }}

---

# {{ .Label }}

**Source:** `{{ .SourceFile }}`
**Community:** {{ .Community }}

## Relationships

### Outbound (Depends On)

{{ range .OutboundEdges }}

- **{{ .Relationship }}** ({{ .Confidence }}): [{{ .TargetLabel }}]({{ .RelativeLink }})
  {{ end }}

### Inbound (Referenced By)

{{ range .InboundEdges }}

- **{{ .Relationship }}** ({{ .Confidence }}): [{{ .SourceLabel }}]({{ .RelativeLink }})
  {{ end }}
```

4. Edge Cases & Error Handling

- Sanitization: Node IDs and labels might contain invalid filename characters (e.g., < > / \). Implement a string sanitizer for generating safe .md filenames.

- Missing Fields: Graphify output might have nodes with null or missing fields depending on the parser. Fallback gracefully (e.g., default type to "Unknown").

- Dangling Edges: If an edge references a target ID that doesn't exist in the nodes list, skip it and log a warning.

- Idempotency: Ensure the output directory is completely cleared before generating a new bundle to prevent orphaned files from previous runs.
