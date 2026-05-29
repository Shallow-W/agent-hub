import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Dropdown,
  Empty,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Tag,
  message as antMessage,
} from 'antd';
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  DeleteOutlined,
  ExclamationCircleOutlined,
  FolderOutlined,
  MoreOutlined,
  PlusOutlined,
  ReloadOutlined,
  TeamOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { createTask, deleteTask, getTasks, moveTaskStatus, updateTask } from '@/api/task';
import { useConversationStore } from '@/store/conversationStore';
import type { CreateTaskPayload, TaskPriority, TaskStatus, WorkspaceTask } from '@/types/task';
import styles from './TaskBoardView.module.css';

interface TaskFormValues {
  title: string;
  description?: string;
  priority: TaskPriority;
  status: TaskStatus;
}

interface TaskColumn {
  key: TaskStatus;
  title: string;
  icon: React.ReactNode;
}

const columns: TaskColumn[] = [
  { key: 'todo', title: '已派发', icon: <ClockCircleOutlined /> },
  { key: 'in_progress', title: '正在执行', icon: <ThunderboltOutlined /> },
  { key: 'blocked', title: '待处理', icon: <ExclamationCircleOutlined /> },
  { key: 'done', title: '完成/已验收', icon: <CheckCircleOutlined /> },
];

const priorityLabels: Record<TaskPriority, string> = {
  low: '低优先级',
  medium: '中优先级',
  high: '高优先级',
};

const nextStatus: Record<TaskStatus, TaskStatus | null> = {
  todo: 'in_progress',
  in_progress: 'done',
  blocked: 'in_progress',
  done: null,
};

