# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

CyberTeam is a Go-based AI software development team using a Boss-Staff architecture. Each Staff is an independent LLM agent (DeepSeek) that communicates with Boss via stdio (JSON Lines). The system simulates a real software development workflow: requirement → design → review → develop → test → deploy.

## Common Commands

```bash
# Build all components
task build

# Build individual components
task build:boss         # Project Manager
task build:staff:product
task build:staff:developer
task build:staff:tester

# Run the system
task run                # Build and run

# Code quality
task fmt                # Format code
task vet                # Static analysis
task check              # Full check (fmt + vet + build)

# Clean build artifacts
task clean
```

Environment variables:
- `DEEPSEEK_API_KEY` - DeepSeek API key (optional, runs in mock mode if not set)
- `DEEPSEEK_MODEL` - Model name (default: deepseek-chat)

## Architecture

```
Boss (Project Manager)
├── Workflow Engine (internal/workflow/)
├── Registry (Staff management)
└── Storage (Project persistence)
    │
    │ stdio (JSON Lines)
    ▼
Staffs (Independent Agents)
├── Product Manager (cmd/staff/product/)
├── Developer (cmd/staff/developer/)
└── Tester (cmd/staff/tester/)
```

### Key Internal Packages

| Package | Purpose |
|---------|---------|
| `internal/master/` | Boss core logic |
| `internal/worker/` | Staff base framework |
| `internal/workflow/` | Workflow engine and stage management |
| `internal/workspace/` | Project workspace and artifact management |
| `internal/storage/` | Project persistence to filesystem |
| `internal/tools/` | Bash tool with security (whitelist/sandbox) |
| `internal/llm/` | DeepSeek LLM client |
| `internal/profile/` | PROFILE.md loader and parsing |
| `internal/protocol/` | stdio message format |

### Boss-Staff Communication

Boss spawns Staff processes as independent executables. Communication uses JSON Lines format over stdio:
- Boss sends `Task` messages to Staffs
- Staffs respond with `Result` messages
- Each Staff has its own `PROFILE.md` declaring capabilities and tool permissions

### Permission System

Staff capabilities are declared in `PROFILE.md` (YAML frontmatter + Markdown body):
```yaml
---
tools:
  bash:
    enabled: true
    allow: [go, git, mkdir, cat, ls]
    deny: [sudo, rm -rf /]
---
# Markdown role description
```

## Project Structure

```
cmd/
├── boss/main.go           # Boss entry point
└── staff/
    ├── product/           # Product Manager
    ├── developer/         # Developer
    └── tester/            # Tester
internal/
├── master/                # Boss implementation
├── worker/                # Staff base
├── workflow/              # Stage engine
├── workspace/             # Project artifacts
├── storage/               # Persistence
├── tools/                 # Bash security
├── llm/                   # LLM client
├── profile/               # Profile loader
└── protocol/              # Message types
workspaces/                # Project outputs (git-ignored)
```

## Workflow Stages

1. **requirement** - Product Manager analyzes requirements
2. **design** - Developer creates system design
3. **review** - Product Manager reviews design
4. **develop** - Developer implements features
5. **test** - Tester runs tests
6. **deploy** - Developer deploys
7. **done** - Complete

Each stage produces artifacts in `workspaces/<project>-<id>/<stage>/`.
