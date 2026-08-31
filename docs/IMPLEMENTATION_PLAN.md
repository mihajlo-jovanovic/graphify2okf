# Graphify to OKF Implementation Plan

This plan targets Graphify `0.9.53`, the current version on its `v8` branch, and Open Knowledge Format (OKF) `0.2`.

References:

- [Graphify package metadata](https://github.com/Graphify-Labs/graphify/blob/v8/pyproject.toml)
- [Graphify graph format](https://github.com/Graphify-Labs/graphify/blob/v8/docs/how-it-works.md)
- [Graphify exporter](https://github.com/Graphify-Labs/graphify/blob/v8/graphify/export.py)
- [Graphify schema validator](https://github.com/Graphify-Labs/graphify/blob/v8/graphify/validate.py)
- [OKF 0.2 specification](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md)

## Required Corrections to `docs/SPEC.md`

Current Graphify differs from the draft input schema:

- Nodes use `file_type`, not `type`.
- Edges use `source`, `target`, and `relation`, not `source_id`, `target_id`, and `relationship`.
- Current output stores edges under `links`; accept `edges` as a compatibility alias.
- Confidence values are `EXTRACTED`, `INFERRED`, or `AMBIGUOUS`.
- Nodes may contain integer or null `community` values.
- Extra fields such as `source_location`, `norm_label`, `community_name`, `confidence_score`, and `weight` can be ignored.
- Hyperedges are outside this prototype's requested scope and will be ignored.

## Minimal Code Layout

Keep everything in one Go package:

```text
main.go              CLI entry point
cmd.go               Cobra root and generate commands
converter.go         JSON parsing, validation, path planning, rendering, writing
templates.go         Node and index text/templates
converter_test.go    Core and integration-style unit tests
cmd_test.go          Small CLI validation test
```

Only Cobra is needed in production. Use `encoding/json` both for Graphify parsing and for safely quoting YAML scalar values, since JSON strings and arrays are valid YAML. This avoids adding a production YAML dependency.

## 1. Implement the CLI

Implement:

```text
graphify2okf generate
  --input,  -i  ./graphify-out/graph.json
  --output, -o  ./okf-bundle
  --group-by, -g directory
```

Behavior:

- Accept only `directory` or `community` for `--group-by`.
- Send warnings to stderr.
- Return a non-zero status for invalid JSON, duplicate IDs, unsafe output configuration, path collisions, rendering errors, or filesystem errors.
- Keep `main.go` limited to constructing and executing the Cobra command.

## 2. Parse the Current Graphify Schema

Define minimal structs:

- `Graph`: `Nodes`, `Links`, and compatibility `Edges`.
- `Node`: `ID`, `Label`, `FileType`, `SourceFile`, `Community`, and optionally `CommunityName`.
- `Edge`: `Source`, `Target`, `Relation`, and `Confidence`.

Parsing rules:

- Require a non-empty node ID.
- Fail on duplicate node IDs.
- If both non-empty `links` and `edges` arrays appear, fail rather than guessing which is authoritative.
- Fall back from an empty label to the node ID.
- Fall back from an empty `file_type` to `concept`, matching current Graphify behavior.
- Treat a missing `source_file` or `community` as ungrouped.
- Skip malformed or dangling edges and emit a warning. This applies to missing sources and targets.
- Use source/target direction exactly as persisted by Graphify.

## 3. Compute Destinations Before Deleting Output

Preflight generation fully before destructive cleanup:

1. Resolve input and output to absolute paths.
2. Refuse a filesystem root, the repository root/current directory, or any output directory containing the input file.
3. Build the node ID index.
4. Compute every node destination.
5. Detect duplicate IDs, sanitized filename collisions, case-insensitive path collisions, and conflicts with reserved `index.md` or `log.md` filenames.
6. Resolve relationships and render every output file into memory.
7. Only after successful preflight, remove and recreate the dedicated output directory.

This preserves an existing bundle when the new input is invalid while still satisfying complete-cleanup idempotency.

## 4. Generate Safe Paths

### Directory grouping

- Convert `\` to `/`.
- Map `src/api/user.go` to `src/api/<sanitized-id>.md`.
- Place nodes from root-level sources such as `main.go` at the bundle root.
- Sanitize each directory component as well as the filename.
- If `source_file` is missing, absolute, contains traversal such as `..`, or otherwise cannot safely remain inside the bundle, place the node under `_ungrouped/` and warn.

This avoids inventing a project root for absolute paths while ensuring no generated path escapes the output directory.

### Community grouping

- Map community `3` to `community-3/`.
- Map a missing or null community to `_ungrouped/`.
- Use the numeric ID for stable folder names; optional `community_name` data remains display metadata only.

### Filename sanitization

- Replace characters outside `A-Z`, `a-z`, `0-9`, `.`, `_`, and `-` with `_`.
- Trim unsafe leading and trailing dots or spaces.
- Fail if sanitization produces an empty name.
- Fail on any destination collision rather than suffixing or overwriting.

## 5. Build Relationship Views

For each valid edge:

- Add it to the source node's outbound list.
- Add it to the target node's inbound list.
- Resolve links using `filepath.Rel` from the current Markdown file's directory.
- Convert path separators to `/`.
- Sort relationships deterministically by label, relation, and destination.

Although OKF recommends bundle-absolute links, it supports relative links and the local specification requires them.

## 6. Render OKF 0.2 Output

Each concept receives valid frontmatter:

```yaml
---
type: "code"
title: "Example"
description: "Graphify extracted node for Example"
resource: "src/example.go"
tags: ["community-3", "graphify_extracted"]
---
```

Rules:

- Use `file_type` as the OKF `type`.
- Omit `resource` when `source_file` is missing.
- Omit the community tag when community is missing.
- Omit `timestamp` and `generated` for this prototype.
- Escape YAML values and Markdown labels safely.
- Render Node Details, Outbound Relationships, and Inbound Relationships.
- Render `- None.` for empty relationship sections.

## 7. Generate Progressive Indexes

Generate `index.md` in the bundle root and every generated subdirectory.

- List only concepts and immediate child directories.
- Include concept descriptions.
- Use relative links.
- Sort entries deterministically.
- Put `okf_version: "0.2"` frontmatter only in the root index.
- Do not generate `log.md`.

This follows OKF's one-level progressive-disclosure index model.

## 8. Delivery Plan: Medium-Sized PRs

The implementation should be delivered as four sequential PRs. Each PR must stay below 400 changed lines of production Go code; test code, `go.sum`, and documentation are excluded from that limit. The line counts below are estimates and should be checked with the PR diff before submission.

### PR 1: CLI Skeleton and Graphify Input Parsing

Estimated production code: 180-250 lines.

Scope:

- Add Cobra and the `graphify2okf generate` command with the specified flags and defaults.
- Add the minimal Graphify `Graph`, `Node`, and `Edge` structs.
- Parse current `links` input and the `edges` compatibility alias.
- Validate `--group-by`, required node IDs, duplicate IDs, and ambiguous input containing both edge arrays.
- Apply the documented label and `file_type` fallbacks.
- Route command errors and warnings through injectable stdout/stderr writers.

Tests:

- CLI defaults and invalid `--group-by`.
- Parsing `links` and compatibility `edges`.
- Duplicate/missing node IDs and simultaneous edge arrays.

Exit criteria:

- The command parses and validates a graph but does not write an OKF bundle yet.
- `go test ./...` and `go vet ./...` pass.

### PR 2: Safe Output Path Planning

Estimated production code: 180-280 lines.

Scope:

- Implement node filename and directory-component sanitization.
- Implement `directory` and `community` grouping.
- Route missing or unsafe source paths and missing communities to `_ungrouped/`.
- Add input/output safety checks before any cleanup.
- Detect sanitized, case-insensitive, and reserved-filename collisions.
- Build a deterministic in-memory plan of node destinations without writing files.

Tests:

- Directory, root-file, community, and ungrouped paths.
- Windows separators, absolute paths, traversal, and empty sanitized names.
- Duplicate destination, case-folded, and `index.md`/`log.md` conflicts.
- Refusal to use unsafe output locations or an output containing the input.

Exit criteria:

- Every valid node has one safe bundle-relative destination.
- Invalid plans fail before altering the output directory.
- `go test ./...` and `go vet ./...` pass.

### PR 3: OKF Concept Rendering and Relationships

Estimated production code: 220-320 lines.

Scope:

- Index nodes by ID and classify edges as outbound and inbound.
- Skip malformed or dangling source/target edges with warnings.
- Compute deterministic relative Markdown links.
- Add YAML-safe scalar/list helpers using `encoding/json`.
- Add the Go template for OKF concept frontmatter and Markdown content.
- Render all concept files into memory without touching the output directory.

Tests:

- Cross-directory inbound and outbound links.
- Dangling sources and targets, including warning output.
- Empty relationship sections.
- Frontmatter quoting and fallbacks for missing optional node fields.
- Deterministic relationship ordering.

Exit criteria:

- The converter can produce complete, deterministic concept-file bytes in memory.
- Render failures do not modify an existing bundle.
- `go test ./...` and `go vet ./...` pass.

### PR 4: Index Generation, Filesystem Commit, and End-to-End Verification

Estimated production code: 180-280 lines.

Scope:

- Generate the root and per-directory progressive `index.md` files.
- Add `okf_version: "0.2"` only to the root index.
- Sort immediate concept and child-directory entries deterministically.
- After successful preflight/rendering, clear and recreate the dedicated output directory.
- Write all planned files and connect the completed converter to the Cobra command.
- Add concise user-facing generation and warning output.

Tests:

- Root and nested index structure, descriptions, links, and sorting.
- End-to-end generation for both grouping modes.
- Regeneration removes orphaned files.
- Repeated runs produce byte-identical bundles.
- A preflight failure preserves the existing output directory.

Exit criteria:

- The CLI produces a conformant OKF 0.2 prototype bundle from a current Graphify graph.
- Cleanup happens only after all input, paths, edges, and templates pass preflight.
- `go test ./...` and `go vet ./...` pass.

### PR Size Guard

Before opening each PR:

```text
git diff --numstat <base>...HEAD -- '*.go'
```

Count production files separately from `*_test.go`. If production changes approach 400 lines, reduce that PR's scope instead of moving tests to a later PR.

## 9. Moderate Test-Suite Boundaries

Use approximately six focused tests, with table-driven subtests where useful:

1. Parse a compact Graphify `0.9.53`-shaped graph using `links`, plus the `edges` compatibility alias.
2. Test filename and grouping paths, including Windows separators, root files, traversal, absolute paths, and missing community.
3. Test duplicate IDs, sanitized collisions, case collisions, and reserved filenames.
4. Run an end-to-end conversion with three nodes across directories, verifying frontmatter, inbound/outbound links, the root index, and nested indexes.
5. Verify dangling source and target edges are skipped with warnings.
6. Verify regeneration removes an orphaned file and produces byte-identical output on repeated runs.
7. Add one small CLI test, either separately or as part of the sixth setup, to verify invalid `--group-by` handling.

Use `t.TempDir()` and targeted assertions instead of large golden files.

## Verification

Run:

```text
go test ./...
go vet ./...
```

Also run the CLI against a compact Graphify-shaped fixture and inspect the generated directory tree and Markdown links.
