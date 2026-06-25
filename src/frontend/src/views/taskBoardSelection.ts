interface GroupConversationLike {
  id: string;
}

export function resolveActiveGroupConversationId(
  groupConversations: readonly GroupConversationLike[],
  activeConversationId?: string | null,
): string | null {
  if (activeConversationId && groupConversations.some((conversation) => conversation.id === activeConversationId)) {
    return activeConversationId;
  }
  return groupConversations[0]?.id ?? null;
}
