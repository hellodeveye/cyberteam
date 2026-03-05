# 声明式工具权限设计

## 核心理念

**"权限即配置"** - Staff 能做什么，完全由 PROFILE.md 声明决定，而非硬编码。

## 对比

### 硬编码方案（旧）
```go
// 在代码里写死
defaultAllowedCommands() map[string]bool {
    return map[string]bool{
        "go": true,      // 所有 Staff 都能用
        "python": true,
        // ...
    }
}
```

**问题**:
- 所有 Staff 权限一样
- 修改需要重新编译
- 不灵活

### 声明式方案（新）

```yaml
# PROFILE.md
tools:
  bash:
    enabled: true
    allow:      # 这个 Staff 只能用它声明的工具
      - go
      - git
      - mkdir
    deny:
      - sudo
    timeout: 60s
```

**优点**:
- 每个 Staff 权限独立
- 修改配置即可，无需编译
- 灵活、安全、可审计

## 配置结构

```yaml
tools:
  bash:        # Bash 工具
    enabled: true/false
    allow: []  # 白名单（可选，不填则使用默认）
    deny: []   # 黑名单（追加到默认黑名单）
    timeout: 30s
    max_output: 1048576
  
  git:         # Git 工具（预留）
    enabled: true/false
    allow: []  # 允许的 git 子命令
  
  docker:      # Docker 工具（预留）
    enabled: true/false
```

## 角色权限示例

### Product（产品经理）
```yaml
tools:
  bash:
    enabled: false   # 不需要命令行，专注文档
```

### Developer（开发工程师）
```yaml
tools:
  bash:
    enabled: true
    allow:
      - go           # 开发工具
      - git          # 版本控制
      - mkdir        # 创建目录
      - gofmt        # 代码格式化
    deny:
      - sudo         # 禁止提权
    timeout: 60s
```

### Tester（测试工程师）
```yaml
tools:
  bash:
    enabled: true
    allow:
      - go
      - python       # 测试脚本
      - pytest       # 测试框架
    deny:
      - rm           # 禁止删除
      - mv           # 禁止移动
    timeout: 120s    # 测试可能需要更长时间
```

### DevOps（运维工程师）- 预留
```yaml
tools:
  bash:
    enabled: true
    allow:
      - docker
      - kubectl
      - helm
  docker:
    enabled: true
```

## 使用方式

```go
// 1. 加载 Profile
prof, _ := profile.Load("PROFILE.md")

// 2. 从配置创建工具
bash, err := tools.NewConfigurableBashTool(
    workspacesDir,
    projectName,
    projectID,
    "04-develop",
    prof.Tools,  // ← 使用 Profile 中的配置
)
if err != nil {
    // bash not enabled in profile
    return
}

// 3. 使用（自动受限于配置的权限）
bash.Execute("go build")     // OK
bash.Execute("sudo ls")      // Error: command 'sudo' is blocked
```

## 权限检查流程

```
Staff 调用 Execute("go build")
        │
        ↓
┌───────────────────┐
│ 1. 检查 enabled   │  false → 返回错误
│   是否启用?       │
└─────────┬─────────┘
          │ true
          ↓
┌───────────────────┐
│ 2. 检查 deny      │  true → 拒绝
│   是否在黑名单?   │
└─────────┬─────────┘
          │ false
          ↓
┌───────────────────┐
│ 3. 检查 allow     │  false → 拒绝
│   是否在白名单?   │
└─────────┬─────────┘
          │ true
          ↓
┌───────────────────┐
│ 4. 检查参数       │  危险模式 → 拒绝
│   是否安全?       │
└─────────┬─────────┘
          │
          ↓
     执行命令
```

## 安全优势

1. **最小权限原则** - 每个 Staff 只能用它声明的工具
2. **配置即文档** - 权限一目了然
3. **易于审计** - 配置文件就是审计依据
4. **动态调整** - 修改 PROFILE.md 无需重启系统
5. **防御纵深** - 配置检查 + 运行时检查双重保护

## 扩展性

未来可以添加更多工具类型：

```yaml
tools:
  database:
    enabled: true
    allow:
      - mysql
      - redis-cli
  
  cloud:
    enabled: true
    allow:
      - aws
      - gcloud
```

## 总结

| 特性 | 硬编码 | 声明式 |
|-----|--------|--------|
| 灵活性 | ❌ 差 | ✅ 好 |
| 安全性 | ⚠️ 一般 | ✅ 细粒度控制 |
| 可维护性 | ❌ 需编译 | ✅ 配置即可 |
| 可审计性 | ❌ 看代码 | ✅ 看配置 |
| 角色差异化 | ❌ 统一 | ✅ 独立配置 |
