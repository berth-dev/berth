# Berth - plan, execute, verify

**One command. Fully built features. Ship your code.**

Berth is a Go CLI tool that orchestrates autonomous AI coding workflows using Claude Code. It combines GSD's structured planning, Ralph's fresh-context execution, Beads' git-backed task memory, and a Knowledge Graph MCP for deep code understanding -- no other workflow tool has all four.

```
   ┌───────────────────────────────────────┐
   │               berth                   │
   │  Smart AI Workflows for Claude Code   │
   ├───────────────────────────────────────┤
   │                                       │
   │  berth run "add OAuth with Google"    │
   │                                       │
   │  * Understand -- interview + graph    │
   │  * Plan -------- beads + deps         │
   │  * Execute ----- fresh process        │
   │    |-- bt-a1 [x]  configure OAuth     │
   │    |-- bt-a2 [x]  login button        │
   │    |-- bt-a3 [~]  OAuth callback      │
   │    '-- bt-a4 [ ]  integration tests   │
   │  * Report ------ 2/4 complete         │
   │                                       │
   └───────────────────────────────────────┘
```

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Status](https://img.shields.io/badge/Status-Early_Development-orange)]()
[![Claude Code](https://img.shields.io/badge/Built_for-Claude_Code-blueviolet)](https://docs.anthropic.com/en/docs/claude-code)

<!-- TODO: Add demo GIF showing `berth run` with progress display -->
<!-- Record with asciinema or ttygif once the CLI is functional -->

---

## Key Features

- **Fresh context per task** -- every bead gets a clean 200k context window. No context rot. No degraded output.
- **Knowledge Graph MCP** -- AST-parsed code understanding via ts-morph. Claude knows who calls a function before changing its signature.
- **3+1 retry with structured recovery** -- 3 blind retries, 1 diagnostic retry, then Pause with Choices: hint, rescue, skip, or abort. Never silently fails.
- **Beads task memory** -- dependency graphs, close reasons, mid-run task injection. State is queryable, not stuffed into context.
- **Accumulated learnings** -- every session teaches the next one. Your codebase's quirks are remembered across runs.
- **One command** -- `berth run "your feature"`, come back to atomic commits and a PR.

---

## Table of Contents

- [Quick Start](#quick-start)
- [The Problem](#the-problem)
- [How Berth Solves It](#how-berth-solves-it)
- [How It Works](#how-it-works)
- [Architecture](#architecture)
- [Compared To](#compared-to)
- [Knowledge Graph MCP](#knowledge-graph-mcp)
- [Git Workflow](#git-workflow)
- [Configuration](#configuration)
- [State Persistence](#state-persistence)
- [Design Philosophy](#design-philosophy)
- [Inspired By](#inspired-by)
- [Contributing](#contributing)
- [License](#license)

---

## Quick Start

### Prerequisites

- Go 1.23+ (for building from source) or pre-built binary
- [Beads](https://github.com/steveyegge/beads) (`bd`) CLI installed
- Claude Code CLI installed and authenticated
- Node.js 20+ (for Knowledge Graph MCP)
- Git 2.25+

### Install

```bash
# Install from npm (downloads platform-specific Go binary)
npm install -g @berthdev/berth

# Or build from source
git clone https://github.com/user/berth.git
cd berth
make build
```

### Usage

```bash
# Initialize in your project
berth init

# Run a feature
berth run "add user authentication with JWT"

# That's it. Come back to commits and a PR.
```

### Other Commands

```bash
berth status                    # Show current run progress
berth add "fix the logout bug"  # Inject a task mid-run
berth report                    # Show last run results
berth pr                        # Create PR from current run branch
berth resume                    # Resume an interrupted run
```

---

## The Problem

Claude Code is extraordinary for small tasks. But ask it to build a full feature across a large codebase and things fall apart:

- **Context rot** -- output quality degrades as the context window fills. A fresh Claude at 10% is a different animal than a tired Claude at 80%.
- **Hallucination at scale** -- Claude invents imports, references wrong modules, generates plausible code that does not compile.
- **Incomplete features** -- writes the endpoint but skips the validation, implements the happy path and leaves TODOs for edge cases.
- **No memory** -- every session starts from zero. You re-explain the same codebase quirks every time.
- **Finnicky on large projects** -- cannot hold the full architecture in context, makes changes that conflict with existing patterns.

These are symptoms of trying to do too much in a single context window without external structure.

---

## How Berth Solves It

Berth wraps Claude Code in four phases. You see one command in, commits out. Internally:

### 1. Understand
Berth interviews you about the feature via a structured loop: Claude generates questions as JSON, Berth presents them in the terminal, you answer. It queries the Knowledge Graph to understand what already exists. Decisions are locked in `requirements.md`.

Three input modes:
- **Description** (default): `berth run "add OAuth"` -- Berth interviews you
- **PRD file**: `berth run --prd tasks/feature.md` -- Claude reads the PRD, asks only clarifying questions
- **Skip**: `berth run "add OAuth" --skip-understand` -- no interview, straight to planning

### 2. Plan
The feature is broken into beads (tasks) managed by [Beads](https://github.com/steveyegge/beads). Each bead has a description, files to touch, code context from the Knowledge Graph, verification commands, and dependencies. The plan is presented for your approval before execution.

### 3. Execute
For each bead, Berth spawns a fresh Claude process with only what it needs: the bead definition, pre-embedded Knowledge Graph data, and accumulated learnings. It implements, verifies (typecheck, lint, test, build), and commits on success. Failures trigger the 3+1 retry strategy, then Pause with Choices if still stuck.

### 4. Report
Summary of what was built, bead close reasons, commits, stuck beads with failure details, and a PR if configured. Learnings saved for next session.

---

## How It Works

### The Understand Phase

Berth does not start coding immediately. It runs a structured interview loop:

1. **What does the codebase look like?** It queries the Knowledge Graph for file structure, exports, imports, and type relationships relevant to your request.

2. **What do you want?** Claude generates questions as structured JSON. Berth presents them in the terminal with numbered options. You pick an option or type a custom answer. If you are unsure, pick "Help me decide" and Berth spawns a focused explanation call.

3. **Is anything ambiguous?** For complex or vague requests, Claude first decomposes the request into sub-features, asks you to confirm scope, then asks per-feature questions. For simple requests, it asks 3-5 targeted questions and moves on.

The interview ends automatically when Claude signals it has enough information. Results are written to `.berth/runs/<run>/requirements.md`.

### The Plan Phase

The feature is decomposed into beads -- each one a focused task with:
- **Description**: What to implement, in concrete terms
- **Files**: Which files will be created or modified
- **Context**: What already exists in those files (from the Knowledge Graph)
- **Verification**: Commands that prove the task is done (beyond the default pipeline)
- **Dependencies**: Which beads must complete first

The plan is presented for your approval. You can approve, reject with feedback (triggers re-planning), or view full details before deciding. Once approved, beads are created via the `bd` CLI and execution begins.

### The Execute Phase

This is the core engine. For each bead in dependency order:

1. **Spawn fresh Claude process** (no accumulated conversation garbage)
2. **Pre-embed Knowledge Graph data** for the files being modified (Go binary queries SQLite directly -- zero MCP round-trips)
3. **Load**: bead definition + learnings + codebase patterns via `--append-system-prompt`
4. **Implement the bead**
5. **Run verification pipeline**: typecheck, lint, test, build (in order, all must pass)
6. **If pass**: Create an atomic git commit, close the bead with a reason, append learnings, incrementally reindex changed files in the Knowledge Graph
7. **If fail**: Retry up to 3 times blind, then spawn a diagnostic Claude to analyze all 3 errors, retry once more with the diagnosis
8. **If still failing after 3+1 retries**: Pause with Choices:
   - **Hint**: Give the executor a one-liner and retry
   - **Rescue**: Open an interactive Claude session pre-loaded with full error context + Knowledge Graph data
   - **Skip**: Continue with unblocked beads, leave this one stuck
   - **Abort**: Stop the entire run (completed commits are preserved)

The Knowledge Graph MCP is health-checked before each bead. If it crashed, Berth restarts it and reindexes automatically.

### The Report Phase

After all beads complete (or are blocked), Berth produces:
- A summary of what was built
- Bead close reasons (what each bead accomplished)
- The list of commits with messages
- Any stuck beads with failure details
- A PR (if configured or via `berth pr`)
- Updated learnings for the next session

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    BERTH (Go binary)                             │
│                                                                 │
│  CLI Layer (Cobra)                                              │
│  ├── berth init          Smart brownfield/greenfield detection  │
│  ├── berth run "desc"    Full workflow: understand→plan→exec    │
│  │   ├── --prd PATH      Feed PRD file, skip interview          │
│  │   ├── --skip-understand  No interview, just plan and go      │
│  │   ├── --skip-approve  Auto-approve plan (fully autonomous)   │
│  │   ├── --reindex       Force full Knowledge Graph reindex      │
│  │   └── --debug         Pass --mcp-debug to Claude processes   │
│  ├── berth add "task"    Inject task mid-run                    │
│  ├── berth status        Show current progress                  │
│  ├── berth report        Show last run results                  │
│  ├── berth pr            Create PR from current run branch      │
│  └── berth resume        Resume interrupted/aborted run         │
│                                                                 │
│  Core Engine                                                    │
│  ├── Understand    Interview user + query Knowledge Graph       │
│  ├── Plan          Break into beads with dependencies           │
│  ├── Execute       Loop: bd ready → spawn Claude → verify       │
│  └── Report        Summary of what happened                     │
│                                                                 │
│  Integrations                                                   │
│  ├── Beads (bd)           Task memory, dependency graphs        │
│  ├── Claude CLI           Spawns fresh processes per bead       │
│  ├── Knowledge Graph MCP  Code understanding (separate process) │
│  └── Git                  Branching, committing, PR creation    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

KNOWLEDGE GRAPH MCP (Node.js, separate process)
┌─────────────────────────────────────────────────────────────────┐
│  ts-morph AST parser → SQLite graph DB → MCP server             │
│  Tools: get_callers, get_dependents, get_type_usages,           │
│         get_exports, get_importers, analyze_impact              │
│  Smart reindex: mtime-based on startup, incremental per bead    │
│  Queried by Claude during UNDERSTAND + EXECUTE phases           │
└─────────────────────────────────────────────────────────────────┘
```

---

## Compared To

| | Berth | Ralph | GSD | Gas Town | Claude Flow |
|---|---|---|---|---|---|
| **Language** | Go | Bash | npx | Bash/JS | Node.js |
| **Commands to learn** | ~5 | 2 files | ~15 | 50+ | 10+ |
| **Setup** | `berth init` | Bash script | npx install | 6 prerequisites | npm install + config |
| **Context strategy** | Fresh per bead + pre-embedded graph data | Fresh per loop | Fresh per subagent | Fresh per worktree | Tiered routing |
| **Code understanding** | Knowledge Graph MCP (AST-parsed) | None | None | None | None |
| **Task memory** | Beads (dependency graphs, close reasons) | progress.txt | STATE.md + phases | Beads + Seance | SQLite + vector |
| **Failure recovery** | 3+1 retry + Pause with Choices | Infinite loop | Post-hoc verify | Watchdog chain | Failed event |
| **Cost per feature** | ~$10 (bounded) | $5-200 (unbounded) | $10-20 | $50-100 | Unknown |
| **Parallelism** | Sequential (by design) | Sequential | Wave-based | 20-30 agents | Configurable swarm |
| **Verification** | Pipeline per bead (typecheck, lint, test, build) | Tests gate commits | Per-task verify | Refinery tests | Unverified claims |
| **Complexity** | Low | Very low | Medium | Very high | High |
| **Maturity** | Early | Stable | Stable | Alpha | Alpha |

---

## Knowledge Graph MCP

Berth includes a Knowledge Graph MCP server that parses your codebase into a queryable SQLite database using ts-morph (AST-level parsing for TypeScript, ripgrep fallback for other languages).

| Tool | What It Does |
|------|-------------|
| `get_callers(fn)` | Who calls this function? |
| `get_callees(fn)` | What does this function call? |
| `get_dependents(fn)` | What breaks if this changes? (transitive) |
| `get_exports(file)` | All exported functions, types, constants |
| `get_importers(file)` | All files that import from this file |
| `get_type_usages(type)` | Where is this type defined and used? |
| `analyze_impact(files)` | Full blast radius if these files change |

- **Freshness**: Smart mtime-based reindex on startup (~200ms for 3 changed files), incremental reindex between beads (~50ms per file), `--reindex` flag for full rebuild
- **Auto-enable**: Activated for projects with 50+ source files. Override with `knowledge_graph.enabled: "always"` or `"never"` in config.

---

## Git Workflow

Berth manages git for you:

1. **Branch creation**: `berth/feature-name` branch created from current HEAD
2. **Atomic commits**: One bead = one commit with conventional format (`feat(berth): description`)
3. **Clean history**: Each commit represents a logical unit of work that builds, passes all verification, and makes sense on its own
4. **PR creation**: Via `berth pr`, auto-generated from requirements + bead close reasons + learnings

```
main
  \
   \--- berth/oauth-login
          |
          +-- feat(berth): add googleSignIn to auth store
          |
          +-- feat(berth): add Google login button
          |
          +-- feat(berth): handle OAuth callback
          |
          +-- feat(berth): add auth state listener
          |
          +-- test(berth): add OAuth integration tests
```

Berth never force-pushes, never rebases without asking, and never commits to main directly.

---

## Configuration

Berth is opinionated by default but configurable where it matters. Configuration lives in `.berth/config.yaml`, auto-generated by `berth init`.

| Setting | Default | Description |
|---|---|---|
| `project.language` | Auto-detected | Language, framework, package manager |
| `model` | `"opus"` | Model to use (single model, no routing) |
| `execution.max_retries` | `3` | Blind retry attempts before diagnostic |
| `execution.timeout_per_bead` | `600` | Kill Claude process after N seconds |
| `execution.branch_prefix` | `"berth/"` | Prefix for feature branches |
| `execution.auto_pr` | `false` | Auto-create PR on completion |
| `verify_pipeline` | Auto-detected | Commands to run in order per bead (typecheck, lint, test, build) |
| `knowledge_graph.enabled` | `"auto"` | Enable Knowledge Graph (`auto`, `always`, `never`) |

---

## State Persistence

All state lives in the `.berth/` directory at your project root, plus `.beads/` managed by the Beads CLI:

```
.berth/
├── config.yaml       # Project settings (auto-generated)
├── CLAUDE.md         # Persistent context (passed via --append-system-prompt)
├── learnings.md      # Accumulated codebase knowledge (append-only)
├── log.jsonl         # Event log (append-only)
└── runs/             # Per-run artifacts (requirements.md, plan.md)

.beads/               # Managed by bd CLI (dependency graphs, task state)
```

`learnings.md` is the key persistence file. It is append-only -- every session adds what it learned. Over time, it becomes a living knowledge base that prevents your most common mistakes from recurring.

---

## Design Philosophy

- **Fresh context per task** -- every bead executes in a clean context window. No accumulated garbage. Ralph proved it works; Berth makes it structural.
- **Deep questioning before coding** -- structured interview, Knowledge Graph queries, locked decisions. Prevents building the wrong thing confidently.
- **Bounded retries with structured recovery** -- 3+1 automatic retries, then Pause with Choices. No infinite loops. No silent failures.
- **One agent, sequential execution** -- 10x less cost, zero merge conflicts. AI is already 100x faster than humans.
- **Verification gates before every commit** -- typecheck, lint, test, build must all pass. Backpressure that forces working code.
- **AST-level code understanding** -- the Knowledge Graph prevents hallucinated imports, duplicate types, and broken call sites.
- **Accumulated learnings across sessions** -- every session teaches the next one. Institutional knowledge, made explicit.

---

## Inspired By

- **[Ralph](https://github.com/snarktank/ralph)** -- fresh context per task, accumulated learnings, backpressure via tests
- **[GSD](https://github.com/glittercowboy/get-shit-done)** -- deep questioning phase, per-task verification, structured planning
- **[Gas Town](https://github.com/steveyegge/gastown)** -- external memory over in-context memory, what to avoid in multi-agent
- **[Claude Flow](https://github.com/ruvnet/claude-flow)** -- MCP-first architecture, tiered routing insight
- **[Beads](https://github.com/steveyegge/beads)** -- dependency graphs, query-based state, close reasons
- **[Greptile](https://www.greptile.com/)** -- AST-parsed code understanding for AI coding

---

## Contributing

Berth is early. Contributions are welcome.

**Areas that need work:**
- **Core engine** -- execute loop, verification pipeline, 3+1 retry with Pause with Choices
- **Knowledge Graph MCP** -- ts-morph parser, SQLite storage, MCP tool handlers, ripgrep fallback
- **Language detection** -- auto-detecting build/test/lint commands for different ecosystems
- **Interview UX** -- terminal UI for the Understand phase interview loop
- **Testing** -- unit tests, integration tests, dogfooding
- **Distribution** -- npm wrapper package, goreleaser, brew formula

**Guidelines:**
- Keep it simple. If a feature requires explaining, it is probably too complex.
- One agent, sequential execution. Do not add parallelism.
- Every change should make the user's experience simpler, not the architecture more elegant.
- Test with real projects.

**How to contribute:**
1. Fork the repository
2. Create a feature branch
3. Submit a PR with a clear description of what and why

Or open an issue. Real-world failure reports are the most valuable contributions at this stage.

---

## License

MIT
