# Code Review: 46f496d feat(mcp): convert Bash to MCP tool with PROFILE.md config

## Overall Assessment

将 Bash 工具转换为 MCP Server 模式，使每个 Staff 根据 PROFILE.md 获得不同 bash 权限。设计方向合理，但存在以下问题需要关注。

---

## Issues Found

### 1. [HIGH] Bug: `handleMCPCall` 错误变量作用域混淆

**File**: `internal/master/manager.go` - `handleMCPCall`

```go
// 第 471 行用了 := 短声明，内层 err 是新变量
result, err := m.mcpManager.CallInternalTool(serverName, toolName, args)
// ...
// 第 479 行使用的是外层 err（第 440 行的），而非内置调用的错误
m.sendMCPError(staffID, err.Error())
```

当 `CallInternalTool` 失败时，第 479 行使用的 `err` 仍然是外部 `CallToolByName` 的错误（因为 `:=` 创建了新变量），导致错误信息误导。

**Fix**:
```go
var lastErr error = err
if serverName != "" {
    result, internalErr := m.mcpManager.CallInternalTool(serverName, toolName, args)
    if internalErr == nil {
        m.sendMCPResult(staffID, msg.ID, result)
        return
    }
    lastErr = internalErr
}
m.sendMCPError(staffID, lastErr.Error())
```

---

### 2. [MEDIUM] 安全: 内置工具列表未按 role 过滤

**File**: `internal/mcp/manager.go` - `ListInternalTools()`

`ListInternalTools()` 返回所有内置 Server 的工具，没有 role 参数。导致 Product Manager 能看到 Developer 的 Bash 工具。

**Fix**: 给 `ListInternalTools` 加上 role 过滤参数。

---

### 3. [MEDIUM] 工作目录使用 staffID 不可复用

**File**: `internal/master/manager.go` - `getStaffWorkDir`

`staffID` 含时间戳 (`developer-1741318836000000000`)，每次启动不同，工作目录无法复用。应考虑使用当前项目的 workspace 路径。

---

### 4. [LOW] 死代码: `BashServer.pending` 字段未使用

**File**: `internal/mcp/server.go:377`

`pending` 从 `ServerInstance` 复制过来但从未使用，应移除。

---

### 5. [LOW] 缺少 command 参数校验

**File**: `internal/mcp/server.go:441`

```go
cmd, _ := args["command"].(string)
```

如果 command 不存在或非 string，会传空字符串给 Execute。应在入口校验。

---

### 6. [LOW] `extractNameFromProfile` 成为死代码

**File**: `internal/master/manager.go:232`

`DiscoverStaffs` 改用 `profile.Load` 后，此方法不再被调用。

---

## Summary

| # | Category | Issue | Severity |
|---|----------|-------|----------|
| 1 | Bug | handleMCPCall err 变量作用域混淆 | HIGH |
| 2 | Security | 内置工具列表未按 role 过滤 | MEDIUM |
| 3 | Design | 工作目录使用 staffID 不可复用 | MEDIUM |
| 4 | Dead Code | BashServer.pending 未使用 | LOW |
| 5 | Robustness | CallTool 缺少 command 参数校验 | LOW |
| 6 | Dead Code | extractNameFromProfile 不再被调用 | LOW |

**建议优先修复**: #1 (err 作用域 bug) 和 #2 (权限过滤)。
