---
name: setup-pre-commit
description: 配置预提交钩子。设置 Husky + lint-staged + Prettier + 类型检查 + 测试。使用场景：项目缺少预提交检查、需要提交前自动格式化/测试。
---

# Setup Pre-Commit — 配置预提交钩子

## 安装内容

- **Husky** 预提交钩子
- **lint-staged** 运行 Prettier
- **Prettier** 配置（如缺失）
- **类型检查和测试**脚本

## 步骤

### 1. 检测包管理器

检查 `package-lock.json`（npm）、`pnpm-lock.yaml`（pnpm）、`yarn.lock`（yarn）、`bun.lockb`（bun）。使用已存在的。默认 npm。

### 2. 安装依赖

```bash
npm install --save-dev husky lint-staged prettier
```

### 3. 初始化 Husky

```bash
npx husky init
```

### 4. 创建 `.husky/pre-commit`

```bash
npx lint-staged
npm run typecheck
npm run test
```

适配：用检测到的包管理器替换 npm。如果仓库无 `typecheck` 或 `test` 脚本，省略对应行。

### 5. 创建 `.lintstagedrc`

```json
{
  "*": "prettier --ignore-unknown --write"
}
```

### 6. 创建 `.prettierrc`（如缺失）

```json
{
  "useTabs": false,
  "tabWidth": 2,
  "printWidth": 80,
  "singleQuote": false,
  "trailingComma": "es5",
  "semi": true,
  "arrowParens": "always"
}
```

### 7. 验证

- [ ] `.husky/pre-commit` 存在且可执行
- [ ] `.lintstagedrc` 存在
- [ ] `prepare` 脚本在 package.json 中为 `"husky"`
- [ ] `prettier` 配置存在
- [ ] 运行 `npx lint-staged` 验证

### 8. 提交

暂存所有改动的文件，提交信息：`Add pre-commit hooks (husky + lint-staged + prettier)`
