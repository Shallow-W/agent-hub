# State Management

> How state is managed in this project.

---

## Overview

AgentHub uses Zustand for shared client state. Stores may cache server data, but
components that only consume global metadata must not be responsible for
discovering and loading that metadata on their own.

---

## State Categories

- Local component state: transient UI state such as modal visibility, form draft
  text, and hovered/selected controls.
- Global UI state: cross-component state such as the active conversation and
  unread counters.
- Shared server metadata: user, agent, conversation, friend, task, and knowledge
  metadata cached in Zustand stores.
- URL state: page-level workspace/view identity and deep-linkable selections.
  Major workspaces should have URL representation rather than living only in
  local React state.

---

## When to Use Global State

Use a Zustand store when data is consumed by multiple panels, list items, or
message renderers. Examples: Agent metadata is used by the conversation list,
chat header, message bubbles, task board, contacts, and skill panels, so it is
global state.

Do not hide shared server metadata behind a child page that happens to fetch it.
If a protected layout can render consumers after a browser refresh, initialize
the shared metadata from a layout-level bootstrap hook.

---

## Server State

- Store fetch methods may use loaded flags and in-flight Promise reuse to avoid
  duplicate requests.
- Login, registration, and logout must reset user-scoped stores that contain
  server data from the previous identity.
- Message history should be fetched from the backend source of truth when a
  conversation is opened; delivery/offline queues are separate from history.
- Shared metadata needed for initial render should be loaded by
  `useAppBootstrap` under `AppLayout`.

---

## Server-Pushed Events (WebSocket)

When the server emits a domain event over WS (e.g. `conversation.role_changed`,
`task.changed`), follow the listener-Set pattern in `src/frontend/src/store/wsStore.ts`:

```ts
type RoleChangedListener = (conversationId: string, agentId: string,
  role: string, actorId: string, demotedAgentId?: string) => void;

const roleChangedListeners = new Set<RoleChangedListener>();

export function onConversationRoleChanged(fn: RoleChangedListener): () => void {
  roleChangedListeners.add(fn);
  return () => { roleChangedListeners.delete(fn); };
}

export function notifyConversationRoleChanged(payload: { ... }): void {
  roleChangedListeners.forEach((fn) => fn(payload.conversationId, ...));
}
```

Wire protocol fields are **snake_case** on the wire; the dispatcher in
`hooks/useWebSocket.ts` maps them to the `notifyX` call.

Components subscribe in `useEffect` and return `unsubscribe` as the
cleanup:

```tsx
useEffect(() => {
  const unsubscribe = onConversationRoleChanged((convId) => {
    if (convId === conversationId) {
      fetchMembers();
      onAgentsChanged?.();
    }
  });
  return unsubscribe;
}, [conversationId, fetchMembers, onAgentsChanged]);
```

When introducing a new WS event type:

1. Add `onX` / `notifyX` pair to `wsStore.ts`.
2. Extend `StreamMessage.type` union in `types/message.ts`.
3. Add `case '<event_type>':` to `hooks/useWebSocket.ts` dispatching to `notifyX`.

### When NOT to dedup by actor

If the operator who triggered the event will also receive it, do not
filter by `actor_id === currentUser.id`. Multi-tab scenarios (same user
on two tabs) rely on tab B receiving the event even when the actor is
self. The local re-fetch after the optimistic update plus the WS-driven
re-fetch converge idempotently; the duplicate GET is cheap.

See [`backend/event-broadcaster.md`](../backend/event-broadcaster.md)
for the full contract.

---

## Common Mistakes

- Rendering chat or conversation avatars before Agent metadata is loaded causes
  fallback avatars to appear until another page happens to fetch Agents.
- Keeping major navigation state only in `activeNav` under `/` makes refresh,
  deep links, browser history, and future deployment routing harder. Prefer
  paths such as `/chat`, `/agents`, `/contacts`, `/knowledge`, `/tasks`, and
  `/settings` for major workspaces.
- Polling the server via `setInterval` to detect changes that already arrive as
  WS events. Adds load and hides the WS source-of-truth.
