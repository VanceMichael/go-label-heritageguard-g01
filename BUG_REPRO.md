# Bug Reproduction

## 包的性质

当前 tasks/heritageguard-0018/green 保存的是被测模型修复后的结果源码，不是初始含 Bug 源码。最终 baseline 分支已在下面固定的 parent SHA 上单独提交任务测试，可直接复核 red；tasks/heritageguard-0018/green 包含同一测试并应得到 green。完整验证日志仍只在本地留存。

## 问题现象

两个主管并发审批同一件藏品的不同借展申请时，藏品只能绑定一个活动借展；请修复审批所有权竞争。

## 含 Bug 版本

- 仓库：VanceMichael/go-label-heritageguard-g01
- 仓库地址：https://github.com/VanceMichael/go-label-heritageguard-g01.git
- parent SHA：1bc142c0133069ddb35882d181805a6984a61056

## 复现步骤

```bash
git clone -- https://github.com/VanceMichael/go-label-heritageguard-g01.git bug-repro
cd bug-repro
git checkout --detach 1bc142c0133069ddb35882d181805a6984a61056
go test ./internal/loan -run '^TestHeritageGuardTask0018$' -count=1
```

## 双架构完整错误信息

### linux/amd64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/loan -run '^TestHeritageGuardTask0018$' -count=1
--- FAIL: TestHeritageGuardTask0018 (0.07s)
    task_0018_test.go:54: artifact approval ownership was not exclusive: successes=2
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/loan	0.135s
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
$ go test ./internal/loan -run '^TestHeritageGuardTask0018$' -count=1
--- FAIL: TestHeritageGuardTask0018 (0.00s)
    task_0018_test.go:54: artifact approval ownership was not exclusive: successes=2
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/loan	0.007s
FAIL

```

stderr：

```text
(empty)
```

## 通过条件

并发审批必须只有一个成功且 artifact 只绑定一个 active loan，失败方必须观察到版本或所有权冲突。
