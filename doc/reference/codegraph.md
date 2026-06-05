# Codegraph CLI 用法

项目已集成 Codegraph（SQLite 代码知识图谱），可通过 MCP 工具直接查询符号、调用关系和代码流程。
索引由文件监听器自动同步，无需手动维护。

当前索引规模：203 文件 / 3058 符号节点 / 6667 条边（Go + TypeScript + TSX + Python）。

## 常用场景

| 场景 | 工具 | 示例 |
|------|------|------|
| 查某个符号是什么 | `codegraph_search` | query=`"handleMessage"` |
| 了解某个模块/功能全貌 | `codegraph_context` | task=`"WebSocket 消息分发"` |
| 追踪调用链 X→Y | `codegraph_trace` | from=`"handleMessage"`, to=`"saveToDB"` |
| 谁调用了这个函数 | `codegraph_callers` | symbol=`"ChatHandler.ServeWS"` |
| 这个函数调用了谁 | `codegraph_callees` | symbol=`"SendMessage"` |
| 改这个符号会影响什么 | `codegraph_impact` | symbol=`"UserStore"` |
| 查看符号源码/签名 | `codegraph_node` | symbol=`"ChatHandler"`, includeCode=true |
| 批量查看多个符号源码 | `codegraph_explore` | query=`"ChatHandler SendMessage Hub"` |
| 浏览目录结构 | `codegraph_files` | path=`"src/backend/internal"` |
| 索引健康状态 | `codegraph_status` | — |

## 推荐查询链路

1. **不了解某模块** → 先 `codegraph_context` 获取全景（自动组合 search + node + callers + callees）
2. **需要改某个函数** → 先 `codegraph_impact` 评估影响范围
3. **排查 bug / 理解流程** → `codegraph_trace` 一步拿到完整调用链

## 注意事项

- 索引通过文件监听器实时更新，延迟约 1 秒
- 对于动态分发（接口/回调），`codegraph_trace` 会标注断点并列出候选实现
- 优先使用 `codegraph_context` / `codegraph_explore` 等组合工具，避免多次单独 `codegraph_node` 调用
