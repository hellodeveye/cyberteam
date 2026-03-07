# CyberTeam - AI 软件开发团队

一个基于 Go + stdio 的 Boss-Staff 智能体集群，模拟真实的软件开发团队运作。每个 AI Staff 都是独立的 LLM Agent，支持声明式权限配置和安全的 Bash 工具执行。

## 核心特性

- **实时会议系统**：独立会议室，支持 @点名、上下文记忆、自动工具执行
- **真实团队协作**：需求分析 → 设计 → 评审 → 开发 → 测试 → 部署
- **声明式权限**：员工能力通过 PROFILE.md 声明，灵活可控
- **安全工作区**：支持 Bash 工具执行，但有完整的安全限制
- **工作流引擎**：自动推进项目阶段，支持审批/驳回闭环
- **持久化存储**：项目产物保存到文件系统，重启不丢失
- **LLM 驱动**：每个员工都是独立的 LLM Agent，使用 DeepSeek API
- **独立进程**：每个角色是独立的可执行文件，支持异构实现
- **MCP 工具集成**：支持 Model Context Protocol，可调用外部工具（GitHub、网页抓取等）

## 架构设计

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Boss (项目经理)                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐  │
│  │  Workflow    │  │   Meeting    │  │   Storage (Persistence)  │  │
│  │   Engine     │  │    Room      │  │   workspaces/            │  │
│  └──────────────┘  └──────────────┘  └──────────────────────────┘  │
└──────────────────┬──────────────────────────────────────────────────┘
                   │ stdio (JSON Lines)                │
       ┌───────────┼───────────────┐                  │
       ▼           ▼               ▼                  ▼
┌────────────┐ ┌──────────┐ ┌────────────┐    ┌──────────────┐
│  张产品     │ │  李开发   │ │  王测试     │    │   Meeting    │
│ 产品经理    │ │ 开发工程师│ │ 测试工程师  │    │   Records    │
│ ├ PROFILE  │ │ ├ PROFILE│ │ ├ PROFILE  │    │  meetings/   │
│ └ tools:   │ │ └ tools: │ │ └ tools:   │    └──────────────┘
│   bash:    │ │   bash:  │ │   bash:    │
│   enabled  │ │   enabled│ │   enabled  │
│   allow: []│ │   allow: │ │   allow:   │
└────────────┘ └──────────┘ └────────────┘
         
         工作区 (workspaces/)
         └── BlogSystem-17727282/
             ├── README.md
             ├── project.json          ← 项目元数据
             ├── 01-requirement/
             │   └── prd.md
             ├── 02-design/
             │   └── design.md
             ├── 04-develop/
             │   ├── main.go
             │   ├── go.mod
             │   └── internal/
             └── 05-test/
                 └── test_report.json
```

## 快速开始

### 1. 安装依赖

```bash
# 安装 Task（可选但推荐）
go install github.com/go-task/task/v3/cmd/task@latest

# 安装依赖
go mod tidy
```

### 2. 配置 LLM（可选）

默认使用模拟模式。要启用真实 DeepSeek LLM：

```bash
# 方式1：使用 DeepSeek 环境变量
export DEEPSEEK_API_KEY=your-api-key
export DEEPSEEK_MODEL=deepseek-reasoner

# 方式2：使用 OpenAI 兼容环境变量
export OPENAI_API_KEY=your-api-key
export OPENAI_BASE_URL=https://api.deepseek.com/v1
export OPENAI_MODEL=deepseek-reasoner
```

### 3. 配置 MCP 工具（可选）

MCP (Model Context Protocol) 允许 Staff 调用外部工具：

```bash
# 配置 Firecrawl（网页抓取）
export FIRECRAWL_API_KEY=your-firecrawl-key

# 配置 GitHub
export GITHUB_TOKEN=ghp_xxxxxxxxxxxx

# 配置 Slack
export SLACK_TOKEN=xoxb-xxxxxxxxxxxx
export SLACK_TEAM_ID=Txxxxxxxx

