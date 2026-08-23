# Bug Reproduction

## 包的性质

当前 tasks/heritageguard-0002/green 保存的是被测模型修复后的结果源码，不是初始含 Bug 源码。最终 baseline 分支已在下面固定的 parent SHA 上单独提交任务测试，可直接复核 red；tasks/heritageguard-0002/green 包含同一测试并应得到 green。完整验证日志仍只在本地留存。

## 问题现象

环境评估请求在加载会话主体期间被取消后，认证不应继续访问用户数据或返回主体；请修复取消传播。

## 含 Bug 版本

- 仓库：VanceMichael/go-label-heritageguard-g01
- 仓库地址：https://github.com/VanceMichael/go-label-heritageguard-g01.git
- parent SHA：ea405233b80edac2849390771b263c30f447a00c

## 复现步骤

```bash
git clone -- https://github.com/VanceMichael/go-label-heritageguard-g01.git bug-repro
cd bug-repro
git checkout --detach ea405233b80edac2849390771b263c30f447a00c
go test ./internal/auth -run '^TestHeritageGuardTask0002$' -count=1
```

## 双架构完整错误信息

### linux/amd64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/auth -run '^TestHeritageGuardTask0002$' -count=1
--- FAIL: TestHeritageGuardTask0002 (0.00s)
    task_0002_test.go:38: cancelled authentication must stop before loading the user, got <nil>
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/auth	0.052s
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
$ go test ./internal/auth -run '^TestHeritageGuardTask0002$' -count=1
--- FAIL: TestHeritageGuardTask0002 (0.00s)
    task_0002_test.go:38: cancelled authentication must stop before loading the user, got <nil>
FAIL
FAIL	github.com/VanceMichael/go-base-heritageguard-g01/internal/auth	0.003s
FAIL

```

stderr：

```text
(empty)
```

## 通过条件

取消发生在会话读取与用户读取之间时必须返回 context.Canceled 且不能调用用户仓库，未取消的有效 token 认证仍须成功。
