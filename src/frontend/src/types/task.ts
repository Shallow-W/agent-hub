export type TaskStatus = 'todo' | 'in_progress' | 'blocked' | 'done' | 'cancelled';

export type TaskPriority = 'low' | 'medium' | 'high';

export interface WorkspaceTask {
  id: string;
  user_id?: string | null;
  conversation_id?: string;
  assignee_id?: string;
  agent_id?: string;
  title: string;
  description: string;
  status: TaskStatus;
  priority: TaskPriority;
  created_at: string;
  updated_at: string;
  assignee_name?: string;
  agent_name?: string;
  orch_task_id?: string;
  worker_name?: string;
  task_hash?: string;
  worker_result?: string;
  completed_at?: string;
}

export interface OrchTaskCard {
  id: string;
  conversation_id: string;
  orch_task_id: string;
  sender_id: string;
  sender_name: string;
  sender_avatar: string;
  worker_id: string;
  worker_name: string;
  worker_avatar: string;
  task_content: string;
  task_summary: string;
  worker_result: string;
  status: 'todo' | 'in_progress' | 'done' | 'failed';
  priority: string;
  task_hash: string;
  dispatched_at: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface TaskQuery {
  conversation_id?: string;
  status?: TaskStatus;
}

export interface CreateTaskPayload {
  title: string;
  description?: string;
  status?: TaskStatus;
  priority?: TaskPriority;
  conversation_id?: string;
  assignee_id?: string;
  agent_id?: string;
}

export interface UpdateTaskPayload {
  title?: string;
  description?: string;
  priority?: TaskPriority;
  assignee_id?: string;
  agent_id?: string;
}
