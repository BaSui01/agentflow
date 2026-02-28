# Git 分批提交示例

## 示例 1：前端功能开发分批提交

假设开发一个用户登录功能，包含多个文件：

```bash
# 1. 创建新分支
git checkout -b feature/user-login-20240115

# 2. 第一批：提交基础组件
git add src/components/LoginForm.vue
git add src/components/LoginForm.css
git commit -m "feat: 添加登录表单组件"

# 3. 第二批：提交API相关
git add src/api/auth.js
git add src/utils/token.js
git commit -m "feat: 添加用户认证API和令牌工具"

# 4. 第三批：提交状态管理
git add src/store/user.js
git commit -m "feat: 添加用户状态管理模块"

# 5. 第四批：提交页面和路由
git add src/views/Login.vue
git add src/router/index.js
git commit -m "feat: 添加登录页面和路由配置"

# 6. 切换到目标分支并合并
git checkout dev
git pull origin dev
git merge --no-ff feature/user-login-20240115 -m "merge: 合并用户登录功能"

# 7. 删除本地临时分支
git branch -d feature/user-login-20240115

# 8. 推送到远程
git push origin dev
```

## 示例 2：Bug 修复分批提交

```bash
# 1. 创建修复分支
git checkout -b hotfix/fix-cart-calculation

# 2. 提交核心修复
git add src/utils/calculator.js
git commit -m "fix: 修复购物车金额计算精度问题"

# 3. 提交相关测试
git add tests/calculator.test.js
git commit -m "test: 添加金额计算单元测试"

# 4. 提交文档更新
git add docs/CHANGELOG.md
git commit -m "docs: 更新修复日志"

# 5. 合并到 main 分支
git checkout main
git pull origin main
git merge --no-ff hotfix/fix-cart-calculation -m "merge: 合并购物车金额计算修复"

# 6. 清理分支
git branch -d hotfix/fix-cart-calculation
git push origin main
```

## 示例 3：配置文件批量更新

```bash
# 1. 创建配置更新分支
git checkout -b chore/update-configs

# 2. 提交构建配置
git add webpack.config.js
git add vite.config.js
git commit -m "chore: 更新构建工具配置"

# 3. 提交代码规范配置
git add .eslintrc.js
git add .prettierrc
git commit -m "chore: 更新代码规范配置"

# 4. 提交依赖更新
git add package.json
git add package-lock.json
git commit -m "chore: 更新项目依赖版本"

# 5. 合并到 dev
git checkout dev
git merge --no-ff chore/update-configs -m "merge: 合并配置文件更新"
git branch -d chore/update-configs
```

## 查看合并历史

合并完成后，使用以下命令查看带有合并线的历史：

```bash
git log --oneline --graph --all
```

输出示例：
```
*   a1b2c3d (HEAD -> dev) merge: 合并用户登录功能
|\
| * d4e5f6g feat: 添加登录页面和路由配置
| * g7h8i9j feat: 添加用户状态管理模块
| * j0k1l2m feat: 添加用户认证API和令牌工具
| * m3n4o5p feat: 添加登录表单组件
|/
* p6q7r8s 之前的提交...
```
