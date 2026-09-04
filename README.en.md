# litkit

An academic writing toolkit for Chinese researchers: cross-source search, standards-compliant citations, manuscript typesetting, and AI-writing compliance gates.

[**English**](README.en.md) | [中文](README.md)

## Overview

litkit is a Go-based paper toolkit (Go 1.26 / cobra / SQLite) with a CLI-only interface, designed for AI agents and command-line users:

- **Cross-source search**: concurrent search + dedup across arxiv / PubMed / bioRxiv / medRxiv / Semantic Scholar / OpenAlex; abstract-only workflow (no PDF download, no full-text extraction).
- **Library ingestion**: resolve metadata by DOI / PMID / arXiv / title; `lib add --doi` resolves and stores; local SQLite library management.
- **Full-text fetch**: by citeKey / DOI — Unpaywall OA first → Sci-Hub fallback; PDF saved to disk + full text cached (zero network on re-fetch).
- **Standards-compliant citations**: export BibTeX / RIS / text in GB/T 7714—2025 / APA / IEEE styles.
- **Manuscript typesetting**: `[@citeKey]` placeholders resolved to citation numbers; `--preview` emits a self-describing review copy, `--docx` converts to Word.
- **Compliance gate**: mechanical rule checks (language / structure / statistics / punctuation / citations / prose / word counts) across three `verify` modes.

**AI-first**: responses default to the minimal field set AI needs (citeKey / title / firstAuthor / year / abstract); full metadata is fetched on demand; the CLI outputs JSON consumable by AI shells.

## Project Principles

- **AI-first, noise-reduced**: interface design optimizes for low context noise above all.
- **CLI-only interface**: every feature is reachable through CLI commands.
- **Abstract-only workflow**: sources must provide abstracts; papers without abstracts are filtered by default; no PDF download, no full-text extraction.
- **Free-first**: all sources are public open APIs; no mandatory API keys; keys live in `.env` (gitignored), never hardcoded.
- **Interface sync**: every new CLI feature is mirrored in the API docs api.md.

## Features

| Capability             | Description                                                                                                               |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| Cross-source search    | 6 sources, concurrent + dedup;`-s` filter / `-n` per-source count / `--mode tiab\|full` / `--years N`              |
| Metadata lookup        | `metadata doi\|pmid\|arxiv\|title <id>` (query only); `lib add --doi <DOI>` resolve and store                            |
| Full-text fetch        | Unpaywall OA → Sci-Hub fallback; PDF to disk + full-text cache (zero network on re-fetch)                                |
| Citations              | `export -f bibtex\|ris\|text`; styles GB/T 7714—2025 / APA / IEEE                                                        |
| Manuscript typesetting | `[@citeKey]` → `[1][2]`; `--preview` / `--docx` / `-o`                                                         |
| Compliance gate        | `lint init` scaffolds a harness; `verify --mode draft\|chapter\|final`; `--report citation-refs` LLM citation scoring |
| Library management     | `lib add\|search\|list\|rm\|stats\|path`                                                                                     |

## Sources

No single search engine — combine public open sources by role:

- **Metadata backbone**: OpenAlex, Semantic Scholar, Crossref (lookup, enrichment)
- **Discipline sources**: arxiv (preprints), PubMed (biomedical), bioRxiv / medRxiv (life-science preprints)
- **Full-text channels**: Unpaywall (OA resolution, needs an email) → Sci-Hub (fallback, use at your own discretion)

All sources provide abstracts (FR-SEARCH-03). Known upstream limitations: Semantic Scholar anonymous rate limiting (429); Sci-Hub mirrors are unstable and may disappear anytime — enabling it is the user's own call.

## Configuration

All configuration is read from `.env` (customizable via `LITKIT_ENV_FILE`); **no API key is required**:

| Variable                                             | Description                                                                           |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `LITKIT_WORK_DIR`                                  | Working directory (library and config are initialized here)                           |
| `LITKIT_LANG`                                      | Default language (zh / en)                                                            |
| `LITKIT_SEMANTIC_SCHOLAR_API_KEY`                  | Optional; raises Semantic Scholar rate limits                                         |
| `LITKIT_UNPAYWALL_EMAIL`                           | Optional; Unpaywall requires an email (OA channel is skipped without it)              |
| `LITKIT_SCI_HUB_URL`                               | Optional; Sci-Hub mirror URL (default sci-hub.se)                                     |
| `LITKIT_HTTP_TIMEOUT_MS` / `LITKIT_HTTP_RETRIES` | Optional; network timeout and retries                                                 |
| `LITKIT_LLM_API_KEY`                               | Optional; LLM citation-scoring key                                                    |
| `LITKIT_LLM_BASE_URL`                              | Optional; self-hosted LLM endpoint                                                    |
| `LITKIT_VERIFY_LINT_LLM`                           | Optional; enable LLM citation scoring (default false, avoids unexpected remote calls) |

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
litkit lib add --doi 10.5555/3295222.3295349           # 3. Resolve DOI and store
litkit fetch <citeKey>                                 # 4. Fetch full text (Unpaywall → Sci-Hub)
litkit manuscript draft.md --lang zh                   # 5. Typeset manuscript ([@citeKey] → [1][2])
litkit manuscript draft.md --preview                   # 5b. Preview copy for human review
litkit lint init --type empirical --lang zh            # 6. Scaffold writing constraints
litkit verify chapter1.md --mode draft                 # 6b. Compliance gate
```

## Interface & Documentation

- **CLI**: every command outputs JSON (`--full` prints full metadata); `litkit --help` is self-describing.
- Full interface contract (CLI / data model): [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md).

| Doc                               | Location                                                                          |
| --------------------------------- | --------------------------------------------------------------------------------- |
| Requirements (PRD)                | [`.harness/specs/requirements/PRD.md`](.harness/specs/requirements/PRD.md)       |
| Architecture & data flow          | [`.harness/specs/architecture/`](.harness/specs/architecture/)                   |
| Interface spec (CLI / data model) | [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md)             |
| Roadmap                           | [`.harness/specs/plans/roadmap.md`](.harness/specs/plans/roadmap.md)             |
| Conventions / gates               | [`.harness/specs/conventions/process.md`](.harness/specs/conventions/process.md) |

## Future Plan

> See [roadmap.md](.harness/specs/plans/roadmap.md) for detailed milestones. Directions beyond the released (M1–M6) and in-progress (M7 semantic search, M8 LLM citation scoring) milestones:

- **Chart generation**: emit SVG directly (forest plots / PRISMA flow diagram / included-literature stats) from the local library.
- **Systematic review `litkit review`**: PRISMA workflow (search → dedup → screening → inclusion list → forest-plot data).
- **Statistical analysis**: run tests via wrapped R/Rscript (optional dependency); hand off data and read results back into the manuscript.
- **AI qualitative research**: LLM-assisted qualitative coding and thematic analysis (standalone command family).

## Development

```bash
# Full gate (gofmt → build → lint → vet → test → vulncheck → arch-check → sync)
powershell -File .harness/constraints/gate.ps1    # Windows
bash .harness/constraints/gate.sh                 # Linux/macOS
```

## Contributing

Requirements, design, and interface baselines live in [`.harness/specs/`](.harness/specs/); key requirements/nodes are developed TDD. Run the full gate before submitting (see [Development](#development)). All contributions are assumed to follow [Apache-2.0](LICENSE).

## License

[Apache-2.0](LICENSE). Copyright © 2026 [YnLo](https://www.ynlo.top/).
