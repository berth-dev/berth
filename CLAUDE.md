# Berth

Go CLI that orchestrates Claude Code for complex tasks using a bead-based workflow with Knowledge Graph code understanding.

## Tech Stack

- Go 1.23+, Cobra CLI framework
- Node.js Knowledge Graph MCP server (ts-morph + SQLite)
- Module path: `github.com/berth-dev/berth`

## Directory Structure

```
cmd/berth/          Entry point (main.go)
internal/cli/       Cobra command definitions (run, init, add, status, report, pr, resume)
internal/detect/    Brownfield/greenfield project detection
internal/understand/ Phase 1: interview loop + KG queries
internal/plan/      Phase 2: break task into beads with dependencies
internal/execute/   Phase 3: spawn Claude per bead, verify, retry
internal/report/    Phase 4: generate summary
internal/context/   CLAUDE.md generation + learnings management
internal/graph/     Knowledge Graph MCP lifecycle and queries
internal/git/       Branch, commit, PR operations
internal/log/       Append-only JSONL event logging
internal/config/    Read/write .berth/config.yaml
internal/beads/     Beads CLI (bd) wrapper
prompts/            Prompt templates embedded via go:embed
```

## Commands

```
go build -o berth ./cmd/berth    # Build
go test ./...                    # Test
golangci-lint run                # Lint
```

## Conventions

- All internal packages live under `internal/`
- Prompts are embedded via `go:embed` from `prompts/` directory
- Cobra commands: each command in its own file under `internal/cli/`
- Single model: Opus 4.5, no routing logic
- Fresh Claude CLI process per bead (zero context rot)

## Git

- Commit format: `<type>(<scope>): <description>`
- Scope: exactly one word (e.g. `plan`, `execute`, `graph`). Never multiple scopes.
- Types: `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `ci`, `build`, `style`, `chore`, `revert`
- Subject: imperative mood, lowercase, no period, max 50 chars
- Body: only when the *why* isn't obvious from the subject, wrap at 72 chars
- 
### Commit granularity

- A commit is a **reviewable unit**: one feature, one fix, or one refactor. Not one file, not one function.
- Include all supporting changes (config fields, log events, type definitions, helpers) in the same commit as the feature that introduces them. Don't split scaffolding into its own commit.
- Scope describes the **primary** package being changed. Cross-package changes are fine in one commit when they serve a single purpose.
- Only make a separate commit when the change is **independently useful** — e.g. a refactor that has value without the feature, or a bug fix unrelated to the feature.
- Target: 3–8 commits for a multi-package feature. >10 commits where most touch 1 file is over-split.

**Examples:**
```
feat(auth): add login with OAuth
fix(api): resolve timeout on slow connections
docs: update installation instructions
```

## Key Files

- Plan: `sunny-tickling-penguin.md`
- Phase files: `.claude-dev/phases/`
- Architecture reference: `.claude-dev/ARCHITECTURE.md`
- Project config (runtime): `.berth/config.yaml`
