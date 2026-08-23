# Bug Reproduction

## 包的性质

当前 tasks/heritageguard-0008/green 保存的是被测模型修复后的结果源码，不是初始含 Bug 源码。最终 baseline 分支已在下面固定的 parent SHA 上单独提交任务测试，可直接复核 red；tasks/heritageguard-0008/green 包含同一测试并应得到 green。完整验证日志仍只在本地留存。

## 问题现象

只有主管可以批准修复方案，保护修复师尝试批准时方案状态和批准人都必须保持不变；请修复审批权限。

## 含 Bug 版本

- 仓库：VanceMichael/go-label-heritageguard-g01
- 仓库地址：https://github.com/VanceMichael/go-label-heritageguard-g01.git
- parent SHA：75d4e374a28e74ae9a2dd8179562b7fb5dbd9668

## 复现步骤

```bash
git clone -- https://github.com/VanceMichael/go-label-heritageguard-g01.git bug-repro
cd bug-repro
git checkout --detach 75d4e374a28e74ae9a2dd8179562b7fb5dbd9668
go test ./internal/conservation -run '^TestHeritageGuardTask0008$' -count=1
```

## 双架构完整错误信息

### linux/amd64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/conservation -run '^TestHeritageGuardTask0008$' -count=1
--- FAIL: TestHeritageGuardTask0008 (0.07s)
    task_0008_test.go:29: only a supervisor may approve a treatment plan, got <nil>
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/conservation	0.121s
FAIL

```

stderr：

```text
(empty)
```

### linux/arm64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/conservation -run '^TestHeritageGuardTask0008$' -count=1
--- FAIL: TestHeritageGuardTask0008 (0.00s)
    task_0008_test.go:29: only a supervisor may approve a treatment plan, got <nil>
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/conservation	0.006s
FAIL

```

stderr：

```text
(empty)
```

## 通过条件

非主管批准必须返回 forbidden、方案仍为 draft 且 ApprovedBy 为空，主管批准路径必须继续工作。
