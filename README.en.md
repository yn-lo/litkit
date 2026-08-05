# litkit

An academic writing toolkit for Chinese researchers: cross-source paper search, standards-compliant citations, manuscript typesetting, and AI-writing compliance gates.

[**English**](README.en.md) | [中文](README.md)

## Table of Contents

- [Overview](#overview)
- [Project Principles](#project-principles)
- [Features](#features)
- [Source Strategy](#source-strategy)
- [Source Capability Matrix](#source-capability-matrix)
- [Configuration](#configuration)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [AI Integration](#ai-integration)
- [Documentation](#documentation)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## Overview

litkit is a Go-based paper toolkit (Go 1.26 / cobra / MCP SDK / SQLite) with a **CLI-first** interface and an optional **MCP Server** interface. Designed for AI agents and command-line users, it covers the full academic writing pipeline:

- **Cross-source search**: concurrent search + dedup across arxiv / PubMed / bioRxiv / medRxiv / Semantic Scholar / OpenAlex, with an abstract-only workflow (no PDF download, no full-text extraction).
- **Metadata lookup**: resolve by DOI / PMID / arXiv ID / title and store into a local SQLite library.
- **Full-text fetch**: by citeKey / DOI — Unpaywall OA first → Sci-Hub fallback; PDF saved to disk + full-text cached in the library.
- **Standards-compliant citations**: export BibTeX / RIS / text in GB/T 7714—2025 / APA / IEEE styles.
- **Manuscript typesetting**: `[@citeKey]` placeholders resolved to citation numbers; `--preview` emits self-describing marks for human review.
- **Compliance gate**: 19 mechanical rules (structure / data / punctuation / citations / word count / prose).

**AI-first**: responses default to the minimal field set AI needs (citeKey / title / firstAuthor / year / abstract); full metadata is fetched on demand; `--full` is the escape hatch for humans. The CLI outputs JSON consumable by AI shells; `litkit mcp` starts a stdio MCP Server sharing the same core, so both interfaces produce identical output for identical input.

## Project Principles

- **AI-first, noise-reduced**: interface design optimizes for low context noise above all.
- **CLI first, MCP second**: CLI is the primary interface, MCP is optional; both share the same core and source registry.
- **Abstract-only workflow**: every source must provide abstracts; papers without abstracts are filtered by default (FR-SEARCH-03); no PDF download, no full-text extraction at search time.
- **Free-first**: all sources are public open APIs; no mandatory API keys; keys live in `.env` (gitignored), never hardcoded.
- **Interface sync**: every new feature is registered on both CLI and MCP and mirrored in the API docs (FR-IFACE-03).

## Features

| Capability | Description |
| --- | --- |
| Cross-source search | 6 sources, concurrent + dedup; `-s` source filter / `-n` per-source count / `--mode tiab\|full` / `--years N` |
| Metadata lookup | `metadata doi\|pmid\|arxiv\|title <id>` resolve and store |
| Full-text fetch | Unpaywall OA → Sci-Hub fallback; PDF to disk + full-text cache (zero network on re-fetch) |
| Citations | `export -f bibtex\|ris\|text`; styles GB/T 7714—2025 / APA / IEEE |
| Manuscript typesetting | `[@citeKey]` → `[1][2]`; `--preview` / `--docx` / `-o` |
| Compliance gate | `lint init` scaffolds a project harness; `verify --mode draft\|chapter\|final` |
| Library management | `lib search\|list\|rm\|stats\|path` |
| MCP interface | `litkit mcp` starts a stdio Server; tools map 1:1 to CLI commands |

## Source Strategy

No single search engine — combine public open sources by role:

- **Metadata backbone**: OpenAlex, Semantic Scholar (lookup, metadata enrichment)
- **Discipline sources**: arxiv (preprints), PubMed (biomedical), bioRxiv / medRxiv (life-science preprints)
- **Full-text channels**: Unpaywall (OA resolution, needs an email) → Sci-Hub (fallback, use at your own discretion)

Roadmap: keep current public sources stable; extend on demand (Crossref / dblp / Europe PMC / PMC OA, etc.).

## Source Capability Matrix

| Source | Search | Abstract | Notes |
| --- | --- | --- | --- |
| arxiv | ✅ | ✅ | Preprints; official rate limit 1 req/3s |
| PubMed | ✅ | ✅ | Biomedical |
| bioRxiv / medRxiv | ✅ | ✅ | Life-science preprints |
| Semantic Scholar | ✅ | ✅ | Optional API key raises rate limits (anonymous 429) |
| OpenAlex | ✅ | ✅ | Open metadata backbone |

> Full-text fetching is source-independent: it resolves by DOI via Unpaywall OA → Sci-Hub. Known upstream limitations: Semantic Scholar anonymous rate limiting (429); Sci-Hub mirrors are unstable and may disappear anytime — enabling it is the user's own call.

## Configuration

All configuration is read from `.env` (customizable via `LITKIT_ENV_FILE`); **no API key is required**:

| Variable | Description |
| --- | --- |
| `LITKIT_WORK_DIR` | Working directory (library and config are initialized here) |
| `LITKIT_LANG` | Default language (zh / en) |
| `LITKIT_SEMANTIC_SCHOLAR_API_KEY` | Optional; raises Semantic Scholar rate limits |
| `LITKIT_UNPAYWALL_EMAIL` | Optional; Unpaywall requires an email (OA channel is skipped without it) |
| `LITKIT_SCI_HUB_URL` | Optional; Sci-Hub mirror URL (default sci-hub.se) |
| `LITKIT_HTTP_TIMEOUT_MS` / `LITKIT_HTTP_RETRIES` | Optional; network timeout and retries |

## Installation

Download the prebuilt binary for your platform from [Releases](https://github.com/yn-lo/litkit/releases) (Windows / Linux / macOS × amd64 / arm64), extract it, and add it to your `PATH`.

Build from source (requires Go 1.26+):

```bash
cd app && go build -o litkit ./cmd/litkit
```

## Quick Start

```powershell
$env:LITKIT_WORK_DIR = "$HOME\litkit-workspace"   # Linux/macOS: export LITKIT_WORK_DIR=...

litkit init --type empirical --lang zh                 # 1. Init (review | empirical)
litkit search "retrieval augmented generation" -n 3    # 2. Cross-source search (JSON out)
litkit metadata doi 10.5555/3295222.3295349            # 3. Metadata lookup and store
litkit fetch <citeKey>                                 # 4. Fetch full text (Unpaywall → Sci-Hub)
litkit manuscript draft.md --lang zh                   # 5. Typeset manuscript ([@citeKey] → [1][2])
litkit manuscript draft.md --preview                   # 5b. Preview mode for human review
litkit lint init --type empirical --lang zh            # 6. Scaffold writing constraints
litkit verify chapter1.md --mode draft                 # 6b. Compliance gate
```

## AI Integration

- **CLI**: every command outputs JSON (`--full` prints full metadata); `litkit --help` is self-describing.
- **MCP**: `litkit mcp` starts a stdio Server registering `search_papers` / `get_paper_metadata` / `fetch_paper` / `process_manuscript` / `export_references` / `lint_init` / `verify_manuscript` / `lib_*` / `search_<source>` tools for clients like Claude Desktop / Trae.

**MCP client setup (JSON)**: install litkit and add it to `PATH` (or use an absolute path in `command`), then add the snippet below to your client's MCP config — Claude Desktop: `claude_desktop_config.json` (Settings → Developer); IDEs like Trae / Cursor: their MCP configuration panel:

```json
{
  "mcpServers": {
    "litkit": {
      "command": "litkit",
      "args": ["mcp"],
      "env": {
        "LITKIT_WORK_DIR": "C:\\Users\\<your-name>\\litkit-workspace"
      }
    }
  }
}
```

Key points:

- **`command`**: use `"litkit"` if it's on `PATH`; otherwise give an absolute path, e.g. `"C:\\tools\\litkit.exe"`.
- **`env`**: JSON doesn't support comments and doesn't inherit shell `export`s. Set `LITKIT_WORK_DIR` explicitly here — without it, library / manuscript / compliance-gate tools won't work (`search_papers` still does; failed store upserts don't block search).
- **macOS path**: `"LITKIT_WORK_DIR": "/Users/<your-name>/litkit-workspace"`.
- After saving, restart the client — all tools above become callable in chat; each maps 1:1 to a CLI command with identical input/output.

Full interface contract (CLI / MCP / data model): [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md).

## Documentation

| Doc | Location |
| --- | --- |
| Requirements (PRD) | [`.harness/specs/requirements/PRD.md`](.harness/specs/requirements/PRD.md) |
| Architecture & data flow | [`.harness/specs/architecture/`](.harness/specs/architecture/) |
| Interface spec (CLI / MCP / data model) | [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md) |
| Roadmap | [`.harness/specs/plans/roadmap.md`](.harness/specs/plans/roadmap.md) |
| Conventions / gates | [`.harness/specs/conventions/process.md`](.harness/specs/conventions/process.md) |

## Development

```bash
# Full gate (8 steps: gofmt → build → lint → vet → test → vulncheck → arch-check → sync)
powershell -File .harness/constraints/gate.ps1    # Windows
bash .harness/constraints/gate.sh                 # Linux/macOS

# Interface consistency (CLI / MCP / api.md, FR-IFACE-03)
cd .harness/constraints/sync && go run .

# Network integration tests (manual)
cd app && go test -tags integration ./tests/integration/

# Release: tag to trigger GitHub Actions build & release upload
git tag v0.1.0 && git push origin v0.1.0
```

## Contributing

Requirements, design, and interface baselines live in [`.harness/specs/`](.harness/specs/); key requirements/nodes are developed TDD. Run the full gate before submitting (see [Development](#development)). All contributions are assumed to follow [Apache-2.0](LICENSE).

## License

[Apache-2.0](LICENSE). Copyright © 2026 [YnLo](https://www.ynlo.top/).
