# 长期记忆自动化方案

## 现状分析

当前记忆系统的数据流：

```
MEMORY.md (静态文件) ──loadFromFiles()──▶ LongTerm []Message
                                              │
用户对话 ──AddMessage()──▶ ShortTerm []Message ─┤
                                              ▼
                                     GetMessages() → LLM
```

**核心问题**：
1. LongTerm 只是静态文件加载，不会动态更新
2. ShortTerm 达到 50 条上限后直接丢弃旧消息，无摘要
3. FlushToLongTerm() 是空实现
4. Save() 只存 count 元数据，不存内容
5. 没有后台自动精炼机制

## 设计目标

- **短期记忆达到阈值时，自动在后台精炼为长期记忆**
- **长期记忆以结构化摘要形式存储，节省 token**
- **精炼过程异步，不阻塞当前对话**
- **长期记忆自动持久化到 MEMORY.md**

## 方案设计

### 三层记忆架构

```
┌─────────────────────────────────────────────────┐
│                   LLM Context                    │
│  [System] + [LongTerm摘要] + [ShortTerm原文]     │
└─────────────────────────────────────────────────┘
         ▲                          │
         │                          ▼ (达到阈值)
┌────────┴─────────┐    ┌─────────────────────────┐
│   LongTerm       │◀───│   Consolidator (后台)    │
│   结构化摘要      │    │   LLM 精炼 ShortTerm     │
│   持久化到文件    │    │   提取关键知识点          │
└──────────────────┘    └─────────────────────────┘
```

### 核心数据结构

```go
// MemoryEntry 一条长期记忆条目
type MemoryEntry struct {
    ID        string    `json:"id"`
    CreatedAt time.Time `json:"created_at"`
    Category  string    `json:"category"`  // task_result / decision / knowledge / preference
    Summary   string    `json:"summary"`   // 精炼后的摘要 (1-3句话)
    Source    string    `json:"source"`    // 来源标识 (task_id / meeting_id)
}
```

### 精炼触发机制

在 `AddMessage()` 中检测阈值，触发后台精炼：

```go
const (
    MaxShortTermMessages    = 50  // 短期记忆上限
    ConsolidationThreshold  = 30  // 触发精炼的阈值
    ConsolidationBatchSize  = 20  // 每次精炼的消息数
)

func (m *FileMemory) AddMessage(role, content string) {
    m.mu.Lock()
    m.ShortTerm = append(m.ShortTerm, ...)

    // 达到阈值时，取出最旧的一批消息，后台精炼
    if len(m.ShortTerm) >= ConsolidationThreshold && m.consolidator != nil {
        batch := make([]llm.Message, ConsolidationBatchSize)
        copy(batch, m.ShortTerm[:ConsolidationBatchSize])
        m.ShortTerm = m.ShortTerm[ConsolidationBatchSize:] // 移除已精炼的
        m.mu.Unlock()

        // 异步精炼，不阻塞当前对话
        go m.consolidator.Consolidate(batch)
        return
    }
    m.mu.Unlock()
}
```

### Consolidator 精炼器

```go
// Consolidator 记忆精炼器 - 后台将短期记忆精炼为长期记忆
type Consolidator struct {
    llmClient  llm.Client
    model      string
    mu         sync.Mutex
    onComplete func(entries []MemoryEntry) // 精炼完成回调
}

func (c *Consolidator) Consolidate(messages []llm.Message) {
    c.mu.Lock() // 防止并发精炼
    defer c.mu.Unlock()

    // 构造精炼 prompt
    prompt := buildConsolidationPrompt(messages)

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    resp, err := c.llmClient.Complete(ctx, []llm.Message{
        {Role: "system", Content: consolidationSystemPrompt},
        {Role: "user", Content: prompt},
    }, &llm.CompleteOptions{
        Model:       c.model,
        Temperature: 0.2,
        MaxTokens:   500,
    })

    // 解析 LLM 返回的摘要条目
    entries := parseMemoryEntries(resp.Content)

    // 回调写入长期记忆
    if c.onComplete != nil {
        c.onComplete(entries)
    }
}
```

精炼 Prompt：

