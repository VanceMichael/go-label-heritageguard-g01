# Bug Reproduction

## 包的性质

当前 tasks/heritageguard-0016/green 保存的是被测模型修复后的结果源码，不是初始含 Bug 源码。最终 baseline 分支已在下面固定的 parent SHA 上单独提交任务测试，可直接复核 red；tasks/heritageguard-0016/green 包含同一测试并应得到 green。完整验证日志仍只在本地留存。

## 问题现象

展柜 incident 结案时如果恢复展柜失败，incident 不能已经变成 closed；请修复结案事务。

## 含 Bug 版本

- 仓库：VanceMichael/go-label-heritageguard-g01
- 仓库地址：https://github.com/VanceMichael/go-label-heritageguard-g01.git
- parent SHA：ed5f5e9e39fa73720e94aebf128fbafa55c06d7e

## 复现步骤

```bash
git clone -- https://github.com/VanceMichael/go-label-heritageguard-g01.git bug-repro
cd bug-repro
git checkout --detach ed5f5e9e39fa73720e94aebf128fbafa55c06d7e
go test ./internal/exhibition -run '^TestHeritageGuardTask0016$' -count=1
```

## 双架构完整错误信息

### linux/amd64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/exhibition -run '^TestHeritageGuardTask0016$' -count=1
--- FAIL: TestHeritageGuardTask0016 (0.07s)
    task_0016_test.go:46: failed close left incident and case split: incident=closed case=incident
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/exhibition	0.120s
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
$ go test ./internal/exhibition -run '^TestHeritageGuardTask0016$' -count=1
--- FAIL: TestHeritageGuardTask0016 (0.00s)
    task_0016_test.go:46: failed close left incident and case split: incident=closed case=incident
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/exhibition	0.006s
FAIL

```

stderr：

```text
(empty)
```

## 通过条件

展柜恢复失败时 incident 必须仍为 monitoring、展柜仍处置状态，正常 remediation 关闭必须同时恢复展柜。
