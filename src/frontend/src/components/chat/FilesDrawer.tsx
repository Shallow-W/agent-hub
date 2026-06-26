import React, { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Drawer, Button, Tooltip, Spin, Empty, Tree, Select, Segmented } from 'antd';
import type { TreeDataNode } from 'antd';
import {
  CodeOutlined,
  FolderOpenOutlined,
  FolderOutlined,
  FileOutlined,
  FileAddOutlined,
  FileExcelOutlined,
  FileExclamationOutlined,
  DownloadOutlined,
  EyeOutlined,
  ReloadOutlined,
  DownOutlined,
  HistoryOutlined,
} from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { ApiError } from '@/api/client';
import { message as antMessage } from '@/utils/message';
import {
  browseTree,
  listDir,
  readFile,
  downloadFile,
  downloadZip,
  fileHistory,
  fileShow,
  type TreeResult,
  type ReadResult,
  type ChangedFile,
  type ChangeStatus,
  type Commit,
} from '@/api/files';
import { WebpageFrame } from './WebpageFrame';
import { defaultFileViewMode, isHtmlPreviewFile, type FileViewMode } from './filePreview';
import styles from './FilesDrawer.module.css';

// CodeMirror 较重，懒加载（与 ArtifactEditor 同款策略）
const CodeSelectView = lazy(() => import('./CodeSelectView'));
// DiffView（@codemirror/merge）懒加载，git 历史版本对比时用
const DiffView = lazy(() => import('./DiffView'));

interface FilesDrawerProps {
  /** 要浏览的 agent ID（agent 必须有 machine_id 且 daemon 在线）。 */
  agentId: string;
  /** 浏览根目录（来自 project 卡片的 workDir，路径生产与文件浏览的唯一契约）。 */
  workDir: string;
  open: boolean;
  onClose: () => void;
}

interface FileNodeExtra {
  /** 该节点在 daemon 机器上的绝对路径（tree key）。 */
  absPath: string;
  isDir: boolean;
}

const STATUS_ICON: Record<ChangeStatus, React.ReactNode> = {
  added: <FileAddOutlined className={styles.iconAdded} />,
  modified: <FileExclamationOutlined className={styles.iconModified} />,
  deleted: <FileExcelOutlined className={styles.iconDeleted} />,
};

const STATUS_LABEL: Record<ChangeStatus, string> = {
  added: '新增',
  modified: '修改',
  deleted: '删除',
};

/** 文件面板的显示模式：普通查看 / git 历史对比。 */
type PanelMode = 'view' | 'history';

/**
 * 文件浏览器抽屉——浏览 agent 所在 daemon 机器上 git 工作区的文件。
 *
 * 解耦：workDir 由调用方（ProjectCard）传入，是 agent 上报的工作目录。
 *
 * 布局：左侧文件树（顶部「改动文件」折叠区 + 下方虚拟化目录树）+ 右侧文件内容面板。
 * 内容面板支持两种模式：
 *   - view：当前工作区内容（markdown/代码/文本/二进制分支）
 *   - history：git 历史对比（选两个 commit，用 DiffView MergeView 渲染）
 * 数据流：browseTree → listDir（逐层展开）→ readFile（查看）/ fileHistory+fileShow（历史）。
 */
