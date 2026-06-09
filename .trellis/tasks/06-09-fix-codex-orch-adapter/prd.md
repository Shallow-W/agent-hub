# Fix Codex Orchestrator Adapter Session Parity

## Problem

When Codex is used as the group Orchestrator, AgentHub dispatches the task through the daemon legacy `codex exec` path. Unlike Claude Code, that path did not inject the current AgentHub `conversation_id` and `user_id` into the `agenthub-platform` MCP server for the current task. As a result, Codex could call tools such as `list_group_agents`, but tools that rely on the current group context fell back to an empty conversation and failed or returned unrelated data.

The Codex path also extracted the orchestrator system prompt from `context_messages` but did not pass it to the CLI, so Codex missed the core Orchestrator behavior instructions that Claude Code receives.

## Acceptance Criteria

- Codex daemon tasks receive a per-task `agenthub-platform` MCP configuration that includes the current conversation and user context.
- Codex Orchestrator tasks receive the orchestrator system prompt in the prompt payload.
- Claude Code behavior remains unchanged.
- The daemon script remains syntactically valid.

## Scope

Modify only the daemon adapter command construction for Codex/Claude shared MCP argument generation.
