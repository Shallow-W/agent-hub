/**
 * 文件浏览 API —— 让前端抽屉浏览 agent 所在 daemon 机器上的文件。
 *
 * 解耦设计（见 ProjectCard）：workDir 来自 agent 上报的 project 卡片，
 * 是路径生产与文件浏览之间的唯一契约。所有调用都必须传 workDir。
 *
 * 数据流：前端 → GET /api/agents/:id/files/browse?work_dir=&action=&path=&rev=
 *         → 后端同步 RPC → daemon 读文件/git。
 * 三层契约（daemon handler 返回结构）直接在这里类型化，后端只透传不解析。
 */
import { get, getAuthHeaders, ApiError } from './client';

/** 单层目录条目。type=dir 时可继续懒加载展开。 */
export interface FileEntry {
  name: string;
  type: 'file' | 'dir';
  size?: number;
}

/** git 改动文件状态（对齐 daemon parseGitStatus）。 */
export type ChangeStatus = 'added' | 'modified' | 'deleted';

/** git 改动文件项。path 是相对 repoRoot 的 posix 相对路径。 */
export interface ChangedFile {
  path: string;
  status: ChangeStatus;
}

/** action=tree 返回：根目录快照 + git 改动清单。 */
export interface TreeResult {
  /** 仓库根目录绝对路径（前端展示 + 作为后续 list/read 的基准）。 */
  repoRoot: string;
  /** 是否 git 仓库。非 git 仓库时 changedFiles 为空，只能裸浏览目录。 */
  isGit: boolean;
  changedFiles: ChangedFile[];
  /** 根目录一层条目。 */
  rootEntries: FileEntry[];
}

/** action=read / show 返回：单文件内容。 */
export interface ReadResult {
  path: string;
  content: string;
  size: number;
  /** 二进制文件（含 NUL 字节），前端应提示不支持预览。 */
  binary: boolean;
  /** 文件过大（>2MB），content 为空。 */
  tooLarge?: boolean;
}

/** git commit 元信息（action=log 返回）。 */
export interface Commit {
  hash: string;
  /** unix 秒。 */
  timestamp: number;
  author: string;
  subject: string;
}

/** 拉根目录快照（打开抽屉时调）。 */
export function browseTree(agentId: string, workDir: string, rev?: string): Promise<TreeResult> {
  const qs = buildQuery({ work_dir: workDir, action: 'tree', rev });
  return get<TreeResult>(`/api/agents/${agentId}/files/browse?${qs}`);
}

/** 单层展开子目录（懒加载）。path 必须是相对 repoRoot 的路径或绝对路径。 */
export function listDir(agentId: string, workDir: string, targetPath: string): Promise<FileEntry[]> {
  const qs = buildQuery({ work_dir: workDir, action: 'list', path: targetPath });
  return get<FileEntry[]>(`/api/agents/${agentId}/files/browse?${qs}`);
}

/** 读单文件内容（点文件查看时调）。 */
export function readFile(agentId: string, workDir: string, targetPath: string): Promise<ReadResult> {
  const qs = buildQuery({ work_dir: workDir, action: 'read', path: targetPath });
  return get<ReadResult>(`/api/agents/${agentId}/files/browse?${qs}`);
}

/**
 * 下载整目录 zip。后端 action=zip 时返回 application/zip 二进制流（不经 ApiResponse 包装），
 * 所以这里不走 client.get（它强制 res.json()），而是单独 fetch 拿 blob。
 */
export async function downloadZip(agentId: string, workDir: string, targetPath: string): Promise<Blob> {
  const qs = buildQuery({ work_dir: workDir, action: 'zip', path: targetPath });
  const res = await fetch(`/api/agents/${agentId}/files/browse?${qs}`, {
    headers: { ...getAuthHeaders() },
  });
  if (!res.ok) {
    let msg = `下载失败 (${res.status})`;
    try {
      const body = await res.json();
      if (body?.message) msg = body.message;
    } catch { /* 非 JSON 错误体，用默认 msg */ }
    throw new ApiError(res.status, 0, msg);
  }
  return res.blob();
}

/**
 * 下载单文件。复用 readFile 拿文本内容，前端用 Blob + a.download 触发下载。
 * 二进制/超大文件直接走后端 read（content 为空）—— 这种情况提示用户用整目录 zip。
 */
export async function downloadFile(
  agentId: string,
  workDir: string,
  targetPath: string,
): Promise<{ blob: Blob; filename: string } | null> {
  const result = await readFile(agentId, workDir, targetPath);
  if (result.binary || result.tooLarge) return null;
  const blob = new Blob([result.content], { type: 'text/plain;charset=utf-8' });
  const filename = targetPath.split('/').pop() || 'download';
  return { blob, filename };
}

/** 拉某文件的 git 历史（commit 列表，最新在前）。供前端版本切换。 */
export function fileHistory(agentId: string, workDir: string, targetPath: string): Promise<Commit[]> {
  const qs = buildQuery({ work_dir: workDir, action: 'log', path: targetPath });
  return get<{ commits: Commit[] }>(`/api/agents/${agentId}/files/browse?${qs}`).then((r) => r.commits);
}

/** 读某 commit 下某文件的内容（git show rev:path）。 */
export function fileShow(
  agentId: string,
  workDir: string,
  targetPath: string,
  rev: string,
): Promise<ReadResult> {
  const qs = buildQuery({ work_dir: workDir, action: 'show', path: targetPath, rev });
  return get<ReadResult>(`/api/agents/${agentId}/files/browse?${qs}`);
}

/** 查多个文件的 git 状态（added/modified/deleted）。供 diff 卡片展示文件清单。 */
export function fileStatus(
  agentId: string,
  workDir: string,
  files: string[],
): Promise<{ path: string; status: ChangeStatus }[]> {
  // files 数组用逗号拼接传 query（避免多次请求）
  const qs = buildQuery({ work_dir: workDir, action: 'status', files: files.join(',') });
  return get<{ statuses: { path: string; status: ChangeStatus }[] }>(
    `/api/agents/${agentId}/files/browse?${qs}`,
  ).then((r) => r.statuses);
}

/** 单个文件的前后内容（fileDiff 返回）。供 DiffCard 预取缓存 / DiffViewer 直接渲染。 */
export interface DiffContent {
  oldContent: string;
  newContent: string;
}

/** 拿某文件的前后内容（默认工作区 vs HEAD）。供 diff 卡片点击文件后做对比。 */
export function fileDiff(
  agentId: string,
  workDir: string,
  targetPath: string,
  oldRev?: string,
  newRev?: string,
): Promise<DiffContent> {
  const qs = buildQuery({ work_dir: workDir, action: 'diff', path: targetPath, old_rev: oldRev, new_rev: newRev });
  return get<DiffContent>(
    `/api/agents/${agentId}/files/browse?${qs}`,
  );
}

/** 构造 query string，跳过空值。 */
function buildQuery(params: Record<string, string | undefined>): string {
  const parts: string[] = [];
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') {
      parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(value)}`);
    }
  }
  return parts.join('&');
}
