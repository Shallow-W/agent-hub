import { del, get, post, put } from './client';
import type {
  CreateTaskPayload,
  TaskQuery,
  TaskStatus,
  UpdateTaskPayload,
  WorkspaceTask,
} from '@/types/task';

function buildTaskQuery(query?: TaskQuery): string {
  const params = new URLSearchParams();
  if (query?.conversation_id) {
    params.set('conversation_id', query.conversation_id);
  }
  if (query?.status) {
    params.set('status', query.status);
  }
  const text = params.toString();
  return text ? `?${text}` : '';
}

export async function getTasks(query?: TaskQuery): Promise<WorkspaceTask[]> {
  const tasks = await get<WorkspaceTask[] | null>(`/api/tasks${buildTaskQuery(query)}`);
  return tasks ?? [];
}

export async function createTask(payload: CreateTaskPayload): Promise<WorkspaceTask> {
  return post<WorkspaceTask>('/api/tasks', payload);
}

export async function updateTask(id: string, payload: UpdateTaskPayload): Promise<WorkspaceTask> {
  return put<WorkspaceTask>(`/api/tasks/${id}`, payload);
}

export async function moveTaskStatus(id: string, status: TaskStatus): Promise<WorkspaceTask> {
  return post<WorkspaceTask>(`/api/tasks/${id}/status`, { status });
}

export async function deleteTask(id: string): Promise<void> {
  return del<void>(`/api/tasks/${id}`);
}
