# `dev` 分支与自动合并配置说明

本仓库已补充 workflow：`.github/workflows/dev-to-master-auto-merge.yml`

目标：

- 日常开发先合入 `dev`
- 推送到 `dev` 后自动创建/更新 `dev -> master` PR
- 当 `master` 分支要求的 CI / 审查条件全部通过后，自动合并到 `master`

## 你还需要在 GitHub 仓库后台手动打开的设置

这些设置**不能完全靠仓库文件本身代替**，需要在 GitHub 仓库设置中开启：

### 1. 打开仓库 Auto-merge

路径：

- `Settings`
- `General`
- `Pull Requests`
- 勾选 `Allow auto-merge`

### 2. 给 `master` 加保护规则

建议：

- Require a pull request before merging
- Require status checks to pass before merging
- Require branches to be up to date before merging
- Require conversation resolution before merging
- Block force pushes
- Block deletions

建议至少把以下检查设为 required：

- `Quality & Tests`
- `Benchmark`
- `Cross Build (linux/amd64)`
- `Cross Build (linux/arm64)`
- `Cross Build (darwin/amd64)`
- `Cross Build (windows/amd64)`
- `Security Scan`

### 3. 给 `dev` 加保护规则

建议：

- 也要求 PR 合并到 `dev`
- 至少要求 `Quality & Tests`
- 禁止 force push
- 禁止删除

## 推荐分支流

```text
feature/* -> dev -> master
```

说明：

- 功能分支先提 PR 到 `dev`
- `dev` 作为集成分支跑 CI/CD
- `dev` 更新后，workflow 自动维护 `dev -> master`
- `master` 只接收通过保护规则的自动合并

## 注意事项

1. 如果仓库还没有远端 `dev` 分支，请先创建并推送：

```bash
git checkout -b dev
git push -u origin dev
```

2. 自动合并是否真的执行，取决于：

- 仓库是否启用 `Allow auto-merge`
- `master` 是否有 required status checks
- `dev -> master` PR 是否满足所有保护规则

3. 当前 workflow 使用 GitHub 自带 `GITHUB_TOKEN` 创建/更新 PR 并开启 auto-merge，不依赖额外密钥。