export const FilesDrawer: React.FC<FilesDrawerProps> = ({ agentId, workDir, open, onClose }) => {
  const [tree, setTree] = useState<TreeResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [treeData, setTreeData] = useState<TreeDataNode[]>([]);
  const [expandedKeys, setExpandedKeys] = useState<string[]>([]);
  const [selectedAbsPath, setSelectedAbsPath] = useState<string>('');
  const [fileContent, setFileContent] = useState<ReadResult | null>(null);
  const [fileLoading, setFileLoading] = useState(false);
  const [changedCollapsed, setChangedCollapsed] = useState(false);
  const [downloading, setDownloading] = useState(false);

  // git 历史模式状态
  const [panelMode, setPanelMode] = useState<PanelMode>('view');
  const [commits, setCommits] = useState<Commit[]>([]);
  const [commitsLoading, setCommitsLoading] = useState(false);
  const [oldRev, setOldRev] = useState<string>('');
  const [newRev, setNewRev] = useState<string>('');
  const [diffOld, setDiffOld] = useState<ReadResult | null>(null);
  const [diffNew, setDiffNew] = useState<ReadResult | null>(null);
  const [diffLoading, setDiffLoading] = useState(false);
  const [fileViewMode, setFileViewMode] = useState<FileViewMode>('source');

  // 文件树虚拟化的可见高度（用 ref 测量，避免固定值在小屏上滚动条计算不准）
  const treeContainerRef = useRef<HTMLDivElement>(null);
  const [treeHeight, setTreeHeight] = useState(480);
  useEffect(() => {
    if (!treeContainerRef.current) return;
    const el = treeContainerRef.current;
    const update = () => setTreeHeight(Math.max(el.clientHeight - 40, 200));
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, [open]);

  // 打开时拉根目录
  const loadTree = useCallback(() => {
    if (!open || !agentId || !workDir) return;
    let cancelled = false;
    setLoading(true);
    setTree(null);
    setTreeData([]);
    setExpandedKeys([]);
    setSelectedAbsPath('');
    setFileContent(null);
    setPanelMode('view');
    setFileViewMode('source');
    setCommits([]);
    browseTree(agentId, workDir)
      .then((data) => {
        if (cancelled) return;
        setTree(data);
        setTreeData(buildRootNodes(data.repoRoot, data.rootEntries));
      })
      .catch((err) => {
        if (!cancelled) {
          antMessage.error(err instanceof ApiError ? err.message : '加载文件列表失败');
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
  }, [open, agentId, workDir]);

  useEffect(() => {
    loadTree();
  }, [loadTree]);

  // 从 FileEntry[] 构造 antd Tree 节点
  const buildNodes = useCallback((absDir: string, entries: import('@/api/files').FileEntry[]): TreeDataNode[] => {
    return entries.map((entry) => {
      const absPath = `${absDir}/${entry.name}`;
      const extra: FileNodeExtra = { absPath, isDir: entry.type === 'dir' };
      const node: TreeDataNode = {
        key: absPath,
        title: (
          <span className={styles.treeNodeName}>
            {entry.type === 'dir' ? <FolderOutlined className={styles.iconDir} /> : <FileOutlined className={styles.iconFile} />}
            {' '}{entry.name}
          </span>
        ),
        icon: null,
        isLeaf: entry.type !== 'dir',
        children: entry.type === 'dir' ? [] : undefined,
      };
      (node as TreeDataNode & { extra?: FileNodeExtra }).extra = extra;
      return node;
    });
  }, []);

  function buildRootNodes(repoRoot: string, entries: import('@/api/files').FileEntry[]): TreeDataNode[] {
    return buildNodes(repoRoot, entries);
  }

  // 懒加载：展开目录时 listDir 拿子层
  const onLoadData = useCallback(
    async ({ key, children }: { key: React.Key; children?: TreeDataNode[] }) => {
      if (children && children.length > 0) return;
      const absPath = String(key);
      try {
        const entries = await listDir(agentId, workDir, absPath);
        const nodes = buildNodes(absPath, entries);
        setTreeData((prev) => updateTreeChildren(prev, absPath, nodes));
      } catch (err) {
        antMessage.error(err instanceof ApiError ? err.message : '加载目录失败');
      }
    },
    [agentId, workDir, buildNodes],
  );

  // 递归更新某个 key 的 children（antd Tree 没有 helper，需手写）
  function updateTreeChildren(nodes: TreeDataNode[], targetKey: string, children: TreeDataNode[]): TreeDataNode[] {
    return nodes.map((node) => {
      if (node.key === targetKey) {
        return { ...node, children };
      }
      if (node.children && node.children.length > 0) {
        return { ...node, children: updateTreeChildren(node.children, targetKey, children) };
      }
      return node;
    });
  }

  // 选中文件（叶子节点）→ 切换到 view 模式读工作区内容
  const onSelectFile = useCallback(
    (absPath: string) => {
      if (!absPath) return;
      setSelectedAbsPath(absPath);
      setPanelMode('view');
      setFileViewMode(defaultFileViewMode(absPath));
      setFileContent(null);
      setFileLoading(true);
      readFile(agentId, workDir, absPath)
        .then((result) => setFileContent(result))
        .catch((err) => {
          antMessage.error(err instanceof ApiError ? err.message : '读取文件失败');
        })
        .finally(() => setFileLoading(false));
    },
    [agentId, workDir],
  );

  const handleTreeSelect = useCallback(
    (_keys: React.Key[], info: { node: TreeDataNode }) => {
      const extra = (info.node as TreeDataNode & { extra?: FileNodeExtra }).extra;
      if (extra && !extra.isDir) {
        onSelectFile(extra.absPath);
      }
    },
    [onSelectFile],
  );

  const onSelectChanged = useCallback(
    (relPath: string) => {
      if (!tree) return;
      onSelectFile(`${tree.repoRoot}/${relPath}`);
    },
    [tree, onSelectFile],
  );

  // 切换到 history 模式：拉当前文件的 git commit 列表
  const handleShowHistory = useCallback(() => {
    if (!selectedAbsPath) return;
    setPanelMode('history');
    setCommits([]);
    setDiffOld(null);
    setDiffNew(null);
    setCommitsLoading(true);
    fileHistory(agentId, workDir, selectedAbsPath)
      .then((list) => {
        setCommits(list);
        // 默认对比最新两个 commit（最新在 index 0）
        if (list.length >= 2) {
          setNewRev(list[0]!.hash);
          setOldRev(list[1]!.hash);
        } else if (list.length === 1) {
          setNewRev(list[0]!.hash);
          setOldRev(list[0]!.hash);
        }
      })
      .catch((err) => {
        antMessage.error(err instanceof ApiError ? err.message : '加载历史失败');
      })
      .finally(() => setCommitsLoading(false));
  }, [agentId, workDir, selectedAbsPath]);

  // 选定 commit 对变化时，拉两个版本内容做 diff
  useEffect(() => {
    if (panelMode !== 'history' || !oldRev || !newRev || !selectedAbsPath) return;
    if (oldRev === newRev) {
      setDiffOld(null);
      setDiffNew(null);
      return;
    }
    let cancelled = false;
    setDiffLoading(true);
    Promise.all([
      fileShow(agentId, workDir, selectedAbsPath, oldRev),
      fileShow(agentId, workDir, selectedAbsPath, newRev),
    ])
      .then(([o, n]) => {
        if (cancelled) return;
        setDiffOld(o);
        setDiffNew(n);
      })
      .catch((err) => {
        if (!cancelled) antMessage.error(err instanceof ApiError ? err.message : '加载版本内容失败');
      })
      .finally(() => {
        if (!cancelled) setDiffLoading(false);
      });
    return () => { cancelled = true; };
  }, [panelMode, oldRev, newRev, selectedAbsPath, agentId, workDir]);

  const handleDownloadFile = useCallback(async () => {
    if (!selectedAbsPath) return;
    setDownloading(true);
    try {
      const result = await downloadFile(agentId, workDir, selectedAbsPath);
      if (!result) {
        antMessage.warning('该文件为二进制或过大，请改用「下载整个目录」');
        return;
      }
      triggerBlobDownload(result.blob, result.filename);
    } catch (err) {
      antMessage.error(err instanceof ApiError ? err.message : '下载失败');
    } finally {
      setDownloading(false);
    }
  }, [agentId, workDir, selectedAbsPath]);

  const handleDownloadZip = useCallback(async () => {
    if (!tree) return;
    setDownloading(true);
    try {
      const blob = await downloadZip(agentId, workDir, tree.repoRoot);
      const name = tree.repoRoot.split('/').pop() || 'agenthub-files';
      triggerBlobDownload(blob, `${name}.zip`);
    } catch (err) {
      antMessage.error(err instanceof ApiError ? err.message : '下载失败');
    } finally {
      setDownloading(false);
    }
  }, [agentId, workDir, tree]);

  const selectedRelPath = useMemo(() => {
    if (!tree || !selectedAbsPath) return '';
    const rel = selectedAbsPath.replace(`${tree.repoRoot}/`, '');
    return rel || selectedAbsPath;
  }, [tree, selectedAbsPath]);

  const fileLanguage = useMemo(() => inferLanguageFromPath(selectedRelPath), [selectedRelPath]);
  const isMarkdown = selectedRelPath.toLowerCase().endsWith('.md');
  const isHtml = isHtmlPreviewFile(selectedRelPath);

  // 文件内容渲染分支（view 模式）
  const renderFileContent = () => {
    if (fileLoading) {
      return <div className={styles.hintArea}><Spin /> 读取中…</div>;
    }
    if (!fileContent) {
      return <div className={styles.hintArea}><FileOutlined /> 选择左侧文件查看内容</div>;
    }
    if (fileContent.binary) {
      return <div className={styles.hintArea}><FileOutlined /> 二进制文件，不支持预览</div>;
    }
    if (fileContent.tooLarge) {
      return <div className={styles.hintArea}><FileOutlined /> 文件过大（{(fileContent.size / 1024 / 1024).toFixed(1)} MB），不支持预览，请下载查看</div>;
    }
    if (isMarkdown) {
      return (
        <div className={styles.markdownPreview}>
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{fileContent.content}</ReactMarkdown>
        </div>
      );
    }
    if (isHtml && fileViewMode === 'preview') {
      return (
        <div className={styles.htmlPreview}>
          <WebpageFrame srcDoc={fileContent.content} />
        </div>
      );
    }
    if (fileLanguage) {
      return (
        <Suspense fallback={<div className={styles.hintArea}><Spin /> 加载代码视图…</div>}>
          <div className={styles.codePreviewWrap}>
            <CodeSelectView value={fileContent.content} language={fileLanguage} />
          </div>
        </Suspense>
      );
    }
    return <pre className={styles.textPreview}>{fileContent.content}</pre>;
  };

  // git 历史对比渲染分支（history 模式）
  const renderHistoryDiff = () => {
    if (commitsLoading) {
      return <div className={styles.hintArea}><Spin /> 加载历史…</div>;
    }
    if (commits.length === 0) {
      return <div className={styles.hintArea}><HistoryOutlined /> 该文件暂无 git 历史</div>;
    }
    if (!oldRev || !newRev) {
      return <div className={styles.hintArea}><HistoryOutlined /> 选择上方两个 commit 进行对比</div>;
    }
    if (oldRev === newRev) {
      return <div className={styles.hintArea}><HistoryOutlined /> 请选择两个不同的 commit</div>;
    }
    if (diffLoading) {
      return <div className={styles.hintArea}><Spin /> 加载版本内容…</div>;
    }
    if (!diffOld || !diffNew) {
      return <div className={styles.hintArea}><HistoryOutlined /> 版本内容加载失败</div>;
    }
    if (diffOld.binary || diffNew.binary || diffOld.tooLarge || diffNew.tooLarge) {
      return <div className={styles.hintArea}><HistoryOutlined /> 版本为二进制或过大，不支持对比</div>;
    }
    return (
      <Suspense fallback={<div className={styles.hintArea}><Spin /> 加载对比视图…</div>}>
        <div className={styles.codePreviewWrap}>
          <DiffView oldDoc={diffOld.content} newDoc={diffNew.content} language={fileLanguage} />
        </div>
      </Suspense>
    );
  };

  const commitSelectOptions = useMemo(
    () => commits.map((c) => ({
      value: c.hash,
      label: `${c.hash.slice(0, 8)} ${c.subject}（${c.author}）`,
    })),
    [commits],
  );

  return (
    <Drawer
      title={
        <span>
          <FolderOpenOutlined /> 文件浏览器
        </span>
      }
      open={open}
      onClose={onClose}
      width={900}
      placement="right"
      destroyOnClose
    >
      <div className={styles.drawerBody}>
        {/* 顶部工具栏：仓库路径 + 下载/刷新按钮 */}
        <div className={styles.toolbar}>
          <span className={styles.toolbarPath} title={tree?.repoRoot || workDir}>
            {tree?.repoRoot || workDir}
          </span>
          <Tooltip title="刷新">
            <Button type="text" size="small" icon={<ReloadOutlined />} onClick={loadTree} disabled={loading} />
          </Tooltip>
          <Tooltip title="下载当前文件">
            <Button
              type="text"
              size="small"
              icon={<DownloadOutlined />}
              onClick={handleDownloadFile}
              disabled={!selectedAbsPath || downloading}
              loading={downloading}
            />
          </Tooltip>
          <Tooltip title="下载整个项目（zip）">
            <Button
              type="text"
              size="small"
              icon={<FileExcelOutlined />}
              onClick={handleDownloadZip}
              disabled={!tree || downloading}
            >
              打包
            </Button>
          </Tooltip>
        </div>

        <div className={styles.contentSplit}>
          {/* 左侧：文件树 */}
          <div className={styles.fileTree}>
            {/* 改动文件折叠区（仅 git 仓库 + 有改动时显示） */}
            {tree && tree.isGit && tree.changedFiles.length > 0 && (
              <div className={styles.treeSection}>
                <div
                  className={styles.treeSectionHeader}
                  onClick={() => setChangedCollapsed((c) => !c)}
                >
                  <span>
                    {changedCollapsed ? <DownOutlined rotate={-90} /> : <DownOutlined />}
                    {' '}改动文件
                  </span>
                  <span className={styles.treeSectionCount}>{tree.changedFiles.length}</span>
                </div>
                {!changedCollapsed && (
                  <div className={styles.changedList}>
                    {tree.changedFiles.map((cf: ChangedFile, idx) => (
                      <div
                        key={`${cf.path}-${idx}`}
                        className={`${styles.changedItem} ${selectedRelPath === cf.path ? styles.changedItemActive : ''}`}
                        onClick={() => onSelectChanged(cf.path)}
                        title={cf.path}
                      >
                        {STATUS_ICON[cf.status]}
                        <span className={styles.treeNodeName}>{cf.path}</span>
                        <span className={`${styles.fileBadge} ${styles[`badge_${cf.status}`] || ''}`}>
                          {STATUS_LABEL[cf.status]}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* 目录树（懒加载 + 虚拟滚动） */}
            <div className={styles.dirTree} ref={treeContainerRef}>
              <div className={styles.treeSectionHeader}>目录</div>
              {loading ? (
                <div className={styles.hintArea}><Spin size="small" /></div>
              ) : treeData.length === 0 ? (
                <Empty description="无文件" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              ) : (
                <Tree
                  treeData={treeData}
                  loadData={onLoadData}
                  onSelect={handleTreeSelect}
                  expandedKeys={expandedKeys}
                  onExpand={(keys) => setExpandedKeys(keys.map(String))}
                  selectedKeys={selectedAbsPath ? [selectedAbsPath] : []}
                  showIcon={false}
                  blockNode
                  virtual
                  height={treeHeight}
                  itemHeight={28}
                />
              )}
            </div>
          </div>

          {/* 右侧：文件内容（view / history 两个模式） */}
          <div className={styles.filePanel}>
            <div className={styles.filePanelHeader}>
              <span className={styles.filePanelPath}>{selectedRelPath || '（未选择）'}</span>
              {selectedAbsPath && panelMode === 'view' && isHtml && (
                <Segmented<FileViewMode>
                  size="small"
                  value={fileViewMode}
                  onChange={setFileViewMode}
                  options={[
                    { label: '预览', value: 'preview', icon: <EyeOutlined /> },
                    { label: '源码', value: 'source', icon: <CodeOutlined /> },
                  ]}
                />
              )}
              {selectedAbsPath && (
                <Tooltip title={panelMode === 'history' ? '返回查看' : '查看 git 历史'}>
                  <Button
                    type="text"
                    size="small"
                    icon={<HistoryOutlined />}
                    onClick={panelMode === 'history' ? () => setPanelMode('view') : handleShowHistory}
                    disabled={!tree?.isGit}
                  />
                </Tooltip>
              )}
            </div>

            {/* history 模式：顶部 commit 对选择器 */}
            {panelMode === 'history' && (
              <div className={styles.historyBar}>
                <Select
                  size="small"
                  style={{ flex: 1, minWidth: 0 }}
                  value={oldRev || undefined}
                  onChange={setOldRev}
                  options={commitSelectOptions}
                  placeholder="旧版本"
                  showSearch
                  optionFilterProp="label"
                />
                <span className={styles.historyArrow}>→</span>
                <Select
                  size="small"
                  style={{ flex: 1, minWidth: 0 }}
                  value={newRev || undefined}
                  onChange={setNewRev}
                  options={commitSelectOptions}
                  placeholder="新版本"
                  showSearch
                  optionFilterProp="label"
                />
              </div>
            )}

            <div className={styles.filePanelContent}>
              {panelMode === 'view' ? renderFileContent() : renderHistoryDiff()}
            </div>
          </div>
        </div>
      </div>
    </Drawer>
  );
};

/** 从文件路径推断代码语言（供 CodeSelectView 语法高亮）。 */
function inferLanguageFromPath(filepath: string): string {
  const ext = filepath.split('.').pop()?.toLowerCase() || '';
  const map: Record<string, string> = {
    ts: 'typescript', tsx: 'typescript', js: 'javascript', jsx: 'javascript', mjs: 'javascript',
    py: 'python', go: 'go', rs: 'rust', java: 'java', rb: 'ruby', kt: 'kotlin',
    html: 'html', css: 'css', scss: 'css', less: 'css', json: 'json', md: 'markdown',
    yml: 'yaml', yaml: 'yaml', toml: 'toml', sh: 'shell', bash: 'shell',
    sql: 'sql', xml: 'xml', vue: 'html', php: 'php', c: 'c', cpp: 'cpp', h: 'c',
  };
  return map[ext] || '';
}

/** 触发浏览器下载（Blob + 临时 a 标签）。 */
function triggerBlobDownload(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
