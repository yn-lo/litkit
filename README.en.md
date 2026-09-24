# litkit

An academic writing toolkit for Chinese researchers: cross-source search, standards-compliant citations, manuscript typesetting, and AI-writing compliance gates.

[**English**](README.en.md) | [中文](README.md)

## Overview

litkit is a Go-based paper toolkit (Go 1.26 / cobra / SQLite) with a CLI-only interface, **designed for nursing and clinical medical research** — covering the full workflow from literature search and library management to data statistics and writing. Medical students can use it too; wet-lab basic-science disciplines (e.g., biochemistry) are out of scope. Built for AI agents and command-line users:

- **Cross-source search**: concurrent search + dedup across arxiv / PubMed / bioRxiv / medRxiv / Semantic Scholar / OpenAlex / Crossref / DOAJ; abstract-only workflow (no PDF download, no full-text extraction); `crossref`/`doaj` add Chinese-language retrieval.
- **Library ingestion**: resolve metadata by DOI / PMID / arXiv / title; `lib add --doi` resolves and stores; local SQLite library management.
- **Full-text fetch**: by citeKey / DOI — Unpaywall OA first → Sci-Hub fallback; PDF saved to disk + full text cached (zero network on re-fetch).
- **Standards-compliant citations**: export BibTeX / RIS / text in GB/T 7714—2025 / APA / IEEE styles.
- **Manuscript typesetting**: `[@citeKey]` placeholders resolved to citation numbers; `--preview` emits a self-describing review copy, `--docx` converts to Word.
- **Compliance gate**: mechanical rule checks (language / structure / statistics / punctuation / citations / prose / word counts) across three `verify` modes; `fix` auto-corrects the fixable ones.

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
| Cross-source search    | 8 sources, concurrent + dedup;`-s` filter / `-n` per-source count / `--mode tiab\|full` / `--years N` / `--since YEAR` / `-y YEAR` / `--exclude` |
| Source list            | `sources` lists the registered search sources and their rate limits                                                       |
| Metadata lookup        | `metadata doi\|pmid\|arxiv\|title <id>` (query only); `lib add --doi <DOI>` resolve and store                            |
| Full-text fetch        | `fetch <citeKey\|doi>`: Unpaywall OA → Sci-Hub fallback; PDF to disk + full-text cache (zero network on re-fetch)         |
| Citations              | `export <papers.json> -f bibtex\|ris\|text`; styles GB/T 7714—2025 / APA / IEEE                                           |
| Manuscript typesetting | `manuscript <draft.md>`: `[@citeKey]` → `[1][2]`; `--preview` / `--docx` / `-o`                                            |
| Compliance gate        | `lint init` scaffolds constraints; `verify --mode chapter\|draft\|final` (lang × type × mode filtering); `--report citation-refs` LLM citation scoring; `fix` auto-corrects fixable rules |
| Rule inspection        | `rules` lists all rules (ID / category / langs and paper types / verification method A·S·M)                                |
| Library management     | `lib add\|get\|list\|search\|rm\|stats\|path`                                                                              |

## Sources

No single search engine — combine public open sources by role:

- **Metadata backbone**: OpenAlex, Semantic Scholar, Crossref (lookup, enrichment; also a Chinese-language search source)
- **Chinese-language corpus**: Crossref, DOAJ (Chinese keywords hit Chinese titles/abstracts)
- **Discipline sources**: arxiv (preprints), PubMed (biomedical), bioRxiv / medRxiv (life-science preprints)
- **Full-text channels**: Unpaywall (OA resolution, needs an email) → Sci-Hub (fallback, use at your own discretion)

All sources provide abstracts (FR-SEARCH-03). Known upstream limitations: Semantic Scholar anonymous rate limiting (429); Sci-Hub mirrors are unstable and may disappear anytime — enabling it is the user's own call.

## Environment Variables

All configuration is read from `.env` (customizable via `LITKIT_ENV_FILE`; discovery order is `LITKIT_WORK_DIR/.env` > upward search from the current directory, with process environment variables taking precedence); **no API key is required**:

