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

## MCP Integration Architecture

### New Architecture (Staff Direct Connection)

Staff processes directly connect to MCP servers, bypassing Boss:

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│    Boss     │     │   Staff     │     │ MCP Server │
│  (管理角色)  │     │  (独立进程)  │────▶│ (npx/uvx)  │
└─────────────┘     └─────────────┘     └─────────────┘
       │                   │
       │ (任务下发)         │ (直接连接)
       ▼                   ▼
```

### Debugging Methodology

When Staff MCP connection fails, follow this systematic approach:

#### Step 1: Identify the Symptom
- **JSON parse error**: Likely stdout pollution (wrong print statement)
- **No tools found**: MCP server started but tools not exposed
- **Registration timeout**: MCP startup blocking Staff initialization

#### Step 2: Isolate the Component
- Run boss and watch stderr output
- Check if Staff processes start at all
- Verify MCP server processes are spawned

#### Step 3: Add Debug Logging (Iterative)

**First, check what's loaded:**
```go
// In staffutil/mcp.go - NewStaffMCPClient
fmt.Fprintf(os.Stderr, "[StaffMCP] 配置加载成功，服务器: %v\n", config.GetEnabledServers())
```

**Then, check what's started:**
```go
// After server.Start()
fmt.Fprintf(os.Stderr, "[StaffMCP] 已启动 %s\n", name)
```

**Then, check tool fetching:**
```go
// In fetchTools()
fmt.Fprintf(os.Stderr, "[StaffMCP:%s] 获取到 %d 个工具\n", s.name, len(s.tools))
```

**Finally, check permission filtering:**
```go
// In refreshTools()
fmt.Fprintf(os.Stderr, "[StaffMCP] refreshTools: tool=%s, allowed=%v\n", tool.Name, allowed)
```

#### Step 4: Common Root Causes

| Symptom | Cause | Fix |
|---------|-------|-----|
| `invalid character 'S'` | `fmt.Printf` writing to stdout | Change to `fmt.Fprintf(os.Stderr, ...)` |
| `tool=fetch, allowed=false` | Tool name mismatch with config | Use server-level ACL check, not tool-level |
| `staff did not register in time` | MCP blocking Staff init | Start MCP servers async or increase timeout |
| Tools always 0 | `refreshTools` called before server ready | Move refresh after server initialization |

#### Step 5: Verify with Minimal Test

Create simplest possible test to isolate:
```bash
./bin/boss 2>&1 | grep -E "(StaffMCP|获取|工具)"
```

### Debug 调试步骤

1. **编译运行**：必须用 `./bin/boss`（不是 `go run`）

2. **观察 stderr**：Staff 日志会输出到 Boss 的 stderr

3. **加调试日志**（在 staffutil/mcp.go）：
   ```go
   // 配置加载后
   fmt.Fprintf(os.Stderr, "[StaffMCP] 配置: %v\n", config.GetEnabledServers())

   // 工具获取后
   fmt.Fprintf(os.Stderr, "[StaffMCP] 获取到 %d 个工具\n", len(s.tools))
   ```

4. **快速测试**：
   ```bash
   ./bin/boss & BOSS_PID=$!; sleep 10; kill $BOSS_PID 2>/dev/null
   ```

### Key Files

| File | Purpose |
|------|---------|
| `internal/staffutil/mcp.go` | Staff MCP 客户端 |
| `config/mcp.yaml` | MCP 服务器配置 |

