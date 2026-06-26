#!/usr/bin/env bash
set -euo pipefail

# 发布 @hust-agenthub/daemon 到 npm
# 用法:
#   bash scripts/publish-daemon.sh          # 正常发布（需要 npm 2FA OTP）
#   bash scripts/publish-daemon.sh --dry-run # 试运行，不实际发布

cd "$(git rev-parse --show-toplevel)/src/daemon-npm"

# 检查是否已登录
if ! npm whoami &>/dev/null; then
  echo "错误: 未登录 npm，请先运行 npm login"
  exit 1
fi

# 检查 package.json 是否存在
if [ ! -f package.json ]; then
  echo "错误: 找不到 package.json"
  exit 1
fi

NAME=$(node -p "require('./package.json').name")
VERSION=$(node -p "require('./package.json').version")

echo "包名: $NAME"
echo "版本: $VERSION"

# 打包预览
npm pack --dry-run 2>&1

if [ "${1:-}" = "--dry-run" ]; then
  echo ""
  echo "[dry-run] 以上是发布内容预览，未实际发布"
  exit 0
fi

# 发布
echo ""
echo "发布到 npm (access: public) ..."
npm publish --access public

echo ""
echo "发布成功: $NAME@$VERSION"
echo "远程电脑安装命令: npx $NAME --server-url <URL> --api-key <KEY>"
