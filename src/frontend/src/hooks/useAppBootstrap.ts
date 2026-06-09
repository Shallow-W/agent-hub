import { useEffect } from 'react';
import { useAgentStore } from '@/store/agentStore';

type BootstrapTask = () => Promise<void>;

function runBootstrapTasks(tasks: BootstrapTask[]): void {
  tasks.forEach((task) => {
    void task().catch(() => {});
  });
}

/**
 * 初始化登录后全局元数据。
 *
 * 聊天消息、会话列表、任务看板等页面都会消费 Agent 展示信息，
 * 因此放在受保护布局统一加载，避免某个子页面先渲染出降级头像。
 */
export function useAppBootstrap(): void {
  const fetchAgents = useAgentStore((s) => s.fetchAgents);

  useEffect(() => {
    runBootstrapTasks([
      () => fetchAgents(),
    ]);
  }, [fetchAgents]);
}