# 配置 PostgreSQL
export DATABASE_URL=postgresql://user:pass@localhost/db
```

编辑 `config/mcp.yaml` 启用需要的工具：

```yaml
servers:
  fetch:
    enabled: true  # 无需配置，开箱即用
  
  firecrawl:
    enabled: true  # 需要 FIRECRAWL_API_KEY
  
  github:
    enabled: false  # 需要 GITHUB_TOKEN
```

### 4. 编译运行

```bash
# 使用 Task（推荐）
task build && task run

# 或手动
mkdir -p bin
go build -o bin/boss ./cmd/boss
go build -o cmd/staff/product/product ./cmd/staff/product
go build -o cmd/staff/developer/developer ./cmd/staff/developer
go build -o cmd/staff/tester/tester ./cmd/staff/tester
./bin/boss
```

## 使用指南

### 独立会议系统

随时召开团队会议，无需绑定项目：

```bash
🎤 > meeting start 需求评审        # 开始会议
🎤 > meeting start 技术讨论 --mode free  # 指定模式

# 直接发言（无需 say 命令）
🎤 > 大家好
🎤 > 这个需求大家怎么看？

# @ 点名提问
🎤 > @李开发 评估一下技术可行性
🎤 > @张产品 @王测试 一起讨论下

# 会议管理
🎤 > meeting list                   # 列出所有会议
🎤 > meeting join xxx               # 加入已有会议
🎤 > meeting transcript             # 查看完整记录
🎤 > meeting end                    # 结束并保存
```

**会议模式：**
- `free` (默认) - 自由讨论，随机 1-2 人回复
- `round` - 轮流发言
- `boss` - Boss 主导

**智能特性：**
- **上下文记忆** - Staff 知道之前说了什么
- **自动工具执行** - 问"磁盘空间够吗"自动执行 `df -h`
- **MCP 工具调用** - 需要外部数据时自动调用 GitHub、网页抓取等工具
- **彩色区分** - 不同角色不同颜色（产品绿/开发蓝/测试黄/Boss紫）
- **右对齐时间** - 灰色时间戳，不干扰阅读

### 会话式交互（类似 tmux）

```bash
🏢 CyberTeam - AI 软件开发团队
==============================

📂 已加载 2 个历史项目
   - TodoApp (completed)
   - BlogSystem (in_progress)

✅ 团队组建完成！

🎤 > new BlogSystem 一个博客系统
✅ 项目创建: BlogSystem (ID: 17727282)
🔀 已进入项目 [BlogSystem]

🎤 [BlogSystem] >            ← 当前在项目中
🎤 [BlogSystem] > status     ← 查看项目状态
🎤 [BlogSystem] > tasks      ← 查看任务列表
🎤 [BlogSystem] > ..         ← 退出项目
🎤 > projects                ← 列出所有项目
🎤 > cd 17727282             ← 进入项目
```

### 完整工作流演示

```bash
🎤 [BlogSystem] > status

📁 项目: BlogSystem
   状态: in_progress
   当前阶段: requirement

📋 最新任务:
   [需求分析] ⏳ 需求分析 (张产品)

# 等待任务完成...
✅ 任务完成: 需求分析 [requirement]
💡 输入 'approve 17727282' 继续，或 'reject 17727282 <原因>' 打回

🎤 [BlogSystem] > approve 17727282
✅ 已批准任务: 17727282
⏳ 工作流正在推进到下一阶段...

# 系统自动创建设计任务并分配
📋 新任务: [design] 设计: BlogSystem
👤 任务分配: 设计 → Developer

# 设计完成，继续推进
✅ 任务完成: 设计: BlogSystem [design]
🎤 [BlogSystem] > approve xxx

# 开发阶段
📋 新任务: [develop] 开发: BlogSystem
👤 任务分配: 开发 → Developer

# 测试阶段
📋 新任务: [test] 测试: BlogSystem
👤 任务分配: 测试 → Tester

# 最终完成
✅ 项目 BlogSystem 已完成！
```

### 查看产出物

```bash
🎤 [BlogSystem] > artifacts