const TaskBoardView: React.FC = () => {
  const [tasks, setTasks] = useState<WorkspaceTask[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<WorkspaceTask | null>(null);
  const [form] = Form.useForm<TaskFormValues>();
  const activeConversationId = useConversationStore((s) => s.activeConversationId);
  const conversations = useConversationStore((s) => s.conversations);
  const setActiveConversation = useConversationStore((s) => s.setActive);
  const activeConversation = conversations.find((item) => item.id === activeConversationId);

  const grouped = useMemo(() => {
    const map = new Map<TaskStatus, WorkspaceTask[]>();
    columns.forEach((column) => map.set(column.key, []));
    tasks.forEach((task) => {
      map.get(task.status)?.push(task);
    });
    return map;
  }, [tasks]);

  const fetchTasks = useCallback(async () => {
    if (!activeConversationId) {
      setTasks([]);
      return;
    }
    setLoading(true);
    try {
      setTasks(await getTasks({ conversation_id: activeConversationId }));
    } catch {
      antMessage.error('获取对话任务失败');
    } finally {
      setLoading(false);
    }
  }, [activeConversationId]);

  useEffect(() => {
    fetchTasks();
  }, [fetchTasks]);

  useEffect(() => {
    const firstConversation = conversations[0];
    if (!activeConversationId && firstConversation) {
      setActiveConversation(firstConversation.id);
    }
  }, [activeConversationId, conversations, setActiveConversation]);

  useEffect(() => {
    if (!modalOpen) return;
    if (editingTask) {
      form.setFieldsValue({
        title: editingTask.title,
        description: editingTask.description,
        priority: editingTask.priority,
        status: editingTask.status,
      });
      return;
    }
    form.setFieldsValue({
      title: '',
      description: '',
      priority: 'medium',
      status: 'todo',
    });
  }, [editingTask, form, modalOpen]);

  const openCreate = () => {
    if (!activeConversationId) {
      antMessage.warning('请先在左侧选择一个对话');
      return;
    }
    setEditingTask(null);
    setModalOpen(true);
  };

  const openEdit = (task: WorkspaceTask) => {
    setEditingTask(task);
    setModalOpen(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      if (editingTask) {
        const updated = await updateTask(editingTask.id, {
          title: values.title,
          description: values.description ?? '',
          priority: values.priority,
        });
        setTasks((prev) => prev.map((task) => (task.id === updated.id ? updated : task)));
        antMessage.success('任务已更新');
      } else {
        const payload: CreateTaskPayload = {
          title: values.title,
          description: values.description ?? '',
          priority: values.priority,
          status: values.status,
          conversation_id: activeConversationId ?? undefined,
        };
        const created = await createTask(payload);
        setTasks((prev) => [created, ...prev]);
        antMessage.success('任务已添加到当前对话');
      }
      setModalOpen(false);
    } catch (err) {
      if (err instanceof Error) {
        antMessage.error(err.message);
      }
    }
  };

  const handleMove = async (task: WorkspaceTask, status: TaskStatus) => {
    try {
      const updated = await moveTaskStatus(task.id, status);
      setTasks((prev) => prev.map((item) => (item.id === updated.id ? updated : item)));
    } catch {
      antMessage.error('状态流转失败');
    }
  };

  const handleDelete = async (task: WorkspaceTask) => {
    try {
      await deleteTask(task.id);
      setTasks((prev) => prev.filter((item) => item.id !== task.id));
      antMessage.success('任务已删除');
    } catch {
      antMessage.error('删除任务失败');
    }
  };

  const renderTaskCard = (task: WorkspaceTask) => {
    const targetStatus = nextStatus[task.status];
    return (
      <article className={styles.taskCard} key={task.id}>
        <div className={styles.cardHeader}>
          <div className={styles.cardTitle}>{task.title}</div>
          <Dropdown
            trigger={['click']}
            menu={{
              items: [
                { key: 'edit', label: '编辑任务' },
                { key: 'blocked', label: '标记待处理', disabled: task.status === 'blocked' },
                { key: 'delete', label: '删除任务', danger: true },
              ],
              onClick: ({ key }) => {
                if (key === 'edit') openEdit(task);
                if (key === 'blocked') handleMove(task, 'blocked');
                if (key === 'delete') handleDelete(task);
              },
            }}
          >
            <button className={styles.iconButton} type="button" aria-label="任务操作">
              <MoreOutlined />
            </button>
          </Dropdown>
        </div>
        {task.description && <p className={styles.cardDescription}>{task.description}</p>}
        <div className={styles.cardMeta}>
          <Tag className={styles.priorityTag}>{priorityLabels[task.priority]}</Tag>
          {task.agent_name && <span>{task.agent_name}</span>}
          {task.assignee_name && <span>{task.assignee_name}</span>}
        </div>
        <div className={styles.cardFooter}>
          <span>{new Date(task.updated_at).toLocaleString()}</span>
          {targetStatus ? (
            <button className={styles.moveButton} type="button" onClick={() => handleMove(task, targetStatus)}>
              推进
            </button>
          ) : (
            <button className={styles.deleteButton} type="button" onClick={() => handleDelete(task)}>
              <DeleteOutlined />
            </button>
          )}
        </div>
      </article>
    );
  };

  const emptyDescription = activeConversationId ? '当前对话暂无任务' : '请先在左侧选择一个对话';

  return (
    <section className={styles.page}>
      <header className={styles.workspaceHeader}>
        <div className={styles.workspaceIcon}><TeamOutlined /></div>
        <div className={styles.workspaceMeta}>
          <h1 className={styles.workspaceTitle}>{activeConversation?.title ?? '当前对话任务'}</h1>
          <div className={styles.workspaceSub}>对话任务 · 后续可集成到群聊任务入口</div>
        </div>
        <Space className={styles.headerActions}>
          <Button size="small" icon={<ReloadOutlined />} loading={loading} onClick={fetchTasks}>
            刷新
          </Button>
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            添加任务
          </Button>
        </Space>
      </header>

      <nav className={styles.tabs} aria-label="对话任务">
        <button className={`${styles.tab} ${styles.tabActive}`} type="button">
          <FolderOutlined /> 任务看板
        </button>
      </nav>

      <div className={styles.board}>
        {columns.map((column) => {
          const columnTasks = grouped.get(column.key) ?? [];
          return (
            <section className={styles.column} key={column.key}>
              <div className={styles.columnHeader}>
                <span className={styles.columnTitle}>{column.icon}{column.title}</span>
                <span className={styles.count}>{columnTasks.length}</span>
              </div>
              <div className={styles.taskList}>
                {columnTasks.length > 0 ? columnTasks.map(renderTaskCard) : (
                  <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyDescription} />
                )}
              </div>
            </section>
          );
        })}
      </div>

      <Modal
        title={editingTask ? '编辑任务' : '添加任务'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={handleSubmit}
        destroyOnHidden
        forceRender
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item
            label="任务标题"
            name="title"
            rules={[{ required: true, message: '请输入任务标题' }, { max: 120, message: '最多 120 个字符' }]}
          >
            <Input placeholder="例如：整理本轮对话中的实现任务" />
          </Form.Item>
          <Form.Item label="任务描述" name="description">
            <Input.TextArea rows={4} placeholder="补充目标、上下文或验收标准" />
          </Form.Item>
          <div className={styles.formGrid}>
            <Form.Item label="优先级" name="priority" rules={[{ required: true }]}>
              <Select
                options={[
                  { value: 'low', label: '低' },
                  { value: 'medium', label: '中' },
                  { value: 'high', label: '高' },
                ]}
              />
            </Form.Item>
            <Form.Item label="初始状态" name="status" rules={[{ required: true }]}>
              <Select
                disabled={Boolean(editingTask)}
                options={[
                  { value: 'todo', label: '已派发' },
                  { value: 'in_progress', label: '正在执行' },
                  { value: 'blocked', label: '待处理' },
                  { value: 'done', label: '完成/已验收' },
                ]}
              />
            </Form.Item>
          </div>
        </Form>
      </Modal>
    </section>
  );
};

export default TaskBoardView;
