# Bug Reproduction

## 包的性质

当前 tasks/heritageguard-0004/green 保存的是被测模型修复后的结果源码，不是初始含 Bug 源码。最终 baseline 分支已在下面固定的 parent SHA 上单独提交任务测试，可直接复核 red；tasks/heritageguard-0004/green 包含同一测试并应得到 green。完整验证日志仍只在本地留存。

## 问题现象

藏品登记的初检持久化失败时，藏品、首份状况报告和首条保管链都必须回滚；请修复登记流程。

## 含 Bug 版本

- 仓库：VanceMichael/go-label-heritageguard-g01
- 仓库地址：https://github.com/VanceMichael/go-label-heritageguard-g01.git
- parent SHA：967fe570d89384d8fef158560da00c7eb73eca5f

## 复现步骤

```bash
git clone -- https://github.com/VanceMichael/go-label-heritageguard-g01.git bug-repro
cd bug-repro
git checkout --detach 967fe570d89384d8fef158560da00c7eb73eca5f
go test ./internal/conservation -run '^TestHeritageGuardTask0004$' -count=1
```

## 双架构完整错误信息

### linux/amd64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/conservation -run '^TestHeritageGuardTask0004$' -count=1
--- FAIL: TestHeritageGuardTask0004 (0.06s)
    task_0004_test.go:35: failed registration left a partial intake aggregate: artifacts=1 reports=1 custody=1
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/conservation	0.114s
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
$ go test ./internal/conservation -run '^TestHeritageGuardTask0004$' -count=1
--- FAIL: TestHeritageGuardTask0004 (0.00s)
    task_0004_test.go:35: failed registration left a partial intake aggregate: artifacts=1 reports=1 custody=1
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/conservation	0.006s
FAIL

```

stderr：

```text
(empty)
```

## 通过条件

初检写入失败必须让四类登记记录全部不存在，成功登记仍须返回完整的藏品、报告和保管事件。
