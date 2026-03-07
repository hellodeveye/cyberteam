# Bash Tool 设计方案

## 设计目标

为 AI Staff 提供安全、可控的命令执行能力，使其能够：
1. 编写实际代码文件
2. 执行编译、测试等开发命令
3. 保持系统安全和可控

## 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                        Staff Process                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │   LLM Agent  │→ │ Bash Tool    │→│  File System     │  │
│  │              │  │ (Safe Exec)  │  │ (workspaces/)    │  │
│  └──────────────┘  └──────────────┘  └──────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ↓
                     ┌──────────────────┐
                     │   Audit Log      │
                     │   (History)      │
                     └──────────────────┘
```

## 安全机制

### 1. 目录隔离
- **工作目录限制**: Staff 只能在 `workspaces/<project>-<id>/<stage>/` 下操作
- **路径检查**: 所有路径操作前检查，防止 `../` 等目录遍历

### 2. 命令白名单
```go
允许: go, python, git, ls, cat, mkdir, cp, mv, rm, echo, find, grep...
禁止: sudo, su, ssh, scp, reboot, rm -rf /, mkfs, dd...
```

### 3. 参数检查
- 过滤危险模式: `/etc/passwd`, `~/.ssh`, `rm -rf /` 等
- 禁止命令替换: `` `command` `` 和 `$(command)`

### 4. 环境隔离
- 清理环境变量，只保留必要的 `PATH`, `HOME`, `GOPATH` 等
- 不允许修改系统环境

### 5. 资源限制
- **超时**: 默认 30 秒，防止长时间运行
- **输出限制**: 最大 1MB，防止内存溢出

### 6. 审计日志
- 记录所有命令执行: 时间、命令、目录、结果、输出
- 保留最近 1000 条记录

## API 设计

### 基础执行
```go
bash := tools.NewStaffBashTool(workspacesDir, projectName, projectID, stage)

// 执行单条命令
output, err := bash.Execute("go build -o app")

// 执行脚本（多条命令）
output, err := bash.ExecuteScript(`
mkdir -p internal/service
cd internal/service
touch user.go
`)
```

### 文件操作
```go
// 写入代码文件
err := bash.WriteCodeFile("main.go", code)
err := bash.WriteCodeFile("internal/service/user.go", serviceCode)

// 读取代码文件
code, err := bash.ReadCodeFile("main.go")
```

### 预设命令
```go
// Go 开发
output, err := bash.RunGoBuild()
output, err := bash.RunGoTest()
output, err := bash.RunGoFmt()

// 文件管理
files, err := bash.ListFiles()
```

### 审计
```go
history := bash.GetHistory()
for _, record := range history {
    fmt.Printf("[%s] %s - %v\n", record.Time, record.Command, record.Success)
}
```

## 使用示例

### 场景 1: Developer 写代码
```go
func (s *DeveloperStaff) implementFeature(task protocol.Task, ...) {
    // 创建 Bash 工具
    bash := tools.NewStaffBashTool(
        "./workspaces",
        "BlogSystem",
        "17727282",
        "04-develop",
    )
    
    // 1. 写入主程序
    bash.WriteCodeFile("main.go", generatedMainCode)
    
    // 2. 写入服务层
    bash.WriteCodeFile("internal/service/post.go", serviceCode)
    
    // 3. 格式化
    bash.RunGoFmt()
    
    // 4. 编译验证
    if _, err := bash.RunGoBuild(); err != nil {
        // 编译失败，返回错误
        resultChan <- TaskResult{Error: err.Error()}
        return
    }
    
    // 5. 运行测试
    bash.RunGoTest()
}
```

### 场景 2: Tester 执行测试
```go
func (s *TesterStaff) executeTest(task protocol.Task, ...) {
    bash := tools.NewStaffBashTool(..., "05-test", ...)
    
    // 写入测试文件
    bash.WriteCodeFile("integration_test.go", testCode)
    
    // 执行测试
    output, err := bash.Execute("go test -v -run Integration")
    
    // 解析结果
    passed := err == nil && strings.Contains(output, "PASS")
}
```

## 目录结构

```
workspaces/
└── BlogSystem-17727282/
    ├── 01-requirement/
    │   └── PRD.md
    ├── 04-develop/              ← Developer 工作目录
    │   ├── main.go
    │   ├── go.mod
    │   ├── internal/
    │   │   ├── service/
    │   │   │   └── user.go
    │   │   └── model/
    │   │       └── user.go
    │   └── app                  ← 编译输出
    ├── 05-test/
    │   └── test_report.json
    └── project.json
```

## 安全测试

```bash
# 尝试危险操作（应该被拒绝）
bash.Execute("sudo ls")           # Error: command 'sudo' is blocked
bash.Execute("rm -rf /")          # Error: dangerous pattern
bash.Execute("cat /etc/passwd")   # Error: dangerous pattern
bash.Execute("cd ../../..")      # 实际仍在工作目录内

# 允许的操作
bash.Execute("go build")          # OK
bash.Execute("git status")        # OK
bash.Execute("mkdir test")        # OK
```

## 后续扩展

1. **容器隔离**: 在 Docker 容器中执行，完全隔离
2. **网络限制**: 只允许访问特定域名
3. **资源监控**: CPU/内存限制
4. **代码审查**: 执行前静态分析
5. **沙箱集成**: 使用 Firejail 等沙箱工具

## 实现状态

- [x] 基础 Bash 执行
- [x] 命令白名单/黑名单
- [x] 目录隔离
- [x] 审计日志
- [x] 文件读写
- [x] Go 开发预设命令
- [ ] 容器隔离（可选）
- [ ] 网络限制（可选）