```
你是记忆管理器。请从以下对话中提取值得长期记住的关键信息。

规则：
1. 只提取有长期价值的信息（决策、偏好、任务结论、学到的知识）
2. 忽略临时性的对话（问候、确认、中间过程）
3. 每条摘要 1-3 句话，简洁明了
4. 分类：task_result / decision / knowledge / preference

输出 JSON 数组：
[{"category": "...", "summary": "..."}]

如果没有值得记住的内容，返回空数组 []
```

### 长期记忆存储与加载

```go
// 精炼完成后的回调 - 追加到 LongTerm 并持久化
func (m *FileMemory) appendLongTermEntries(entries []MemoryEntry) {
    m.mu.Lock()
    defer m.mu.Unlock()

    for _, entry := range entries {
        m.longTermEntries = append(m.longTermEntries, entry)
        // 同时更新 LongTerm messages (给 LLM 用)
        m.LongTerm = m.buildLongTermMessages()
    }

    // 持久化到 MEMORY.md
    m.saveLongTermToFile()
}

// 将结构化条目转为 LLM 可用的消息格式
func (m *FileMemory) buildLongTermMessages() []llm.Message {
    if len(m.longTermEntries) == 0 {
        return nil
    }

    var sb strings.Builder
    sb.WriteString("=== 长期记忆 ===\n")
    for _, e := range m.longTermEntries {
        sb.WriteString(fmt.Sprintf("- [%s] %s\n", e.Category, e.Summary))
    }

    return []llm.Message{{
        Role:    "system",
        Content: sb.String(),
    }}
}
```

### MEMORY.md 文件格式

精炼后自动写入，格式如下：

```markdown
# 长期记忆

## 任务结论
- [2026-03-08] 用户偏好 Go + PostgreSQL 技术栈
- [2026-03-08] PRD 评审通过，需要增加错误码规范

## 决策记录
- [2026-03-08] API 采用 RESTful 风格，不用 GraphQL

## 学到的知识
- [2026-03-08] 项目部署环境为 K8s，需要提供 Dockerfile

## 用户偏好
- [2026-03-08] 用户希望代码注释用中文
```

### 长期记忆上限管理

```go
const MaxLongTermEntries = 100 // 长期记忆最大条目数

// 超出上限时，对最旧的条目进行二次精炼（摘要的摘要）
func (m *FileMemory) compactLongTerm() {
    if len(m.longTermEntries) <= MaxLongTermEntries {
        return
    }
    // 取最旧的一半，精炼为更简短的摘要
    half := len(m.longTermEntries) / 2
    oldEntries := m.longTermEntries[:half]
    // 二次精炼...
    compacted := m.consolidator.CompactEntries(oldEntries)
    m.longTermEntries = append(compacted, m.longTermEntries[half:]...)
}
```

## 需要修改的文件

| 文件 | 变更 |
|------|------|
| `internal/agent/memory.go` | 新增 MemoryEntry, Consolidator；修改 FileMemory 结构体和 AddMessage；实现 FlushToLongTerm；实现 saveLongTermToFile |
| `internal/agent/agent.go` | Agent.Config 新增 LLMClient 用于创建 Consolidator |
| `internal/staffutil/bootstrap.go` | LoadMemory 时初始化 Consolidator |
| `internal/staffutil/agent.go` | NewAgent 时传入 Consolidator 配置 |

## 数据流总览

```
对话进行中:
  User msg → ShortTerm[0..29]  (正常积累)

达到阈值 (30条):
  ShortTerm[0..19] ──异步──▶ Consolidator ──LLM精炼──▶ MemoryEntry[]
       │                                                     │
       ▼ (移除已精炼的)                                       ▼
  ShortTerm[20..29]                               LongTerm += entries
                                                  MEMORY.md 更新

下次对话:
  GetMessages() → [LongTerm摘要] + [ShortTerm原文] → LLM
```

## 关键设计决策

1. **精炼时机**：达到 30 条时触发，而非 50 条（留出 buffer 防止精炼期间丢消息）
2. **异步精炼**：不阻塞当前对话，用 goroutine + mutex 保证单次精炼
3. **LLM 精炼**：用同一个 LLM 做摘要，低温度(0.2)保证稳定性
4. **结构化存储**：分类 + 时间戳，便于检索和管理
5. **二级压缩**：长期记忆超过 100 条时，对旧条目做二次精炼
6. **向后兼容**：MEMORY.md 格式可读，手动编辑的内容也能被加载