📦 产出物列表:
   📁 01-requirement/
      📄 requirement-output.json
   📁 02-design/
      📄 design-output.json
   📁 04-develop/
      📄 main.go
      📄 go.mod
      📄 internal/service/blog.go

🎤 [BlogSystem] > show prd     ← 查看 PRD 内容
🎤 [BlogSystem] > show code    ← 查看代码
```

### MCP 工具使用

Staff 可以在会议或私聊中自动调用 MCP 工具获取外部数据：

```bash
# 查看 MCP 工具状态
🎤 > mcp
🛠️ MCP 工具状态:
----------------------------------------
  ✅ fetch: ready
  ✅ firecrawl: ready
  ❌ github: not ready  # 缺少 Token

# 在会议中使用（自动触发）
🎤 > meeting start 技术调研
🎤 > @Alex 查一下 Gin 框架的 GitHub 仓库
Alex: 让我查一下...
     [TOOL:github:search_repositories]{"query":"gin golang"}
     [工具查询结果]
     gin-gonic/gin: 78k stars, HTTP web framework...

🎤 > @Sarah 抓取这个网页看看
Sarah: 我来抓取...
      [TOOL:fetch:fetch_url]{"url":"https://example.com"}
      [工具查询结果]
      网页标题: Example Domain...
```

**支持的 MCP 工具：**

| 工具 | 功能 | 需要配置 |
|-----|------|---------|
| `fetch` | 网页抓取 | 无需配置 |
| `firecrawl` | 高级网页抓取 | `FIRECRAWL_API_KEY` |
| `github` | GitHub API | `GITHUB_TOKEN` |
| `slack` | Slack 通知 | `SLACK_TOKEN` |
| `postgres` | 数据库查询 | `DATABASE_URL` |

## 工作流阶段

```
┌─────────────────────────────────────────────────────────────────┐
│                        工作流流程图                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐        │
│  │ requirement │───▶│    design   │───▶│    review   │        │
│  │  产品经理    │    │  开发工程师  │    │  产品经理    │        │
│  │ analyze_req │    │ design_system│    │ design_review│        │
│  └──────┬──────┘    └──────┬──────┘    └──────┬──────┘        │
│         │                   │                   │               │
│         │                   │                   ▼               │
│         │                   │            ┌─────────────┐        │
│         │                   │            │   develop   │        │
│         │                   │            │  开发工程师  │        │
│         │                   │            │ implement_f │        │
│         │                   │            └──────┬──────┘        │
│         │                   │                   │               │
│         │                   │                   ▼               │
│         │                   │            ┌─────────────┐        │
│         │                   │            │     test    │        │
│         │                   │            │  测试工程师  │        │
│         │                   │            │ execute_test│        │
│         │                   │            └──────┬──────┘        │
│         │                   │                   │               │
│         │                   │                   ▼               │
│         │                   │            ┌─────────────┐        │
│         │                   │            │    deploy   │        │
│         │                   │            │  开发工程师  │        │
│         │                   │            │ deploy_svc  │        │
│         │                   │            └──────┬──────┘        │
│         │                   │                   │               │
│         └───────────────────┴───────────────────┴──────▶ done   │
│                                                                 │
│  reject ───────────────────────────────────────────────────────▶│
│            (任意阶段可打回重做)                                  │
└─────────────────────────────────────────────────────────────────┘
```

## 声明式权限系统

每个员工的能力通过 `PROFILE.md` 声明：

```yaml
# cmd/staff/developer/PROFILE.md
---
name: 李开发
role: developer
capabilities:
  - name: design_system
    description: 系统设计
    inputs:
      - name: prd
        type: string
        required: true
    outputs:
      - name: design
        type: string

  - name: implement_feature
    description: 功能开发
    ...

# 工具声明（声明式权限）
tools:
  bash:
    enabled: true
    allow:                    # 允许的命令
      - go
      - git
      - mkdir
      - cat
      - ls
      - gofmt
    deny:                     # 禁止的命令
      - sudo
      - su
      - chmod
    timeout: 60s
    max_output: 2097152       # 2MB
  
  git:
    enabled: true
    allow:
      - init
      - add
      - commit
      - push
