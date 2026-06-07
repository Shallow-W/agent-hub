# CLAUDE.md

开始任何任务之前，先阅读 `AGENTS.md` 获取项目上下文、技术栈。
编码规范和核心规则由 Trellis 通过 `.trellis/spec/` 自动注入，无需在此重复。

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 脚本命令

所有构建、测试、启动操作必须通过 `scripts/` 目录下的脚本执行，不要手动 cd 到子目录再运行命令。

| 命令 | 用途 |
|------|------|
| `bash scripts/build.sh` | 构建后端二进制 + 前端产物 |
| `bash scripts/test.sh` | 运行后端 Go 测试 |
| `bash scripts/dev.sh` | 启动开发环境（PostgreSQL + 后端 + 前端） |

详细说明见 `scripts/README.md`。
