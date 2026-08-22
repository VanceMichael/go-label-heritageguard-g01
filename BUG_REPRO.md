# Bug Reproduction

## 包的性质

当前 tasks/heritageguard-0010/green 保存的是被测模型修复后的结果源码，不是初始含 Bug 源码。最终 baseline 分支已在下面固定的 parent SHA 上单独提交任务测试，可直接复核 red；tasks/heritageguard-0010/green 包含同一测试并应得到 green。完整验证日志仍只在本地留存。

## 问题现象

环境评估在读取窗口期间取消后，系统不能把部分读数当成完整放行决定；请修复评估的取消处理。

## 含 Bug 版本

- 仓库：VanceMichael/go-label-heritageguard-g01
- 仓库地址：https://github.com/VanceMichael/go-label-heritageguard-g01.git
- parent SHA：2601dcd7248b05914cdf56e870d1a8e2ba04493a

## 复现步骤

```bash
git clone -- https://github.com/VanceMichael/go-label-heritageguard-g01.git bug-repro
cd bug-repro
git checkout --detach 2601dcd7248b05914cdf56e870d1a8e2ba04493a
go test ./internal/exhibition -run '^TestHeritageGuardTask0010$' -count=1
```

## 双架构完整错误信息

### linux/amd64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/exhibition -run '^TestHeritageGuardTask0010$' -count=1
--- FAIL: TestHeritageGuardTask0010 (0.00s)
    task_0010_test.go:36: cancelled assessment must not produce a readiness decision, got <nil>
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/exhibition	0.053s
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
$ go test ./internal/exhibition -run '^TestHeritageGuardTask0010$' -count=1
--- FAIL: TestHeritageGuardTask0010 (0.00s)
    task_0010_test.go:36: cancelled assessment must not produce a readiness decision, got <nil>
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/exhibition	0.003s
FAIL

```

stderr：

```text
(empty)
```

## 通过条件

取消的评估必须返回 context.Canceled 并且不产生 Ready 决定，完整窗口的评估结果必须仍按读数计算。