---

# Markdown 正文（角色设定）
## 工作职责
- 根据设计文档实现功能代码
- 编写单元测试

## 编码规范
1. 代码完整可运行
2. 包含错误处理
```

### 权限对比

| 角色 | Bash | 允许命令 | 说明 |
|-----|------|---------|------|
| **Product** | ❌ | - | 专注文档，无需命令行 |
| **Developer** | ✅ | go, git, mkdir, cat, ls, gofmt | 开发工具 |
| **Tester** | ✅ | go, python, pytest, cat, ls | 测试工具 |

## 安全机制

### 1. 目录沙箱
```
工作目录限制: workspaces/<project>-<id>/<stage>/
禁止: ../ 目录遍历攻击
```

### 2. 命令白名单与净化
```go
// 完全禁止危险的 shell 元字符
dangerous := []string{";", "|", "&", ">", "<", "`", "$", "(", ")", "{", "}", ...}

允许: go, python, git, mkdir, cat, ls, gofmt...
禁止: sudo, su, ssh, rm -rf /, mkfs, dd...
```

### 3. 路径遍历防护
```go
// 使用 filepath.Base 提取纯文件名
filename = filepath.Base(filename)
if filename == "" || filename == "." {
    return fmt.Errorf("invalid filename")
}
```

### 3. 审计日志
```go
type CommandRecord struct {
    Time      time.Time // 执行时间
    Command   string    // 命令内容
    WorkDir   string    // 工作目录
    Success   bool      // 执行结果
    Output    string    // 输出内容
    Duration  int64     // 执行耗时
}
```

### 4. 审计示例
```bash
🎤 [BlogSystem] > show audit

📋 执行历史:
[2026-03-06 01:30:00] go mod init blog    - Success (120ms)
[2026-03-06 01:30:05] go build -o app     - Success (2.5s)
[2026-03-06 01:30:10] go test -v ./...    - Success (5.1s)
```

## 项目结构

```
agent-cluster/
├── bin/
│   └── boss                          # 项目经理可执行文件
├── cmd/
│   ├── boss/
│   │   └── main.go                   # Boss 入口
│   └── staff/
│       ├── product/
│       │   ├── main.go               # 产品经理
│       │   └── PROFILE.md            # 角色配置
│       ├── developer/
│       │   ├── main.go               # 开发工程师
│       │   └── PROFILE.md            # 角色配置
│       └── tester/
│           ├── main.go               # 测试工程师
│           └── PROFILE.md            # 角色配置
├── internal/
│   ├── llm/
│   │   └── client.go                 # LLM 客户端
│   ├── profile/
│   │   └── loader.go                 # PROFILE.md 解析
│   ├── protocol/
│   │   └── message.go                # stdio 通信协议
│   ├── master/
│   │   └── manager.go                # Boss 核心逻辑
│   ├── meeting/
│   │   ├── room.go                   # 会议房间管理
│   │   └── participant.go            # 会议参与者
│   ├── worker/
│   │   └── base.go                   # Staff 基础框架
│   ├── workflow/
│   │   ├── engine.go                 # 工作流引擎
│   │   └── presets.go                # 预定义工作流
│   ├── workspace/
│   │   └── manager.go                # 工作空间管理
│   ├── storage/
│   │   └── store.go                  # 项目持久化
│   └── tools/
│       ├── bash.go                   # Bash 工具
│       └── staff_bash.go             # Staff 专用封装
├── workspaces/                       # 项目工作空间
│   └── BlogSystem-17727282/
│       ├── README.md
│       ├── project.json
│       ├── 01-requirement/
│       ├── 02-design/
│       ├── 04-develop/
│       └── 05-test/
├── meetings/                         # 会议记录
│   └── mtg-1772776077421400012/
│       ├── meeting.json              # 完整消息历史
│       └── transcript.md             # 可读会议记录
├── docs/
│   ├── bash-tool-design.md
│   └── declarative-permissions.md
├── Taskfile.yml
└── README.md
```

## 核心概念

### 1. 声明式权限 (PROFILE.md)

员工能力通过 YAML 声明，而非硬编码：

```yaml
tools:
  bash:
    enabled: true
    allow: [go, git, mkdir]
    deny: [sudo, rm -rf /]
