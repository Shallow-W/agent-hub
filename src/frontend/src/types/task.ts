export type TaskStatus = 'todo' | 'in_progress' | 'blocked' | 'done';

export type TaskPriority = 'low' | 'medium' | 'high';

export interface WorkspaceTask {
  id: string;
  user_id: string;
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