| Variable                                             | Description                                                                           |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `LITKIT_WORK_DIR`                                  | Working directory (library and config are initialized here)                           |
| `LITKIT_ENV_FILE`                                  | Optional; explicit `.env` path                                                        |
| `LITKIT_LANG`                                      | Default writing language (zh / en, default zh)                                        |
| `LITKIT_HTTP_TIMEOUT_MS` / `LITKIT_HTTP_RETRIES` | Optional; per-request timeout (default 15000 ms) / retries on 429 and 5xx (default 2)  |
| `LITKIT_PROXY_URL`                                 | Optional; explicit proxy (http/https/socks5); when set, all outbound calls (search / metadata / full text / LLM scoring) go through it |
| `LITKIT_SEMANTIC_SCHOLAR_API_KEY`                  | Optional; raises Semantic Scholar rate limits                                         |
| `LITKIT_DEFAULT_MAX_RESULTS` / `LITKIT_DEFAULT_RECENT_YEARS` / `LITKIT_DEFAULT_SEARCH_MODE` / `LITKIT_SEARCH_TIMEOUT_MS` | Optional; search defaults: per-source count (5) / recent years (3) / search mode (tiab) / search timeout |
| `LITKIT_UNPAYWALL_EMAIL`                           | Optional; Unpaywall requires an email (OA channel is skipped without it)              |
| `LITKIT_SCI_HUB_URL` / `LITKIT_FETCH_DOWNLOAD_DIR` | Optional; Sci-Hub mirror (default sci-hub.se) / PDF download directory (default `WORK_DIR/downloads`) |
| `LITKIT_LLM_API_KEY` / `LITKIT_LLM_BASE_URL` / `LITKIT_LLM_TIMEOUT_MS` | Optional; LLM citation-scoring credentials, endpoint and per-call timeout (default 30000 ms). Global fallback; prefer per-model `api_key`/`base_url` in `.litkit/verifier_models.json` (JSON wins), and `LITKIT_LLM_API_KEY_<MODEL_ID_UPPERCASE>` per-model naming is also supported |

See [`app/.env.example`](app/.env.example) for the full list with inline comments.

## Installation

Download the prebuilt binary for your platform from [Releases](https://github.com/yn-lo/litkit/releases) (Windows / Linux / macOS × amd64 / arm64), extract it, and add it to your `PATH`.

Build from source (requires Go 1.26+):

```bash
cd app && go build -o litkit ./cmd/litkit
```

## Quick Start

```powershell
$env:LITKIT_WORK_DIR = "$HOME\litkit-workspace"   # Linux/macOS: export LITKIT_WORK_DIR=...

litkit init --type empirical --lang zh                 # 1. Init (review | empirical | book | proposal)
litkit search "retrieval augmented generation" -n 3    # 2. Cross-source search (JSON out)
litkit lib add --doi 10.5555/3295222.3295349           # 3. Resolve DOI and store
litkit fetch <citeKey>                                 # 4. Fetch full text (Unpaywall → Sci-Hub)
litkit manuscript draft.md --lang zh                   # 5. Typeset manuscript ([@citeKey] → [1][2])
litkit manuscript draft.md --preview                   # 5b. Preview copy for human review
litkit lint init --type empirical --lang zh            # 6. Scaffold writing constraints (for an existing work dir)
litkit verify chapter1.md --mode draft                 # 6b. Compliance gate
litkit fix chapter1.md                                 # 6c. Auto-fix fixable rules (in place)
litkit verify chapter1.md --report citation-refs       # 6d. LLM citation-relevance scoring (needs model credentials)
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

> See [roadmap.md](.harness/specs/plans/roadmap.md) for detailed milestones. M1–M6 are released; M7 (local-library Chinese retrieval and phase-2 sources) and M8 (LLM citation-relevance scoring) are in progress — M8's core has landed (Scorer / ScorerEngine multi-model fan-out / cite-sentence extraction / `citation_scores` cache / `verify --report citation-refs`), with multi-model selection POC and human-labelled threshold calibration remaining. Directions beyond that:

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