```

### 2. 工作流引擎

```go
// 阶段定义
Stage{
    Name: StageDesign,
    OnComplete: func(engine, project, task) {
        // 自动创建下一阶段任务
        engine.CreateTask(project.ID, StageReview, ...)
    },
}
```

### 3. 断点续传

```bash
# Boss 重启后自动恢复
🏢 CyberTeam

📂 已加载 3 个历史项目
   - TodoApp (completed)
   - BlogSystem (in_progress)

🔄 恢复任务: 开发: BlogSystem [develop] → developer
✅ 已恢复 1 个任务
```

## Task 命令速查

```bash
# 开发
task build              # 编译所有
task build:boss         # 只编译 boss
task build:staffs       # 编译所有 staff
task run                # 编译并运行
task clean              # 清理

# 调试
task staff:product      # 单独运行产品经理
task staff:dev          # 单独运行开发
task staff:test         # 单独运行测试

# 代码质量
task fmt                # 格式化
task vet                # 静态检查
task check              # 完整检查
```

## 配置说明

### 环境变量

| 变量 | 说明 | 默认值 |
|-----|------|-------|
| `DEEPSEEK_API_KEY` | DeepSeek API Key | -（模拟模式） |
| `DEEPSEEK_MODEL` | 模型名称 | deepseek-reasoner |
| `OPENAI_API_KEY` | OpenAI 兼容 API Key | - |
| `OPENAI_BASE_URL` | API 基础 URL | https://api.openai.com/v1 |
| `OPENAI_MODEL` | 模型名称 | - |
| `FIRECRAWL_API_KEY` | Firecrawl API Key | - |
| `GITHUB_TOKEN` | GitHub Personal Access Token | - |
| `SLACK_TOKEN` | Slack Bot Token | - |
| `SLACK_TEAM_ID` | Slack Team ID | - |
| `DATABASE_URL` | PostgreSQL 连接字符串 | - |

### PROFILE.md 配置

```yaml
# 能力声明
capabilities:
  - name: design_system
    inputs: [...]
    outputs: [...]

# 工具权限
tools:
  bash:
    enabled: true/false
    allow: []      # 白名单
    deny: []       # 黑名单
    timeout: 60s

# 角色设定
---
# Markdown 格式的角色说明
```

## 故障排查

| 问题 | 可能原因 | 解决方案 |
|-----|---------|---------|
| 任务分配失败 | Staff 未注册 | 等待团队组建完成 |
| Staff 离线 | 进程崩溃 | 重启 Boss 自动恢复任务 |
| LLM 调用失败 | API Key 无效 | 检查 `DEEPSEEK_API_KEY` |
| 命令被拒绝 | 不在白名单 | 检查 PROFILE.md 的 tools.allow |

## 后续优化

### MCP 待完善

- [x] **Tool 参数 Schema 暴露**：`MCPToolInfo` 已加 `InputSchema` 字段，`ListTools()` 传递 Schema，`GetToolsPrompt()` 格式化参数供 LLM 使用
- [x] **Product / Tester 接入 MCPClient**：三个角色均已注入 MCPClient（经核查已完成）

### 功能扩展

- [ ] **语音会议**：支持语音输入和 TTS 回复
- [ ] **容器隔离**：在 Docker 容器中执行 Bash 命令
- [ ] **Web UI**：可视化项目进度和会议记录
- [ ] **异构支持**：不同 Staff 用不同语言实现
- [ ] **更多角色**：运维、设计师、数据分析师
- [ ] **插件系统**：动态加载新能力
- [ ] **会议摘要**：自动生成会议纪要和待办

## License

MIT
