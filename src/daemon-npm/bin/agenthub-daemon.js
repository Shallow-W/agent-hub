#!/usr/bin/env node

const { execFileSync, spawn, spawnSync } = require('node:child_process');
const { EventEmitter } = require('node:events');
const crypto = require('node:crypto');
const fs = require('node:fs');
const http = require('node:http');
const https = require('node:https');
const os = require('node:os');
const path = require('node:path');
// CliToolSpec 注册表：把原本散落在 commandForTask / ensureGlobalMcpConfigs /
// skillRoots / scanAgents 四处分支点的 CLI 行为（claude/codex/opencode/openclaw）
// 收敛为 spec 对象。新增 CLI 只需在 cli/index.js 加一个工厂，零修改分发函数。
const cliTools = require('../cli');
const { StreamBuffer } = require('../cli/stream_adapter');

// ===========================================================================
// CONFIG —— daemon 所有运行时配置集中在此，便于一处查看与调整。
// 按功能分组：连接 / 任务执行 / MCP / 部署 / 文件浏览 / 技能 / 路径。
// 单文件发布约束：不抽成独立文件（发布包只含 bin/agenthub-daemon.js）。
// ===========================================================================
const CONFIG = {
  // —— 连接（WebSocket + 轮询）——
  wsReconnectDelayMs: 3000,
  wsPingIntervalMs: 30000,
  inboundWatchdogMs: 70000,        // 无入站流量的最大静默期，超时强制断开重连
  pollIntervalMs: 3000,            // WS 不可用时的 HTTP 轮询间隔
  heartbeatIntervalMs: 30000,

  // —— 任务执行 ——
  execTimeoutMs: 400000,           // agent CLI 单次执行上限（~6.6 分钟）

  // —— MCP ——
  mcpProtocolVersion: '2024-11-05',

  // —— 部署（Docker + cloudflared 隧道，agent 主导模式）——
  deploy: {
    stateDir: path.join(os.homedir(), '.agenthub', 'deploys'),
    ttlMs: 4 * 60 * 60 * 1000,       // 部署有效期 4 小时，过期自动停止
    buildTimeoutMs: 5 * 60 * 1000,   // docker build（含 npm install 等）
    runTimeoutMs: 60 * 1000,         // docker run（启动容器）
    stopTimeoutMs: 10 * 1000,        // docker stop
    tunnelTimeoutMs: 30 * 1000,      // cloudflared 拿 URL 超时
    cloudflaredDir: path.join(os.homedir(), '.agenthub', 'cloudflared'),
    tunnelUrlRegex: /https:\/\/[a-z0-9-]+\.trycloudflare\.com/i,
  },

  // —— 文件浏览 RPC（前端抽屉浏览 agent 机器文件）——
  browse: {
    toolName: '__agenthub_browse_files__',
    gitTimeoutMs: 10000,             // git 命令单独超时，避免大仓库卡住
    fileReadMaxSize: 2 * 1024 * 1024,// 单文件预览 2MB 上限
    zipMaxTotalSize: 100 * 1024 * 1024, // 整目录打包 100MB 上限
    excludeDirs: new Set(['node_modules', '.git', '.next', 'dist', 'build', '.cache', '.agenthub']),
  },

  // —— 技能 / Agent 管理 ——
  openPathTool: '__agenthub_open_path__',
  openPathTimeoutMs: 5000,
  startQueueIntervalMs: 3000,
  minDescriptionChars: 6,
  sessionsFile: path.join(os.homedir(), '.agenthub', 'sessions.json'),
};

// 向后兼容别名（迁移期保留旧名引用，避免大范围改写；新代码请用 CONFIG.xxx）
const EXEC_TIMEOUT_MS = CONFIG.execTimeoutMs;
const HEARTBEAT_INTERVAL_MS = CONFIG.heartbeatIntervalMs;
const WS_RECONNECT_DELAY_MS = CONFIG.wsReconnectDelayMs;
const WS_PING_INTERVAL_MS = CONFIG.wsPingIntervalMs;
const INBOUND_WATCHDOG_MS = CONFIG.inboundWatchdogMs;
const POLL_INTERVAL_MS = CONFIG.pollIntervalMs;
const DAEMON_LOG_EVENT = 'daemon_flow';
// MCP
const MCP_PROTOCOL_VERSION = CONFIG.mcpProtocolVersion;
// cloudflared / 隧道
const CLOUDFLARED_DIR = CONFIG.deploy.cloudflaredDir;
const TUNNEL_URL_REGEX = CONFIG.deploy.tunnelUrlRegex;
// 部署
const DEPLOY_STATE_DIR = CONFIG.deploy.stateDir;
const DEPLOY_TTL_MS = CONFIG.deploy.ttlMs;
const DEPLOY_BUILD_TIMEOUT_MS = CONFIG.deploy.buildTimeoutMs;
const DEPLOY_RUN_TIMEOUT_MS = CONFIG.deploy.runTimeoutMs;
const DEPLOY_STOP_TIMEOUT_MS = CONFIG.deploy.stopTimeoutMs;
const DEPLOY_TUNNEL_TIMEOUT_MS = CONFIG.deploy.tunnelTimeoutMs;
// 文件浏览
const BROWSE_FILES_TOOL = CONFIG.browse.toolName;
const BROWSE_GIT_TIMEOUT_MS = CONFIG.browse.gitTimeoutMs;
const BROWSE_FILE_READ_MAX_SIZE = CONFIG.browse.fileReadMaxSize;
const BROWSE_ZIP_MAX_TOTAL_SIZE = CONFIG.browse.zipMaxTotalSize;
const BROWSE_EXCLUDE_DIRS = CONFIG.browse.excludeDirs;
// 技能 / Agent 管理
const SESSIONS_FILE = CONFIG.sessionsFile;
const START_QUEUE_INTERVAL_MS = CONFIG.startQueueIntervalMs;
const OPEN_PATH_TIMEOUT_MS = CONFIG.openPathTimeoutMs;
const MIN_DESCRIPTION_CHARS = CONFIG.minDescriptionChars;
const OPEN_PATH_TOOL = CONFIG.openPathTool;

// ---------------------------------------------------------------------------
// TaskContext + OutputCollector 注册表
// ---------------------------------------------------------------------------
// 每个任务的执行上下文：承载 per-task 的可收集输出（cards 等），取代旧的
// 模块级 currentCardFile 全局。从 handleTaskDispatch 一路传到
// dispatchToPersistentSlot → spawnStreamJsonProcess → sendPrompt，使"当前任务"显式化。
//
// 扩展点：新增一种 daemon 输出类型（如 progress 流、截图、日志），只需：
//   registerOutputCollector('xxx', (ctx) => ({ reset(), drain(), cleanup(), ... }))
// handleTaskDispatch 主干不再需要改动。

/**
 * 单个任务的执行上下文。每个 task.dispatch 创建一个实例，随执行链路传递。
 */
class TaskContext {
  constructor(task) {
    this.taskId = task && task.id;
    this.agentId = task && task.agent_id;
    this.conversationId = task && task.conversation_id;
    this.userId = task && task.user_id;
    // D5 ADR: backend 预创建的 streaming message ID。task.dispatch 时由 backend 传入，
    // daemon 在 task.progress 原样回传，让前端按 message_id 路由流式 delta。
    this.messageId = task && task.message_id;
    // name → collector。collector 由 OutputCollector 工厂创建。
    this.outputs = new Map();
  }

  /** 由 handleTaskDispatch 调用：用注册的工厂实例化所有 collector 挂到本任务。 */
  attachCollectors() {
    for (const [name, factory] of outputCollectors) {
      this.outputs.set(name, factory(this));
    }
  }

  /** 取某个输出通道的 collector（如 'cards'）。不存在返回 undefined。 */
  output(name) {
    return this.outputs.get(name);
  }

  /** 结束一个输出通道：返回其 drain() 结果（通常为数组）。无 collector 返回 []。 */
  finalize(name) {
    const collector = this.outputs.get(name);
    if (!collector) return [];
    return typeof collector.drain === 'function' ? collector.drain() : (collector.items || []);
  }

  /** 任务结束后清理所有 collector 的临时资源（临时文件等）。 */
  cleanup() {
    for (const collector of this.outputs.values()) {
      try { if (typeof collector.cleanup === 'function') collector.cleanup(); } catch { /* ignore */ }
    }
    this.outputs.clear();
  }
}

// name → factory(ctx) => collector。collector 接口：
//   { reset(), drain() => any[], cleanup(), ... 任意自定义字段 }
//
// 当前为空——卡片原由 'cards' collector 通过临时文件 IPC 收集，已改为 agent 直接
// 在回复正文写 fenced JSON block，daemon 不再参与卡片收集（见 extractCardsFromContent）。
// 保留 registerOutputCollector + attachCollectors 作为扩展点：未来若新增 daemon 侧
// 的批量输出类型（progress 流、截图、日志聚合等），可在此注册，无需改 TaskContext。
const outputCollectors = new Map();

/**
 * 注册一个输出收集器工厂。factory 收到 TaskContext，返回一个 collector 对象。
 * collector 至少应实现 reset() / drain() / cleanup()；可携带任意附加字段。
 *
 * 当前未使用——保留为未来扩展点（如 daemon 侧聚合 progress / 截图 / 日志）。
 * 新增 collector 时在文件底部调用 registerOutputCollector('xxx', factory) 即可，
 * TaskContext.attachCollectors 会自动实例化并挂到 task.outputs。
 */
function registerOutputCollector(name, factory) {
  outputCollectors.set(name, factory);
}

// ---------------------------------------------------------------------------
// Docker 检测 + 部署执行 + cloudflared 隧道
// ---------------------------------------------------------------------------
// daemon 侧的网页部署能力：收到后端的 deploy.request → 写文件树 → docker build/run
// → cloudflared 穿透容器端口 → 回报公网 URL。
// 与 task.dispatch 平行，是 daemon 的第二类执行能力。

/** 检测本机 docker 是否可用（docker info 能跑即认为可用）。 */
function detectDocker() {
  try {
    execFileSync('docker', ['info', '--format', '{{.ServerVersion}}'], { stdio: ['ignore', 'pipe', 'ignore'], timeout: 5000 });
    return true;
  } catch {
    return false;
  }
}

/** 收集本机能力清单，用于 daemon.register 上报。后端据此选合适的 machine 部署。 */
function detectCapabilities() {
  const caps = [];
  if (detectDocker()) caps.push('docker');
  return caps;
}

/** 找一个空闲端口（用于 docker -p 端口映射）。 */
function findFreePort() {
  return new Promise((resolve, reject) => {
    const srv = require('node:net').createServer();
    srv.unref();
    srv.on('error', reject);
    srv.listen(0, '127.0.0.1', () => {
      const { port } = srv.address();
      srv.close(() => resolve(port));
    });
  });
}

// cloudflared 二进制管理：优先 PATH，找不到则下载到 ~/.agenthub/cloudflared。
// 路径与隧道 URL 正则在顶部 CONFIG.deploy 配置；这里只有二进制文件名映射逻辑。

function cloudflaredBinary() {
  const platform = process.platform;
  const arch = process.arch;
  // 映射 (platform, arch) → cloudflared release 文件名
  const osName = platform === 'darwin' ? 'darwin' : platform === 'win32' ? 'windows' : 'linux';
  const archName = arch === 'arm64' ? 'arm64' : 'amd64';
  const ext = osName === 'windows' ? '.exe' : '';
  return path.join(CLOUDFLARED_DIR, `cloudflared-${osName}-${archName}${ext}`);
}

/** 确保 cloudflared 二进制存在：PATH 里有就用系统的，否则下载。返回可执行路径或 null。 */
function ensureCloudflared() {
  // 1. 先试 PATH 里的
  try {
    const which = process.platform === 'win32' ? 'where' : 'which';
    const out = execFileSync(which, ['cloudflared'], { stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim();
    if (out) return out.split('\n')[0];
  } catch { /* not in PATH */ }
  // 2. 再试已下载的
  const bin = cloudflaredBinary();
  if (fs.existsSync(bin)) return bin;
  // 3. 下载（首次）
  return downloadCloudflared();
}

/** 下载 cloudflared 到 ~/.agenthub/cloudflared/。返回路径或 null（失败）。 */
function downloadCloudflared() {
  try {
    fs.mkdirSync(CLOUDFLARED_DIR, { recursive: true });
    const platform = process.platform;
    const osName = platform === 'darwin' ? 'darwin' : platform === 'win32' ? 'windows' : 'linux';
    const archName = process.arch === 'arm64' ? 'arm64' : 'amd64';
    const ext = osName === 'windows' ? '.exe' : '';
    // cloudflared 官方提供单文件二进制（非 tgz），直接下载即可
    const url = `https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-${osName}-${archName}${ext}`;
    logFlow('info', 'cloudflared.download_start', { url });
    const binPath = cloudflaredBinary();
    // 同步下载（execFileSync 调 curl/wget，避免 Node https 流式处理的复杂度）
    const downloader = process.platform === 'win32' ? null : 'curl';
    if (downloader === 'curl') {
      execFileSync('curl', ['-sL', '-o', binPath, url], { stdio: 'ignore', timeout: 120000 });
    } else {
      // fallback: wget 或手动 https 下载
      try { execFileSync('wget', ['-qO', binPath, url], { stdio: 'ignore', timeout: 120000 }); }
      catch { return null; }
    }
    if (!fs.existsSync(binPath) || fs.statSync(binPath).size < 100000) return null;
    fs.chmodSync(binPath, 0o755);
    logFlow('info', 'cloudflared.download_done', { path: binPath });
    return binPath;
  } catch (e) {
    logFlow('warn', 'cloudflared.download_failed', { error: errorMessage(e) });
    return null;
  }
}

/**
 * 启动一个 cloudflared quick tunnel 穿透指定端口。
 * 返回 Promise<{url, process, pid}>，url 形如 https://xxx.trycloudflare.com。
 * 失败（超时 30s 未拿到 URL / 二进制不可用）则 reject。
 * 返回的 process 会被调用方 unref 脱离父进程；pid 写入部署状态文件供停止时 kill。
 */
function startTunnel(port) {
  return new Promise((resolve, reject) => {
    const bin = ensureCloudflared();
    if (!bin) {
      reject(new Error('cloudflared 不可用：PATH 无且下载失败'));
      return;
    }
    const child = spawn(bin, ['tunnel', '--no-autoupdate', '--url', `http://localhost:${port}`], {
      stdio: ['ignore', 'pipe', 'pipe'],
      detached: true, // 独立进程组，父进程退出后 cloudflared 继续运行（部署需长驻 4h）
    });
    let resolved = false;
    const timer = setTimeout(() => {
      if (!resolved) {
        resolved = true;
        try { child.kill(); } catch { /* ignore */ }
        reject(new Error(`cloudflared 隧道启动超时（${DEPLOY_TUNNEL_TIMEOUT_MS / 1000}s 未拿到 URL）`));
      }
    }, DEPLOY_TUNNEL_TIMEOUT_MS);

    const scanUrl = (chunk) => {
      if (resolved) return;
      const text = chunk.toString();
      const match = text.match(TUNNEL_URL_REGEX);
      if (match) {
        resolved = true;
        clearTimeout(timer);
        resolve({ url: match[0], process: child });
      }
    };
    child.stdout.on('data', scanUrl);
    child.stderr.on('data', scanUrl);
    child.on('error', (e) => {
      if (!resolved) {
        resolved = true;
        clearTimeout(timer);
        reject(new Error(`cloudflared 启动失败: ${errorMessage(e)}`));
      }
    });
    child.on('exit', (code) => {
      if (!resolved) {
        resolved = true;
        clearTimeout(timer);
        reject(new Error(`cloudflared 进程退出（code=${code}）`));
      }
    });
  });
}

// ---------------------------------------------------------------------------
// 部署状态文件 IPC + TTL 管理
// ---------------------------------------------------------------------------
// 部署由 MCP 工具（deploy_project）在 MCP 子进程里发起，docker 容器和 cloudflared
// 进程独立存活（detached）。状态通过 ~/.agenthub/deploys/<id>.json 文件传递给
// daemon 主进程，由主进程负责 TTL 清理（4 小时后停止）。
// 所有路径/超时/TTL 在顶部 CONFIG.deploy 配置。

/** 部署状态文件路径。 */
function deployStatePath(deployId) {
  return path.join(DEPLOY_STATE_DIR, `${deployId}.json`);
}

/**
 * 写部署状态文件。MCP 子进程部署成功后调用，供 daemon 主进程 TTL 清理。
 * state: { deployId, containerName, port, tunnelPid, url, workDir, createdAt, expiresAt }
 * workDir 是 agent 的代码目录（sourceDir），仅作记录，停止时不删除（代码属 agent）。
 */
function writeDeployState(state) {
  try {
    fs.mkdirSync(DEPLOY_STATE_DIR, { recursive: true });
    fs.writeFileSync(deployStatePath(state.deployId), JSON.stringify(state));
  } catch (e) {
    logFlow('warn', 'deploy.state_write_failed', { deploy_id: state.deployId, error: errorMessage(e) });
  }
}

/** 读单个部署状态文件。不存在返回 null。 */
function readDeployState(deployId) {
  try {
    const raw = fs.readFileSync(deployStatePath(deployId), 'utf8');
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

/** 列出所有部署状态（扫 ~/.agenthub/deploys/）。 */
function listDeployStates() {
  try {
    const files = fs.readdirSync(DEPLOY_STATE_DIR).filter((f) => f.endsWith('.json'));
    const states = [];
    for (const f of files) {
      try {
        const raw = fs.readFileSync(path.join(DEPLOY_STATE_DIR, f), 'utf8');
        states.push(JSON.parse(raw));
      } catch { /* 损坏文件跳过 */ }
    }
    return states;
  } catch {
    return [];
  }
}

/** 删除部署状态文件。 */
function removeDeployState(deployId) {
  try { fs.rmSync(deployStatePath(deployId), { force: true }); } catch { /* ignore */ }
}

/**
 * 停止一个部署：杀 cloudflared 进程 + docker stop 容器 + 删状态文件。
 * 供 stop_deploy MCP 工具和 TTL 扫描器调用。
 *
 * 部署都在 MCP 子进程发起，状态通过文件 IPC（~/.agenthub/deploys/<id>.json）传递，
 * 故这里只读状态文件（不再有内存 runningDeploys——那是已废弃的 WS 部署路径遗留）。
 *
 * 注意：不删除 sourceDir（agent 的真实代码目录）——重构后部署直接用 sourceDir 作
 * 构建上下文，代码属于 agent，平台无权删除。
 */
function stopDeploy(deployId) {
  const info = readDeployState(deployId);
  if (!info) return false;

  logFlow('info', 'deploy.stop', { deploy_id: deployId, container: info.containerName });
  // 杀 cloudflared 进程
  if (info.tunnelPid) {
    try { process.kill(info.tunnelPid); } catch { /* 进程已退出 */ }
  }
  // 停 docker 容器
  if (info.containerName) {
    try { execFileSync('docker', ['stop', info.containerName], { stdio: 'ignore', timeout: DEPLOY_STOP_TIMEOUT_MS }); } catch { /* ignore */ }
  }
  removeDeployState(deployId);
  return true;
}

/**
 * TTL 扫描器：检查所有部署状态文件，过期（createdAt + 4h < now）则停止。
 * daemon 主进程启动时立即扫一次，之后每 5 分钟扫一次。
 */
function scanAndCleanupDeploys() {
  const states = listDeployStates();
  const now = Date.now();
  for (const state of states) {
    const expiresAt = state.expiresAt || (state.createdAt + DEPLOY_TTL_MS);
    if (now >= expiresAt) {
      logFlow('info', 'deploy.ttl_expired', { deploy_id: state.deployId, container: state.containerName });
      stopDeploy(state.deployId);
    }
  }
}

/**
 * 纯执行：校验 Dockerfile → docker build/run → cloudflared 隧道。
 * 不依赖 ws，返回 { url, containerName, port, workDir, tunnelProcess }。
 * 供 MCP 工具 deploy_project 调用（部署都在 MCP 子进程，状态走文件 IPC）。
 *
 * 注意：cloudflared 进程会 detached + unref，脱离父进程独立存活——
 * MCP 子进程退出后隧道仍运行，由 daemon 主进程的 TTL 扫描器按 4h 清理。
 */
// 部署执行：纯平台职责（执行器 + 环境提供者）。
// agent 主导模式——sourceDir 是 agent 的真实代码目录（workDir），Dockerfile 由 agent 写好放在里面。
// 平台只做：校验 Dockerfile 存在 → docker build（用 sourceDir 作上下文）→ docker run（port 生效）→ 隧道。
// 不做：文件收集、Dockerfile 生成、runtime 推断、URL 验证（这些是 agent 的职责）。
async function executeDeploy(deployId, sourceDir, port = 80) {
  if (!deployId || !sourceDir) {
    throw new Error('参数缺失（deploy_id/source_dir）');
  }
  const containerName = `agenthub-deploy-${deployId.slice(0, 12)}`;
  logFlow('info', 'deploy.start', { deploy_id: deployId, source_dir: sourceDir, port, container: containerName });

  // 1. 校验 sourceDir 存在且是目录
  if (!fs.existsSync(sourceDir) || !fs.statSync(sourceDir).isDirectory()) {
    throw new Error(`source_dir 不存在或不是目录: ${sourceDir}`);
  }

  // 2. Dockerfile 必须存在（agent 负责，平台不兜底生成）
  if (!fs.existsSync(path.join(sourceDir, 'Dockerfile'))) {
    throw new Error('source_dir 缺少 Dockerfile。agent 必须先写好 Dockerfile（FROM + 业务构建步骤 + EXPOSE <端口>）再调用部署');
  }

  // 3. docker build（直接用 sourceDir 作构建上下文）
  try {
    execFileSync('docker', ['build', '-t', containerName, sourceDir], {
      stdio: ['ignore', 'pipe', 'pipe'],
      timeout: DEPLOY_BUILD_TIMEOUT_MS,
    });
  } catch (e) {
    const stderr = e.stderr ? e.stderr.toString().slice(-2000) : errorMessage(e);
    throw new Error(`docker build 失败：${stderr}`);
  }

  // 4. docker run（后台 -d，port 参数生效——映射到 agent Dockerfile 的 EXPOSE 端口）
  const hostPort = await findFreePort();
  let runOut;
  try {
    runOut = execFileSync('docker', ['run', '-d', '--rm', '-p', `${hostPort}:${port}`, '--name', containerName, containerName], {
      stdio: ['ignore', 'pipe', 'pipe'],
      timeout: DEPLOY_RUN_TIMEOUT_MS,
    }).toString().trim();
  } catch (e) {
    const stderr = e.stderr ? e.stderr.toString().slice(-2000) : errorMessage(e);
    throw new Error(`docker run 失败（端口 ${port}→${hostPort}）：${stderr}`);
  }
  logFlow('info', 'deploy.container_started', { deploy_id: deployId, container: containerName, port: hostPort, container_port: port, container_id: runOut.slice(0, 12) });

  // 5. cloudflared 隧道穿透 hostPort
  const tunnelInfo = await startTunnel(hostPort);
  logFlow('info', 'deploy.tunnel_ready', { deploy_id: deployId, url: tunnelInfo.url });

  // 6. cloudflared 进程 detached + unref，脱离父进程独立存活
  try { tunnelInfo.process.unref(); } catch { /* ignore */ }

  return { url: tunnelInfo.url, containerName, port: hostPort, workDir: sourceDir, tunnelProcess: tunnelInfo.process };
}

// ---------------------------------------------------------------------------
// 文件浏览 RPC：让前端抽屉浏览 agent 所在机器上 git 工作区的文件。
// 复用 OPEN_PATH 同步 RPC 通道（taskID promise + task.dispatch + task.complete）。
// 后端发 task.dispatch(cli_tool=__agenthub_browse_files__)，prompt 是 JSON payload。
// 这里只做只读 + git diff 识别改动文件，不做编辑/写入。
// 所有工具名/超时/大小上限/排除目录在顶部 CONFIG.browse 配置。
// ---------------------------------------------------------------------------


// 同步跑 git 命令（带超时），返回 stdout 字符串；失败抛错。
function runGitSync(cwd, args) {
  try {
    return execFileSync('git', args, {
      cwd: cwd || undefined,
      encoding: 'utf8',
      timeout: BROWSE_GIT_TIMEOUT_MS,
      stdio: ['ignore', 'pipe', 'pipe'],
      windowsHide: true,
    }).replace(/\r?\n$/, ''); // 只去尾部换行，不动行首空格（porcelain 首列可能是空格，trim 会吃掉）
  } catch (err) {
    throw new Error(`git ${args.join(' ')} failed: ${errorMessage(err)}`);
  }
}

// 返回 git 仓库根目录绝对路径；非 git 仓库或出错返回 null。
function gitRepoRoot(cwd) {
  try {
    const root = runGitSync(cwd, ['rev-parse', '--show-toplevel']);
    return root || null;
  } catch { return null; }
}

// 在 task dispatch 前，若 workdir 不是 git 仓库则自动 git init + baseline commit。
// 这样 agent 后续跑 git diff 才能拿到 HEAD 对比，不会因为非 git 仓库直接 fatal。
//
// 行为：
// - 已是 git 仓库：no-op
// - 非 git 仓库：git init + 配置 user.email/user.name + add -A + commit baseline
// - 任何环节失败：logFlow warn，不阻塞 task 执行（agent 可能不需要 git）
//
// taskMeta 用于日志关联（task_id / cli_tool 等），可省略。
function ensureGitRepoForTask(workDir, taskMeta = {}) {
  if (!workDir) return;
  const root = gitRepoRoot(workDir);
  if (root) return; // 已是 git 仓库，跳过
  const logFields = { ...taskMeta, work_dir: workDir };
  try {
    runGitSync(workDir, ['init']);
    // 配置本仓库 user.email/user.name，避免后续 commit 因缺少 identity 报错。
    try { runGitSync(workDir, ['config', 'user.email', 'agenthub@local']); } catch { /* 已有 global identity 也行 */ }
    try { runGitSync(workDir, ['config', 'user.name', 'AgentHub']); } catch { /* 同上 */ }
    // 只在有内容时才 commit，避免空目录 commit 报 "nothing to commit"。
    let hasContent = false;
    try {
      const status = runGitSync(workDir, ['status', '--porcelain', '--untracked-files=all']);
      hasContent = Boolean(status && status.trim());
    } catch { /* status 失败也无所谓，下面 commit 失败会被外层吞掉 */ }
    if (hasContent) {
      runGitSync(workDir, ['add', '-A']);
      runGitSync(workDir, ['commit', '-m', 'baseline (auto)']);
    }
    logFlow('info', 'task.git_init_baseline', logFields);
  } catch (err) {
    logFlow('warn', 'task.git_init_failed', { ...logFields, error: errorMessage(err) });
  }
}

// 解析 git status 字母为前端统一的状态枚举。
// 接受 porcelain 的两列状态码 "XY"（X=staged，Y=工作区），也可接受单字符（diff --name-status）。
// 优先级：删除 > 新增 > 修改；未跟踪 "??" 归 added。两列任一非空格即取该状态。
function parseGitStatus(code) {
  if (!code) return 'modified';
  const s = String(code).trim();
  if (!s) return 'modified';
  // "??" 未跟踪 → added（仅 porcelain 会出现）
  if (s[0] === '?' || s[1] === '?') return 'added';
  // 合并两列，逐字符判定，删除/新增优先于修改
  const has = (ch) => s[0] === ch || s[1] === ch;
  if (has('D')) return 'deleted';
  if (has('A')) return 'added';
  return 'modified'; // M/R/C/U/其他统一归为 modified
}

// 列出工作区改动文件（已暂存 + 未暂存 + 未跟踪）。
// 返回 [{ path, status }]，path 为相对 repoRoot 的 posix 路径。
function gitChangedFiles(cwd, rev) {
  let lines;
  if (rev) {
    // 指定 base rev：对比 rev..HEAD 的已提交改动
    const out = runGitSync(cwd, ['diff', '--name-status', `${rev}..HEAD`]);
    lines = out ? out.split('\n') : [];
  } else {
    // 工作区 vs HEAD：含已暂存、未暂存、未跟踪（--porcelain 已覆盖这三种）
    const out = runGitSync(cwd, ['status', '--porcelain', '--untracked-files=all']);
    lines = out ? out.split('\n') : [];
  }
  const files = [];
  for (const line of lines) {
    if (!line) continue;
    // porcelain 格式："XY filename"，XY 两列状态码
    // name-status 格式："X\tpath" 或 "R100\told\tnew"（重命名取新路径）
    if (line.length >= 3 && line[2] === ' ') {
      // porcelain：XY filename —— 传两列状态码，parseGitStatus 合并 staged+工作区
      files.push({ path: line.slice(3).trim(), status: parseGitStatus(line.slice(0, 2)) });
    } else if (line.includes('\t')) {
      // name-status：X\tpath（重命名取第二列）
      const [code, ...rest] = line.split('\t');
      const target = rest.length > 1 ? rest[rest.length - 1] : rest[0];
      files.push({ path: target.trim(), status: parseGitStatus(code) });
    }
  }
  return files;
}

// 列单层目录，过滤隐藏文件 + BROWSE_EXCLUDE_DIRS。
// 返回 [{ name, type: 'file'|'dir', size? }]，size 仅文件提供（字节）。
function listDirAbs(absDir) {
  let entries;
  try { entries = fs.readdirSync(absDir, { withFileTypes: true }); }
  catch { return []; }
  const result = [];
  for (const entry of entries) {
    if (entry.name.startsWith('.')) continue;
    const isDir = entry.isDirectory();
    const isFile = entry.isFile();
    if (!isDir && !isFile) continue; // 跳过符号链接/设备等
    if (isDir && BROWSE_EXCLUDE_DIRS.has(entry.name)) continue;
    const item = { name: entry.name, type: isDir ? 'dir' : 'file' };
    if (isFile) {
      try { item.size = fs.statSync(path.join(absDir, entry.name)).size; } catch { /* 忽略 stat 失败 */ }
    }
    result.push(item);
  }
  // 目录在前，文件在后；同类按名称排序
  result.sort((a, b) => {
    if (a.type !== b.type) return a.type === 'dir' ? -1 : 1;
    return a.name.localeCompare(b.name);
  });
  return result;
}

// 校验 target 是否在 repoRoot 内（防 ../ 越狱）。
// 返回归一化后的绝对路径；非法返回 null。
function safePathWithin(repoRoot, target) {
  if (!target || !repoRoot) return null;
  const resolved = path.resolve(repoRoot, target);
  const rel = path.relative(repoRoot, resolved);
  if (rel.startsWith('..') || path.isAbsolute(rel)) return null; // 越狱或绝对路径外的
  return resolved;
}

// 二进制检测：含 NUL 字节视为二进制（不预览）。
function looksBinary(buf) {
  for (let i = 0; i < buf.length; i++) {
    if (buf[i] === 0) return true;
    if (i > 8192) break; // 只检查前 8KB，避免大文件全扫
  }
  return false;
}

// 解析 git log 自定义 format 输出（%x1e 记录分隔、%x1f 字段分隔）。
// 返回 [{hash, timestamp, author, subject}]，按 git log 原序（最新在前）。
function parseGitLog(out) {
  if (!out) return [];
  const records = out.split('\x1e');
  const commits = [];
  for (const rec of records) {
    const trimmed = rec.replace(/^\n+|\n+$/g, '');
    if (!trimmed) continue;
    const [hash, ts, author, subject] = trimmed.split('\x1f');
    if (!hash) continue;
    commits.push({
      hash,
      timestamp: Number(ts) || 0,
      author: author || '',
      subject: subject || '',
    });
  }
  return commits;
}

// 递归收集目录下所有文件（用于打包），返回 [{ absPath, relPath, size }]。
// 排除隐藏文件 + BROWSE_EXCLUDE_DIRS；累计超 BROWSE_ZIP_MAX_TOTAL_SIZE 时停止。
function collectForZip(rootDir) {
  const files = [];
  let total = 0;
  let truncated = false;
  const walk = (dir, relBase) => {
    let entries;
    try { entries = fs.readdirSync(dir, { withFileTypes: true }); }
    catch { return; }
    for (const entry of entries) {
      if (truncated) return;
      if (entry.name.startsWith('.')) continue;
      const full = path.join(dir, entry.name);
      const rel = relBase ? `${relBase}/${entry.name}` : entry.name;
      if (entry.isDirectory()) {
        if (BROWSE_EXCLUDE_DIRS.has(entry.name)) continue;
        walk(full, rel);
      } else if (entry.isFile()) {
        try {
          const stat = fs.statSync(full);
          if (total + stat.size > BROWSE_ZIP_MAX_TOTAL_SIZE) { truncated = true; return; }
          total += stat.size;
          files.push({ absPath: full, relPath: rel, size: stat.size });
        } catch { /* 忽略不可读文件 */ }
      }
    }
  };
  walk(rootDir, '');
  return { files, truncated };
}

/**
 * 文件浏览 RPC handler。prompt 是 JSON 字符串：{ cwd?, action, path?, rev? }
 * action: 'tree' | 'list' | 'read' | 'zip'
 *   tree —— 根目录快照 + git 改动文件清单（打开抽屉时调）
 *   list —— 单层展开子目录（懒加载）
 *   read —— 读单文件内容（点文件查看）
 *   zip  —— 收集整目录文件数组（后端打包）
 * 返回值会被 executeTask 当作 task 结果回传给后端（JSON 字符串）。
 */
function browseFiles(prompt) {
  let payload = null;
  try {
    payload = JSON.parse(prompt);
  } catch {
    throw new Error('Invalid browse files payload');
  }
  const action = String(payload.action || '').trim();
  // work_dir 来自前端 project 卡片（解耦：路径生产与文件浏览分离），不再 fallback process.cwd()。
  const workDir = String(payload.work_dir || '').trim();
  if (!workDir) throw new Error('work_dir is required');

  // 非 git 仓库时只支持裸目录浏览（无 changedFiles / log / show）
  const repoRoot = gitRepoRoot(workDir);
  logFlow('info', 'browse.tree', { work_dir: workDir, repo_root: repoRoot, is_git: Boolean(repoRoot) });
  if (action === 'tree') {
    const root = repoRoot || path.resolve(workDir);
    const rootEntries = listDirAbs(root);
    logFlow('info', 'browse.tree_result', { root, entry_count: rootEntries.length });
    return JSON.stringify({
      repoRoot: root,
      isGit: Boolean(repoRoot),
      changedFiles: repoRoot ? gitChangedFiles(workDir, payload.rev) : [],
      rootEntries,
    });
  }

  // status 不需要单文件 path（用的是 files 数组），提前走自己的分支，避免被下方共享 path 校验挡住。
  // 复用 gitChangedFiles 拿全部改动文件后按 wanted 过滤，避免重复 porcelain 解析逻辑。
  if (action === 'status') {
    if (!repoRoot) return JSON.stringify({ statuses: [] });
    const rawFiles = payload.files;
    const fileList = Array.isArray(rawFiles) ? rawFiles.map(String)
      : (typeof rawFiles === 'string' && rawFiles ? rawFiles.split(',').map(s => s.trim()).filter(Boolean)
        : []);
    const wanted = new Set(fileList);
    if (wanted.size === 0) return JSON.stringify({ statuses: [] });
    const all = gitChangedFiles(repoRoot); // rev 不传 = 工作区状态（porcelain）
    return JSON.stringify({ statuses: all.filter((f) => wanted.has(f.path)) });
  }

  // 以下 action 都要求 path 必须落在 repoRoot（或 workDir 兜底）内
  const baseRoot = repoRoot || path.resolve(workDir);
  const target = safePathWithin(baseRoot, payload.path);
  if (!target) {
    throw new Error('Invalid or out-of-root path');
  }

  if (action === 'list') {
    let stat;
    try { stat = fs.statSync(target); }
    catch { throw new Error(`Path not found: ${payload.path}`); }
    if (!stat.isDirectory()) throw new Error('Not a directory');
    return JSON.stringify(listDirAbs(target));
  }

  if (action === 'read') {
    let stat;
    try { stat = fs.statSync(target); }
    catch { throw new Error(`File not found: ${payload.path}`); }
    if (!stat.isFile()) throw new Error('Not a file');
    if (stat.size > BROWSE_FILE_READ_MAX_SIZE) {
      return JSON.stringify({ path: payload.path, content: '', size: stat.size, binary: false, tooLarge: true });
    }
    const buf = fs.readFileSync(target);
    if (looksBinary(buf)) {
      return JSON.stringify({ path: payload.path, content: '', size: stat.size, binary: true });
    }
    return JSON.stringify({ path: payload.path, content: buf.toString('utf8'), size: stat.size, binary: false });
  }

  if (action === 'zip') {
    // 不在此打包：返回文件数组（含 base64 内容），由后端 Go archive/zip 打包。
    // 后端拿 rawBytes 太重，这里返回结构化数据，后端重建文件树写 zip。
    let stat;
    try { stat = fs.statSync(target); }
    catch { throw new Error(`Path not found: ${payload.path}`); }
    if (!stat.isDirectory()) throw new Error('Not a directory');
    const { files, truncated } = collectForZip(target);
    // 每个文件单独读，base64 编码（避免 JSON 里嵌二进制）
    const encoded = files.map((f) => {
      const content = fs.readFileSync(f.absPath);
      return { path: f.relPath, content_b64: content.toString('base64'), size: f.size };
    });
    return JSON.stringify({ baseDir: payload.path, files: encoded, truncated });
  }

  // git 历史：某文件的 commit 列表（供前端版本切换）。
  // 用 --follow 跟踪重命名；自定义 format 以 \x1e 分隔记录、\x1f 分隔字段。
  // 注意 cwd 必须用 repoRoot 而非 workDir——rel 是相对 repoRoot 算的，
  // 若 workDir 是仓库子目录，用 workDir 作 cwd 会导致路径偏移查错文件。
  if (action === 'log') {
    if (!repoRoot) return JSON.stringify({ commits: [] });
    const target = safePathWithin(repoRoot, payload.path);
    if (!target) throw new Error('Invalid or out-of-root path');
    let stat;
    try { stat = fs.statSync(target); } catch { throw new Error(`Path not found: ${payload.path}`); }
    if (!stat.isFile()) throw new Error('Not a file');
    const rel = path.relative(repoRoot, target);
    const out = runGitSync(repoRoot, [
      'log', '--follow', '--no-color', '-n', '50',
      '--format=%H%x1f%ct%x1f%an%x1f%s%x1e', '--', rel,
    ]);
    return JSON.stringify({ commits: parseGitLog(out) });
  }

  // 读某 commit 下某文件的内容（git show <rev>:<path>）。同样用 repoRoot 作 cwd。
  // 非 git 目录没有 commit 历史，无法解析 rev，给出明确错误而非含糊的 "Not a git repo"。
  if (action === 'show') {
    if (!repoRoot) throw new Error('File history requires git repo');
    if (!payload.rev) throw new Error('rev is required for show');
    const target = safePathWithin(repoRoot, payload.path);
    if (!target) throw new Error('Invalid or out-of-root path');
    const rel = path.relative(repoRoot, target);
    const out = runGitSync(repoRoot, ['show', `${payload.rev}:${rel}`]);
    const buf = Buffer.from(out);
    if (buf.length > BROWSE_FILE_READ_MAX_SIZE) {
      return JSON.stringify({ path: payload.path, content: '', size: buf.length, binary: false, tooLarge: true });
    }
    if (looksBinary(buf)) {
      return JSON.stringify({ path: payload.path, content: '', size: buf.length, binary: true });
    }
    return JSON.stringify({ path: payload.path, content: out, size: buf.length, binary: false });
  }

  // 拿某文件的前后内容（默认工作区 vs HEAD）——供 diff 卡片点击文件后做对比。
  // 一次调用拿全 oldContent/newContent，前端不必分别 fileShow 两次。
  // 非 git 目录优雅降级：oldContent 留空（无历史），newContent 读工作区文件内容（若存在），
  // 这样 diff 卡片仍可展示为"新增文件"。
  if (action === 'diff') {
    const oldRev = payload.old_rev || 'HEAD';
    const newRev = payload.new_rev || '';  // 空 = 工作区当前版本
    if (!repoRoot) {
      // 非 git 仓库：用 workDir 兜底解析 path，oldContent 留空，newContent 读工作区文件。
      const nonGitTarget = safePathWithin(baseRoot, payload.path);
      if (!nonGitTarget) throw new Error('Invalid or out-of-root path');
      let newContent = '';
      if (!newRev) {
        try {
          const buf = fs.readFileSync(nonGitTarget);
          if (buf.length <= BROWSE_FILE_READ_MAX_SIZE && !looksBinary(buf)) {
            newContent = buf.toString('utf8');
          }
        } catch { /* 工作区无此文件，newContent 留空 */ }
      }
      return JSON.stringify({ path: payload.path, oldContent: '', newContent });
    }
    const target = safePathWithin(repoRoot, payload.path);
    if (!target) throw new Error('Invalid or out-of-root path');
    const rel = path.relative(repoRoot, target);
    // 旧版本：git show <rev>:<path>
    let oldContent = '';
    try { oldContent = runGitSync(repoRoot, ['show', `${oldRev}:${rel}`]); }
    catch { /* 新增文件在 oldRev 不存在，oldContent 留空 */ }
    // 新版本：默认读工作区文件；指定 rev 则 git show
    let newContent = '';
    if (!newRev) {
      try {
        const buf = fs.readFileSync(target);
        newContent = looksBinary(buf) ? '' : buf.toString('utf8');
      } catch { /* 文件已删除，newContent 留空 */ }
    } else {
      try { newContent = runGitSync(repoRoot, ['show', `${newRev}:${rel}`]); }
      catch { /* rev 不存在该文件 */ }
    }
    return JSON.stringify({ path: payload.path, oldContent, newContent });
  }

  throw new Error(`Unknown browse action: ${action}`);
}

function logValue(value) {
  if (value === undefined || value === null) return '';
  if (typeof value === 'number') return String(value);
  if (typeof value === 'boolean') return String(value);
  if (Array.isArray(value) || typeof value === 'object') return JSON.stringify(value);
  const text = String(value);
  return /^[A-Za-z0-9._:/@=-]+$/.test(text) ? text : JSON.stringify(text);
}

function logFlow(level, stage, fields = {}) {
  const pairs = Object.entries(fields)
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .map(([key, value]) => `${key}=${logValue(value)}`);
  const line = [
    new Date().toISOString(),
    DAEMON_LOG_EVENT,
    `level=${level}`,
    `stage=${stage}`,
    ...pairs,
  ].join(' ');
  if (level === 'error' || level === 'warn' || process.argv.includes('--mcp')) {
    console.error(line);
    return;
  }
  console.log(line);
}

function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}

function firstLine(value) {
  return String(value || '').split(/\r?\n/)[0].trim();
}

let WebSocket;
try {
  WebSocket = require('ws');
} catch {
  // Node 22+ has global WebSocket
  if (typeof globalThis.WebSocket !== 'undefined') {
    WebSocket = globalThis.WebSocket;
  }
}

function safeSend(ws, data) {
  try {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(data);
      return true;
    }
  } catch { /* connection already closed */ }
  return false;
}

function sendTaskComplete(data) {
  const taskId = data && data.task_id;
  if (!taskId) return;
  logFlow('info', 'task.complete_SEND_CALLED', {
    task_id: taskId,
    has_error: Boolean(data.error),
    result_len: typeof data.result === 'string' ? data.result.length : 0,
    already_pending: pendingTaskCompletions.has(taskId),
  });
  const envelope = JSON.stringify({ type: 'task.complete', data });
  if (safeSend(currentDaemonWs, envelope)) {
    pendingTaskCompletions.delete(taskId);
    logFlow('info', 'task.complete_sent', {
      task_id: taskId,
      has_error: Boolean(data.error),
      result_len: typeof data.result === 'string' ? data.result.length : 0,
      artifact_count: Array.isArray(data.artifacts) ? data.artifacts.length : 0,
      pending_count: pendingTaskCompletions.size,
    });
    return;
  }
  pendingTaskCompletions.set(taskId, data);
  logFlow('warn', 'task.complete_buffered', {
    task_id: taskId,
    has_error: Boolean(data.error),
    pending_count: pendingTaskCompletions.size,
  });
}

/**
 * sendTaskProgress —— 把流式增量（AgentEvent 批量）发给后端。
 *
 * 与 sendTaskComplete 不同：
 * - 流式期间 WS 断开时丢弃（不 buffer），因为增量是"短暂信息"，重连后可继续。
 * - 日志只打 event 数和字节数，不打具体内容（避免日志爆炸）。
 */
function sendTaskProgress(data) {
  const taskId = data && data.task_id;
  if (!taskId) return;
  const events = Array.isArray(data.events) ? data.events : [];
  if (events.length === 0) return;
  const envelope = JSON.stringify({ type: 'task.progress', data: { ...data, events } });
  const sent = safeSend(currentDaemonWs, envelope);
  if (sent) {
    logFlow('info', 'task.progress_sent', {
      task_id: taskId,
      event_count: events.length,
      bytes: envelope.length,
    });
  }
}

function flushPendingTaskCompletions() {
  if (pendingTaskCompletions.size > 0) {
    logFlow('info', 'task.complete_flush_start', { pending_count: pendingTaskCompletions.size });
  }
  for (const data of pendingTaskCompletions.values()) {
    sendTaskComplete(data);
  }
}

// ---------------------------------------------------------------------------
// WS 消息处理器注册表
// 替代旧的 if-ladder 分发。新增消息类型只需 registerWsHandler('type', fn)。
// ---------------------------------------------------------------------------

const wsHandlers = new Map();

function registerWsHandler(type, handler) {
  wsHandlers.set(type, handler);
}

function dispatchWsMessage(ws, envelope) {
  const handler = wsHandlers.get(envelope.type);
  if (handler) {
    return handler(ws, envelope.data || {});
  }
  return false;
}

// ---------------------------------------------------------------------------
// 事件总线
// 解耦 WS 消息接收 → 业务执行 → 结果回传的链条。
// 每一步通过 bus.emit / bus.on 通信，新增横切逻辑（metrics/审计/重试）
// 只需加监听器，不改核心流程。
//
// 事件清单：
//   ws.connected         — WS 连接成功
//   ws.disconnected      — WS 断开
//   ws.message           — 收到任意 WS 消息（raw envelope）
//   agent.start_request  — 后端要求启动 agent
//   agent.stop_request   — 后端要求停止 agent
//   agent.restart_request— 后端要求重启 agent
//   agent.started        — agent 进程已启动
//   agent.stopped        — agent 进程已退出
//   task.dispatch        — 收到任务派发
//   task.completed       — 任务执行成功
//   task.failed          — 任务执行失败
//   daemon.ready         — daemon 完成初始化
// ---------------------------------------------------------------------------

const bus = new EventEmitter();
bus.setMaxListeners(50); // 横切监听器可能较多

// Task dispatch dedup：同一 agent + prompt 在 3 秒内不重复执行。
const recentDispatches = new Map();
// 定期清理过期条目
setInterval(() => {
  const now = Date.now();
  for (const [key, val] of recentDispatches) {
    if (now - val.ts > 10000) recentDispatches.delete(key);
  }
}, 10000).unref();

// 结构化日志：所有 daemon 事件自动写入 logFlow
bus.on('agent.started', (info) =>
  logFlow('info', 'agent.started', info),
);
bus.on('agent.stopped', (info) =>
  logFlow('info', 'agent.stopped', info),
);
bus.on('task.completed', (info) =>
  logFlow('info', 'task.completed', info),
);
bus.on('task.failed', (info) =>
  logFlow('error', 'task.failed', info),
);

function onWebSocket(ws, eventName, handler) {
  if (typeof ws.on === 'function') {
    ws.on(eventName, handler);
    return;
  }
  if (typeof ws.addEventListener !== 'function') {
    throw new Error('unsupported WebSocket implementation');
  }
  ws.addEventListener(eventName, (event) => {
    if (eventName === 'message') {
      handler(event.data);
      return;
    }
    if (eventName === 'close') {
      handler(event.code, event.reason);
      return;
    }
    if (eventName === 'error') {
      handler(event.error || event);
      return;
    }
    handler(event);
  });
}

const activeSessions = new Map();
const runningAgents = new Map(); // agentID → { process, sessionId, cliTool, sendPrompt, _queue }
const idleAgentConfigs = new Map(); // agentID → { cliTool, sessionId, systemPrompt }
const agentTurnStates = new Map(); // agentID → 'idle' | 'active'
let currentDaemonWs = null;
const activeTaskIDs = new Set();
const completedTaskIDs = new Set();
const pendingTaskCompletions = new Map(); // taskID → task.complete data, flushed after WS reconnect

// Per-conversation session mapping: `${agent_id}:${conversation_id}` → sessionId
// 持久化路径在顶部 CONFIG.sessionsFile。
const conversationSessions = new Map();

function loadSessionMap() {
  try {
    const data = fs.readFileSync(SESSIONS_FILE, 'utf8');
    for (const [key, value] of Object.entries(JSON.parse(data))) {
      conversationSessions.set(key, value);
    }
  } catch { /* file not found or invalid — start fresh */ }
}

function saveSessionMap() {
  try {
    const obj = Object.fromEntries(conversationSessions);
    fs.mkdirSync(path.dirname(SESSIONS_FILE), { recursive: true });
    fs.writeFileSync(SESSIONS_FILE, JSON.stringify(obj, null, 2));
  } catch (err) {
    logFlow('warn', 'session_map.save_failed', { file: SESSIONS_FILE, error: errorMessage(err) });
  }
}

// agent 启动串行化队列（避免并发启动多个 CLI 实例冲突）；间隔在顶部 CONFIG.startQueueIntervalMs。
let lastAgentStartAt = 0;
const agentStartQueue = [];

// 轮询模式下的后端连接信息，供派发任务时给 Claude Code 注入平台 MCP server。
const daemonConn = { serverURL: '', apiKey: '', daemonToken: '' };

// buildPlatformMcpServerArgs builds the daemon --mcp invocation for the current
// AgentHub task. Passing conversation/user/agent IDs here gives MCP tools a default
// group context, matching Claude Code's per-task injection behavior.
//
// taskId 用于 MCP subprocess emit 卡片时回传 task_id 给后端
// （POST /api/internal/task-cards）。不传则卡片 emit 无效——subprocess 拿不到 task 标识。
function buildPlatformMcpServerArgs(conversationId, userId, agentId, taskId) {
  if (!daemonConn.serverURL || !daemonConn.apiKey) return [];
  const mcpServerArgs = [__filename, '--server-url', daemonConn.serverURL, '--api-key', daemonConn.apiKey, '--mcp'];
  if (daemonConn.daemonToken) mcpServerArgs.push('--daemon-token', daemonConn.daemonToken);
  if (conversationId) mcpServerArgs.push('--conversation-id', conversationId);
  if (userId) mcpServerArgs.push('--user-id', userId);
  if (agentId) mcpServerArgs.push('--agent-id', agentId);
  if (taskId) mcpServerArgs.push('--task-id', taskId);
  return mcpServerArgs;
}

// buildPlatformMcpArgs generates Claude Code MCP injection args for this task.
function buildPlatformMcpArgs(conversationId, userId, agentId, taskId) {
  const mcpServerArgs = buildPlatformMcpServerArgs(conversationId, userId, agentId, taskId);
  if (mcpServerArgs.length === 0) return [];
  const mcpConfig = JSON.stringify({
    mcpServers: {
      'agenthub-platform': {
        command: 'node',
        args: mcpServerArgs,
      },
    },
  });
  return ['--mcp-config', mcpConfig, '--allowedTools', 'mcp__agenthub-platform'];
}

function buildAgentHubContextEnv(conversationId, userId, agentId, taskId) {
  const env = {};
  if (conversationId) env.AGENTHUB_CONVERSATION_ID = conversationId;
  if (userId) env.AGENTHUB_USER_ID = userId;
  if (agentId) env.AGENTHUB_AGENT_ID = agentId;
  if (taskId) env.AGENTHUB_TASK_ID = taskId;
  return Object.keys(env).length > 0 ? env : undefined;
}

function opencodeContextChanged(task, savedSessionId) {
  if (!savedSessionId) return false;
  return Boolean(task && task.agent_id && task.conversation_id);
}

// ensureGlobalMcpConfigs 为不支持按次注入的 CLI（openclaw/codex/opencode）在启动时
// 幂等写入全局 MCP 配置，把本 daemon 以 --mcp 模式注册为 agenthub-platform server。
// 仅对本机实际安装的 CLI 生效，失败仅告警、不影响 daemon 连接。
// 改造后：遍历所有已注册 CliToolSpec，调用其可选 ensureMcp(mcpArgs) 钩子。
function ensureGlobalMcpConfigs(serverURL, apiKey) {
  const mcpArgs = [__filename, '--server-url', serverURL, '--api-key', apiKey, '--mcp'];
  if (daemonConn.daemonToken) mcpArgs.push('--daemon-token', daemonConn.daemonToken);
  for (const spec of cliTools.allCliTools()) {
    if (typeof spec.ensureMcp === 'function') spec.ensureMcp(mcpArgs);
  }
}

function openCodeConfigPath() {
  if (process.env.AGENTHUB_OPENCODE_CONFIG) return process.env.AGENTHUB_OPENCODE_CONFIG;
  return path.join(os.homedir(), '.config', 'opencode', 'opencode.json');
}

function ensureOpenCodeMcpConfig(command) {
  const configPath = openCodeConfigPath();
  let config = {};
  if (fs.existsSync(configPath)) {
    const raw = fs.readFileSync(configPath, 'utf8');
    config = raw.trim() ? JSON.parse(raw) : {};
    if (!config || typeof config !== 'object' || Array.isArray(config)) {
      throw new Error('OpenCode config root must be a JSON object');
    }
  }
  if (!config.$schema) config.$schema = 'https://opencode.ai/config.json';
  if (!config.mcp || typeof config.mcp !== 'object' || Array.isArray(config.mcp)) config.mcp = {};
  config.mcp['agenthub-platform'] = {
    type: 'local',
    command,
    enabled: true,
  };
  fs.mkdirSync(path.dirname(configPath), { recursive: true });
  fs.writeFileSync(configPath, `${JSON.stringify(config, null, 2)}\n`);
  return configPath;
}

function killSessionProcess(sessionId) {
  const child = activeSessions.get(sessionId);
  if (!child) return;
  try {
    if (process.platform === 'win32') {
      spawn('taskkill', ['/pid', String(child.pid), '/T', '/F'], { windowsHide: true });
    } else {
      process.kill(-child.pid, 'SIGKILL');
    }
    logFlow('info', 'process.kill_session', { session_id: sessionId, pid: child.pid });
  } catch { /* already dead */ }
  activeSessions.delete(sessionId);
}

// OPEN_PATH 同步 RPC（打开 skill 所在目录）的工具名/超时/最小描述长度在顶部 CONFIG 配置。

// CANDIDATES 现在由 CliToolSpec Registry 派生（见 initCliTools 调用点）。
// 每次调用都重新映射，确保注册表变化（测试替换 spec）即时反映。
function getCandidates() {
  return cliTools.allCliTools().map((spec) => ({
    name: spec.name,
    cli_tool: spec.cliTool,
    capabilities: spec.defaultCapabilities,
  }));
}

function defaultSkills(names) {
  return names.map((name) => ({
    name,
    description: defaultSkillDescription(name),
    auto: true,
  }));
}

function defaultSkillDescription(name) {
  const descriptions = {
    coding: 'Handle local coding tasks such as implementation, refactoring, and project edits.',
    review: 'Review code changes, identify risks, and suggest focused improvements.',
    orchestration: 'Coordinate multi-step work and route tasks across local Agent workflows.',
  };
  return descriptions[name] || `Provides the ${name} skill for local Agent workflows.`;
}

function npmWrapperScript(command) {
  if (process.platform !== 'win32') return null;
  const root = process.env.APPDATA;
  if (!root) return null;
  const scripts = {
    claude: path.join(root, 'npm', 'node_modules', '@anthropic-ai', 'claude-code', 'cli.js'),
    openclaw: path.join(root, 'npm', 'node_modules', 'openclaw', 'openclaw.mjs'),
  };
  const script = scripts[command];
  return script && fs.existsSync(script) ? script : null;
}

function processSpec(command, args) {
  const wrapperScript = npmWrapperScript(command);
  if (wrapperScript) {
    return { command: 'node', args: [wrapperScript, ...args] };
  }
  if (process.platform === 'win32' && !command.toLowerCase().endsWith('.exe')) {
    return { command: 'cmd.exe', args: ['/d', '/s', '/c', command, ...args] };
  }
  return { command, args };
}

function readArg(name) {
  const prefix = `${name}=`;
  for (let i = 2; i < process.argv.length; i += 1) {
    const current = process.argv[i];
    if (current === name) {
      return process.argv[i + 1] || '';
    }
    if (current.startsWith(prefix)) {
      return current.slice(prefix.length);
    }
  }
  return '';
}

function sleep(ms) {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

function makeSessionId(conversationID, agentID) {
  const namespace = 'a1b2c3d4-e5f6-7890-abcd-ef1234567890';
  const name = `conv:${conversationID}:agent:${agentID}`;
  const hash = crypto.createHash('sha1');
  hash.update(Buffer.from(namespace.replace(/-/g, ''), 'hex'));
  hash.update(name);
  const digest = hash.digest();
  digest[6] = (digest[6] & 0x0f) | 0x50;
  digest[8] = (digest[8] & 0x3f) | 0x80;
  const hex = digest.toString('hex', 0, 16);
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20, 32)}`;
}

function sessionKeyForTask(task) {
  return task && task.agent_id && task.conversation_id ? `${task.agent_id}:${task.conversation_id}` : '';
}

function commandOutput(command, args, timeout = 5000) {
  try {
    const spec = processSpec(command, args);
    return execFileSync(spec.command, spec.args, {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
      timeout,
      windowsHide: true,
    }).trim();
  } catch {
    return null;
  }
}

function commandVersion(command) {
  return commandOutput(command, ['--version']);
}

function codexLoginStatus(command) {
  const spec = processSpec(command, ['login', 'status']);
  const result = spawnSync(spec.command, spec.args, {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
    timeout: 5000,
    windowsHide: true,
  });
  if (result.status !== 0) return null;
  return `${result.stdout || ''}${result.stderr || ''}`.trim();
}

function existingFile(value) {
  return value && fs.existsSync(value) ? value : null;
}

function codexLocalInstallPaths() {
  if (process.platform !== 'win32') return [];
  const root = process.env.LOCALAPPDATA;
  if (!root) return [];
  const binRoot = path.join(root, 'OpenAI', 'Codex', 'bin');
  try {
    return fs.readdirSync(binRoot, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => path.join(binRoot, entry.name, 'codex.exe'))
      .filter((candidate) => fs.existsSync(candidate))
      .sort((a, b) => {
        try {
          return fs.statSync(b).mtimeMs - fs.statSync(a).mtimeMs;
        } catch {
          return 0;
        }
      });
  } catch {
    return [];
  }
}

function codexExtensionPath() {
  if (process.platform !== 'win32') return null;
  const root = process.env.USERPROFILE;
  if (!root) return null;
  const extensionRoot = path.join(root, '.vscode', 'extensions');
  try {
    const matches = fs.readdirSync(extensionRoot)
      .filter((name) => name.startsWith('openai.chatgpt-'))
      .sort()
      .reverse();
    for (const match of matches) {
      const candidate = path.join(extensionRoot, match, 'bin', 'windows-x86_64', 'codex.exe');
      if (fs.existsSync(candidate)) return candidate;
    }
  } catch {
    return null;
  }
  return null;
}

function resolveCommand(cliTool) {
  // Switch 1: 委托给 CliToolSpec.resolveCommand（每个 spec 内部实现多路径 fallback）。
  // 未注册的 cli_tool 退化为字面量，与原 if-else 链未匹配分支行为一致。
  // 注：codexLocalInstallPaths / codexExtensionPath / existingFile / commandVersion 等
  // 辅助函数仍保留——它们通过 initCliToolsCtx 注入到 spec 内部使用。
  // Switch 7: resolveCodexCommand / resolveOpenCodeCommand 顶层 wrapper 已删除
  // （spec.resolveCommand 自己负责多路径 fallback）。
  const spec = cliTools.getCliTool(cliTool);
  if (spec && typeof spec.resolveCommand === 'function') {
    return spec.resolveCommand(initCliToolsCtx);
  }
  return cliTool;
}

function scanAgents() {
  return getCandidates()
    .map((candidate) => {
      const spec = cliTools.getCliTool(candidate.cli_tool);
      const command = resolveCommand(candidate.cli_tool);
      // 保留原 codex 调试日志（onResolvedCommand 钩子，仅 codex 实现）。
      if (spec && typeof spec.onResolvedCommand === 'function') {
        spec.onResolvedCommand(command);
      }
      const version = commandVersion(command);
      if (version === null) return null;
      // 登录态检测委托给 spec.isAuthenticated（仅 codex 实现 isCodexAuthenticated）。
      if (spec && typeof spec.isAuthenticated === 'function' && !spec.isAuthenticated(command)) return null;
      const skills = scanSkills(candidate.cli_tool);
      return {
        name: candidate.name,
        cli_tool: candidate.cli_tool,
        version,
        capabilities: skills.length > 0 ? skills : candidate.capabilities,
      };
    })
    .filter(Boolean);
}

function scanSkills(cliTool) {
  const skills = [];
  const seen = new Set();
  for (const root of skillRoots(cliTool)) {
    for (const skillPath of findSkillFiles(root)) {
      let content = '';
      try {
        content = fs.readFileSync(skillPath, 'utf8');
      } catch {
        continue;
      }
      const skill = parseSkillFile(path.basename(path.dirname(skillPath)), skillPath, content);
      const key = skill.name.toLowerCase();
      if (!key || seen.has(key)) continue;
      seen.add(key);
      skills.push(skill);
    }
  }
  return skills;
}

function findSkillFiles(root) {
  const results = [];
  function walk(current) {
    let entries = [];
    try {
      entries = fs.readdirSync(current, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      const entryPath = path.join(current, entry.name);
      if (entry.isDirectory()) {
        if (entry.name === '.git') continue;
        walk(entryPath);
        continue;
      }
      if (entry.isFile() && entry.name === 'SKILL.md') {
        results.push(entryPath);
      }
    }
  }
  walk(root);
  return results;
}

function addRoot(roots, root) {
  if (root && !roots.includes(root)) roots.push(root);
}

function isAgentHubWorkspace(root) {
  const daemonPackage = path.join(root, 'src', 'daemon-npm', 'package.json');
  const frontendPackage = path.join(root, 'src', 'frontend', 'package.json');
  if (!fs.existsSync(daemonPackage) || !fs.existsSync(frontendPackage)) return false;
  try {
    const pkg = JSON.parse(fs.readFileSync(daemonPackage, 'utf8'));
    return pkg.name === '@hust-agenthub/daemon';
  } catch {
    return false;
  }
}

function agentHubWorkspaceForPath(targetPath) {
  let current = path.dirname(path.resolve(targetPath));
  while (current && current !== path.dirname(current)) {
    if (isAgentHubWorkspace(current)) return current;
    current = path.dirname(current);
  }
  return null;
}

function openClawInstallSkillRoots(home) {
  const installsPath = path.join(home, '.openclaw', 'plugins', 'installs.json');
  let installs = null;
  try {
    installs = JSON.parse(fs.readFileSync(installsPath, 'utf8'));
  } catch {
    return [];
  }

  const roots = [];
  const records = installs && typeof installs.installRecords === 'object'
    ? Object.values(installs.installRecords)
    : [];
  for (const record of records) {
    if (!record || typeof record !== 'object') continue;
    if (record.installPath) addRoot(roots, path.join(String(record.installPath), 'skills'));
    if (record.sourcePath) addRoot(roots, path.join(String(record.sourcePath), 'skills'));
  }
  return roots;
}

function skillRoots(cliTool) {
  // 委托给 CliToolSpec.skillRoots（claude/codex/opencode/openclaw 各自实现）。
  // 未知 CLI 返回空数组，与原 if-else 链未匹配分支行为一致。
  const spec = cliTools.getCliTool(cliTool);
  if (!spec || typeof spec.skillRoots !== 'function') return [];
  const cwd = process.cwd();
  const home = os.homedir();
  return spec.skillRoots(cwd, home);
}

function parseSkillFile(fallbackName, sourcePath, content) {
  const skill = {
    name: fallbackName,
    detail: content.length > 500 ? content.slice(0, 500) + "..." : content,
    source_path: sourcePath,
    auto: true,
  };
  const lines = content.split(/\r?\n/);
  if (lines[0]?.trim() !== '---') {
    skill.description = normalizeSkillDescription(skill.name, skill.description, content);
    return skill;
  }
  for (let i = 1; i < lines.length; i += 1) {
    const line = lines[i].trim();
    if (line === '---') break;
    const separator = line.indexOf(':');
    if (separator === -1) continue;
    const key = line.slice(0, separator).trim();
    const parsed = readFrontmatterValue(lines, i, line.slice(separator + 1).trim());
    const value = parsed.value;
    i = parsed.nextIndex - 1;
    if (key === 'name' && value) skill.name = value;
    if (key === 'description') skill.description = value;
  }
  skill.description = normalizeSkillDescription(skill.name, skill.description, content);
  return skill;
}

function normalizeSkillDescription(name, description, content) {
  const current = String(description || '').trim();
  if (isUsefulDescription(current)) return current;
  return inferSkillDescription(name, content);
}

function isUsefulDescription(description) {
  const text = String(description || '').trim();
  if (!text) return false;
  if (/^(ok|todo|tbd|none|n\/a|na|null|undefined|test|demo|sample|example)$/i.test(text)) {
    return false;
  }
  const compact = text.replace(/[\s\p{P}\p{S}]/gu, '');
  return descriptionLength(compact) >= MIN_DESCRIPTION_CHARS;
}

function inferSkillDescription(name, content) {
  const body = stripFrontmatter(content);
  const chunks = [];
  let inFence = false;
  for (const rawLine of body.split(/\r?\n/)) {
    let line = rawLine.trim();
    if (line.startsWith('```') || line.startsWith('~~~')) {
      inFence = !inFence;
      continue;
    }
    if (inFence || !line) continue;
    line = cleanMarkdownLine(line);
    if (!line || line.toLowerCase() === String(name || '').toLowerCase()) continue;
    chunks.push(line);
    if (descriptionLength(chunks.join(' ')) >= 120) break;
  }
  const summary = truncateDescription(chunks.join(' ').replace(/\s+/g, ' ').trim());
  if (summary) return summary;
  return `Provides the ${name || 'selected'} skill for local Agent workflows.`;
}

function stripFrontmatter(content) {
  const lines = String(content || '').split(/\r?\n/);
  if (lines[0]?.trim() !== '---') return String(content || '');
  for (let i = 1; i < lines.length; i += 1) {
    if (lines[i].trim() === '---') {
      return lines.slice(i + 1).join('\n');
    }
  }
  return String(content || '');
}

function cleanMarkdownLine(line) {
  return line
    .replace(/^#{1,6}\s+/, '')
    .replace(/^[-*+]\s+/, '')
    .replace(/^\d+[.)]\s+/, '')
    .replace(/^>\s?/, '')
    .replace(/!\[[^\]]*]\([^)]*\)/g, '')
    .replace(/\[([^\]]+)]\([^)]*\)/g, '$1')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/\*([^*]+)\*/g, '$1')
    .replace(/__([^_]+)__/g, '$1')
    .replace(/_([^_]+)_/g, '$1')
    .trim();
}

function truncateDescription(text) {
  if (!text) return '';
  const chars = Array.from(text);
  if (chars.length <= 180) return text;
  return `${chars.slice(0, 177).join('').trimEnd()}...`;
}

function descriptionLength(text) {
  return Array.from(String(text || '')).length;
}

function readFrontmatterValue(lines, index, rawValue) {
  let value = rawValue.replace(/^['"]|['"]$/g, '');
  let nextIndex = index + 1;
  if (rawValue !== '>' && rawValue !== '|') {
    return { value, nextIndex };
  }
  const folded = rawValue === '>';
  const parts = [];
  for (let i = index + 1; i < lines.length; i += 1) {
    const current = lines[i];
    const trimmed = current.trim();
    if (trimmed === '---' || isFrontmatterKeyLine(current)) {
      nextIndex = i;
      break;
    }
    parts.push(trimmed);
    nextIndex = i + 1;
  }
  value = folded ? parts.join(' ') : parts.join('\n');
  return { value: value.trim(), nextIndex };
}

function isFrontmatterKeyLine(line) {
  if (/^\s/.test(line)) return false;
  return /^[A-Za-z0-9_-]+\s*:/.test(line.trim());
}

function apiURL(serverURL, apiKey, pathname) {
  const url = new URL(serverURL);
  url.pathname = `${url.pathname.replace(/\/$/, '')}${pathname}`;
  url.searchParams.set('token', apiKey);
  url.hash = '';
  return url;
}

function requestJSON(method, url, body, bearerToken) {
  const data = body === undefined ? null : Buffer.from(JSON.stringify(body));
  const transport = url.protocol === 'https:' ? https : http;

  const headers = {};
  if (data) {
    headers['Content-Type'] = 'application/json';
    headers['Content-Length'] = data.length;
  }
  if (bearerToken) {
    headers['Authorization'] = `Bearer ${bearerToken}`;
  }

  return new Promise((resolve, reject) => {
    const req = transport.request(url, {
      method,
      headers: Object.keys(headers).length ? headers : undefined,
    }, (res) => {
      let response = '';
      res.setEncoding('utf8');
      res.on('data', (chunk) => {
        response += chunk;
      });
      res.on('end', () => {
        if (!res.statusCode || res.statusCode < 200 || res.statusCode >= 300) {
          reject(new Error(`HTTP ${res.statusCode}: ${response}`));
          return;
        }
        try {
          resolve(response ? JSON.parse(response) : null);
        } catch (error) {
          reject(error);
        }
      });
    });
    req.on('error', reject);
    if (data) req.write(data);
    req.end();
  });
}

function truncateStr(s, max) {
  if (!s || s.length <= max) return s || '';
  return s.slice(0, max) + '...';
}

function buildPromptParts(task) {
  let ctx = task.context_messages;
  ctx = typeof ctx === 'string' ? ctx : '';

  // 长度保护：超过 8000 字符时截断中间部分，保留头部和尾部
  const maxCtxLen = 8000;
  if (ctx.length > maxCtxLen) {
    const headLen = Math.floor(maxCtxLen * 0.4);
    const tailLen = Math.floor(maxCtxLen * 0.4);
    ctx = ctx.slice(0, headLen) + '\n...[上下文已截断]...\n' + ctx.slice(ctx.length - tailLen);
  }

  // 从 context_messages 中提取系统指令和工具配置作为 system prompt
  let systemPrompt = '';
  let remainingCtx = ctx;
  const contextSectionBoundary = '(?=\\n\\n\\[可用工具\\]|\\n\\n\\[平台 Skills\\]|\\n\\n\\[群聊背景\\]|\\n\\n\\[调度指令\\]|\\n\\n\\[依赖输出\\]|\\n\\n\\{|$)';

  const sysSection = remainingCtx.match(new RegExp(`^(\\[系统指令\\]\\n[\\s\\S]*?)${contextSectionBoundary}`));
  if (sysSection) {
    systemPrompt += sysSection[1].replace('[系统指令]\n', '').trim();
    remainingCtx = remainingCtx.slice(sysSection[0].length).replace(/^\s+/, '');
  }

  const toolsSection = remainingCtx.match(new RegExp(`^(\\[可用工具\\]\\n[\\s\\S]*?)${contextSectionBoundary}`));
  if (toolsSection) {
    systemPrompt += (systemPrompt ? '\n\n' : '') + '# 可用工具\n' + toolsSection[1].replace('[可用工具]\n', '').trim();
    remainingCtx = remainingCtx.slice(toolsSection[0].length).replace(/^\s+/, '');
  }

  const skillsSection = remainingCtx.match(new RegExp(`^(\\[平台 Skills\\]\\n[\\s\\S]*?)${contextSectionBoundary}`));
  if (skillsSection) {
    systemPrompt += (systemPrompt ? '\n\n' : '') + '# 平台 Skills\n' + skillsSection[1].replace('[平台 Skills]\n', '').trim();
    remainingCtx = remainingCtx.slice(skillsSection[0].length).replace(/^\s+/, '');
  }

  remainingCtx = remainingCtx.trim();

  // 构建用户 prompt
  const parts = [];

  if (remainingCtx) {
    let handoffs = null;
    try {
      handoffs = JSON.parse(remainingCtx);
    } catch {
      // not JSON — treat as plain text (orchestrator dispatch context)
    }

    if (handoffs !== null && Array.isArray(handoffs)) {
      parts.push('[历史 Agent 交接]');
      for (const h of handoffs) {
        const req = truncateStr(h.user_request, 100);
        const res = truncateStr(h.result, 200);
        parts.push(`- ${h.agent_name}: 用户问 "${req}" → 回复：${res}`);
      }
      parts.push('');
      parts.push('你是 AgentHub 群聊中被 @提及的机器人，请参考上述交接上下文回答用户消息。');
    } else {
      parts.push(remainingCtx);
    }
  } else if (!systemPrompt) {
    parts.push('你是 AgentHub 群聊中被 @提及的机器人，请直接回答用户当前消息。');
  }

  parts.push('');
  parts.push(task.prompt);

  return { systemPrompt, userPrompt: parts.join('\n') };
}

function ensureTaskWorkdir(task) {
  const safeID = String(task.id || crypto.randomUUID()).replace(/[^a-zA-Z0-9_-]/g, '-');
  const dir = path.join(os.tmpdir(), 'agenthub-cli-tasks', safeID);
  fs.mkdirSync(dir, { recursive: true });
  return dir;
}

function ensureAgentHubCodexHome() {
  // Codex 登录态默认保存在 ~/.codex。这里默认复用用户已登录的 CODEX_HOME，
  // 避免 scanAgents 用默认 home 判定已登录、实际执行却切到空 ~/.agenthub/codex。
  const configured = process.env.AGENTHUB_CODEX_HOME || process.env.CODEX_HOME;
  const dir = configured || path.join(os.homedir(), '.codex');
  fs.mkdirSync(dir, { recursive: true });
  return dir;
}

function tomlString(value) {
  return JSON.stringify(String(value));
}

function tomlArray(values) {
  return `[${values.map(tomlString).join(', ')}]`;
}

function ensureAgentHubCodexMcpConfig(codexHome, conversationId, userId, agentId, taskId) {
  const configFile = path.join(codexHome, 'config.toml');
  let content = fs.existsSync(configFile) ? fs.readFileSync(configFile, 'utf8') : '';
  const sectionPattern = /\n?\[mcp_servers\.agenthub-platform\]\n[\s\S]*?(?=\n\[[^\]]+\]|\s*$)/;
  content = content.replace(sectionPattern, '').trimEnd();
  const args = buildPlatformMcpServerArgs(conversationId, userId, agentId, taskId);
  if (args.length === 0) return configFile;
  const section = [
    '',
    '[mcp_servers.agenthub-platform]',
    'command = "node"',
    `args = ${tomlArray(args)}`,
    'enabled = true',
    'default_tools_approval_mode = "approve"',
    '',
  ].join('\n');
  fs.writeFileSync(configFile, `${content}${section}`, 'utf8');
  return configFile;
}

// initCliToolsCtx 是传给 CliToolSpec 工厂的依赖注入对象。
// 包含 spec 实现需要的全部 daemon 辅助函数（避免 spec 直接 require 主文件造成循环依赖）。
// 新增 spec 时若需要新辅助函数，在此对象补一个键即可。
//
// === Step 2 扩展（agent-adapter 重构） ===
// 下面带 // step2 注释的字段是为 Step 2-3 新增的 spec 方法（resolveCommand / parseResult /
// spawnPersistent）准备的依赖。Step 2-3 阶段 spec 方法是 dormant 的（没有 call site 调用），
// 此处只是预先 wire 好；Step 4 才会切换 call site 去调用 spec 方法。
const initCliToolsCtx = {
  // 通用工具
  pathJoin: path.join,
  tmpdir: os.tmpdir,
  addRoot,
  errorMessage,
  firstLine,
  logFlow,
  // 命令解析 / 版本检测
  resolveCommand,
  commandVersion,
  processSpec,
  spawnSync,
  // step2: spawn（persistent 进程启动，给 spec.spawnPersistent 用）
  spawn,
  // step2: fs / crypto（outputFile 读取 / sessionId 生成）
  fs,
  crypto,
  // step2: truncateStr（spec.spawnPersistent 的 stderr 日志截断）
  truncateStr,
  // step2: EXEC_TIMEOUT_MS（spec.spawnPersistent 的 turn timeout）
  EXEC_TIMEOUT_MS,
  // step2: existingFile（spec.resolveCommand 的路径校验）
  existingFile,
  // step2: codex 路径辅助（spec.codex.resolveCommand 用）
  codexLocalInstallPaths,
  codexExtensionPath,
  // step2: saveSessionMap（spec.opencode.parseResult 持久化 session 用）
  saveSessionMap,
  // step2: agentTurnStates（spec.claude.spawnPersistent 同步 turn 状态）
  agentTurnStates,
  // step2: createAsyncQueue（spec.claude.spawnPersistent 构造事件队列）
  createAsyncQueue: require('../cli/events').createAsyncQueue,
  // prompt / context 辅助
  buildPlatformMcpArgs,
  buildAgentHubContextEnv,
  buildPlatformMcpServerArgs,
  makeSessionId,
  sessionKeyForTask,
  opencodeContextChanged,
  // codex 专用
  ensureAgentHubCodexHome,
  ensureAgentHubCodexMcpConfig,
  ensureTaskWorkdir,
  codexLoginStatus,
  // opencode 专用
  ensureOpenCodeMcpConfig,
  // skill 扫描辅助
  isAgentHubWorkspace,
  openClawInstallSkillRoots,
  // 默认能力构造（CANDIDATES 派生用）
  defaultSkills,
  // conversationSessions（运行时可变的会话映射）
  conversationSessions,
};

// 启动时注册 4 个 CLI spec。函数声明会被提升，此处所有辅助函数已可用。
cliTools.initCliTools(initCliToolsCtx);

function commandForTask(task, taskCtx) {
  const { systemPrompt, userPrompt } = buildPromptParts(task);
  const command = resolveCommand(task.cli_tool);
  // 委托给 CliToolSpec.buildCommand。未知 cli_tool 走 fallback（保留原 default 分支行为）。
  const spec = cliTools.getCliTool(task.cli_tool);
  if (spec && typeof spec.buildCommand === 'function') {
    return spec.buildCommand(task, {
      command,
      systemPrompt,
      userPrompt,
      runningAgents,
      taskId: (taskCtx && taskCtx.taskId) || null,
    });
  }
  return { command, args: [userPrompt] };
}

async function executeTask(task, taskCtx) {
  if (task.cli_tool === OPEN_PATH_TOOL) {
    logFlow('info', 'task.open_path_start', { task_id: task.id });
    return openSkillLocation(task.prompt);
  }
  if (task.cli_tool === BROWSE_FILES_TOOL) {
    // 文件浏览同步 RPC：前端抽屉浏览该机器 git 工作区文件。
    logFlow('info', 'task.browse_files_start', { task_id: task.id });
    return browseFiles(task.prompt);
  }
  const spec = commandForTask(task, taskCtx);
  const taskMeta = {
    task_id: task.id,
    cli_tool: task.cli_tool || 'unknown',
    agent_id: task.agent_id,
    conversation_id: task.conversation_id,
  };
  // 在 spawn 之前确保 workdir 是 git 仓库——agent 跑 git diff 生成 diff 卡片时
  // 非 git 仓库会 fatal（exit 128），导致前面的文件修改全部丢失。daemon 自动 init +
  // baseline commit 作为兜底，失败不阻塞 task。
  ensureGitRepoForTask(spec.cwd || process.cwd(), taskMeta);
  logFlow('info', 'task.command_prepared', {
    ...taskMeta,
    command: spec.command,
    args_count: Array.isArray(spec.args) ? spec.args.length : 0,
    has_stdin: typeof spec.stdin === 'string' && spec.stdin.length > 0,
    stdin_len: typeof spec.stdin === 'string' ? spec.stdin.length : 0,
    session_id: spec.sessionId,
    output_file: Boolean(spec.outputFile),
    result_format: spec.resultFormat,
  });

  let stdout;
  let stderr;
  if (spec.sessionId) {
    logFlow('info', 'task.session_resume_start', { ...taskMeta, session_id: spec.sessionId });
    killSessionProcess(spec.sessionId);
    await sleep(1000);

    try {
      ({ stdout, stderr } = await runProcess(
        spec.command,
        ['--resume', spec.sessionId, ...spec.args],
        spec.stdin,
        spec.sessionId,
        spec.cwd,
        spec.env,
        { ...taskMeta, mode: 'resume' },
      ));
    } catch (_err) {
      logFlow('warn', 'task.session_resume_failed', {
        ...taskMeta,
        session_id: spec.sessionId,
        error: errorMessage(_err),
      });
      killSessionProcess(spec.sessionId);
      await sleep(500);
      try {
        ({ stdout, stderr } = await runProcess(
          spec.command,
          ['--session-id', spec.sessionId, ...spec.args],
          spec.stdin,
          spec.sessionId,
          spec.cwd,
          spec.env,
          { ...taskMeta, mode: 'session_id_retry' },
        ));
      } catch (_err2) {
        const freshId = `agenthub-${String(task.id || crypto.randomUUID()).replace(/[^a-zA-Z0-9_-]/g, '-')}`;
        logFlow('warn', 'task.session_fresh_start', {
          ...taskMeta,
          previous_session_id: spec.sessionId,
          fresh_session_id: freshId,
          error: errorMessage(_err2),
        });
        ({ stdout, stderr } = await runProcess(
          spec.command,
          ['--session-id', freshId, ...spec.args],
          spec.stdin,
          freshId,
          spec.cwd,
          spec.env,
          { ...taskMeta, mode: 'fresh_session' },
        ));
      }
    }
  } else {
    ({ stdout, stderr } = await runProcess(spec.command, spec.args, spec.stdin, undefined, spec.cwd, spec.env, { ...taskMeta, mode: 'one_shot' }));
  }

  if (spec.outputFile && fs.existsSync(spec.outputFile)) {
    const text = fs.readFileSync(spec.outputFile, 'utf8').trim();
    fs.rmSync(spec.outputFile, { force: true });
    if (text) {
      logFlow('info', 'task.result_ready', { ...taskMeta, source: 'output_file', result_len: text.length });
      return text;
    }
  }
  // Switch 2: 委托给 CliToolSpec.parseResult —— 替代原 openclaw-json / opencode-json / stdio
  // 三分支。spec.parseResult 可能返回 string（claude/openclaw/codex）或 { text, sessionId }
  // 对象（opencode，保留原 parseOpenCodeOutput 结构化返回）；opencode 的 session 持久化副作用
  // 已在 spec 内部完成，daemon 不再手动 set/saveSessionMap。
  const cliSpec = cliTools.getCliTool(task.cli_tool);
  if (cliSpec && typeof cliSpec.parseResult === 'function') {
    const parsed = cliSpec.parseResult(
      { stdout, stderr, outputFile: spec.outputFile, meta: { persistSessionKey: spec.persistSessionKey, task } },
      initCliToolsCtx,
    );
    if (parsed && typeof parsed === 'object' && typeof parsed.text === 'string') {
      logFlow('info', 'task.result_ready', {
        ...taskMeta,
        source: 'spec_parse_result',
        result_len: parsed.text.length,
        session_id: parsed.sessionId,
        session_persisted: Boolean(spec.persistSessionKey && parsed.sessionId),
      });
      return parsed.text || '(Agent CLI 没有返回内容)';
    }
    const text = typeof parsed === 'string' ? parsed : String(parsed || '');
    logFlow('info', 'task.result_ready', { ...taskMeta, source: 'spec_parse_result', result_len: text.length });
    return text || '(Agent CLI 没有返回内容)';
  }
  const text = `${stdout || ''}${stderr ? `\n${stderr}` : ''}`.trim();
  logFlow('info', 'task.result_ready', { ...taskMeta, source: 'stdio', result_len: text.length });
  return text || '(Agent CLI 没有返回内容)';
}

async function executeTaskOnce(task, taskCtx) {
  const taskID = task && task.id;
  if (!taskID) return executeTask(task, taskCtx);
  if (activeTaskIDs.has(taskID) || completedTaskIDs.has(taskID)) {
    logFlow('warn', 'task.dispatch_duplicate_ignored', {
      task_id: taskID,
      cli_tool: task.cli_tool,
      agent_id: task.agent_id,
      conversation_id: task.conversation_id,
    });
    return null;
  }
  activeTaskIDs.add(taskID);
  try {
    const result = await executeTask(task, taskCtx);
    completedTaskIDs.add(taskID);
    if (completedTaskIDs.size > 1000) {
      completedTaskIDs.delete(completedTaskIDs.values().next().value);
    }
    return result;
  } finally {
    activeTaskIDs.delete(taskID);
  }
}

function openSkillLocation(prompt) {
  let payload = null;
  try {
    payload = JSON.parse(prompt);
  } catch {
    throw new Error('Invalid open path payload');
  }
  const sourcePath = String(payload.source_path || '').trim();
  if (!sourcePath || path.basename(sourcePath) !== 'SKILL.md') {
    throw new Error('Invalid skill file path');
  }
  if (agentHubWorkspaceForPath(sourcePath)) {
    throw new Error('Refuse to open stale AgentHub workspace skill source. Reconnect this computer to refresh skills.');
  }
  if (!fs.existsSync(sourcePath) || !fs.statSync(sourcePath).isFile()) {
    throw new Error(`Skill file not found: ${sourcePath}`);
  }
  const folder = path.dirname(sourcePath);
  let command = 'xdg-open';
  let args = [folder];
  let hideWindow = false;
  if (process.platform === 'win32') {
    command = 'explorer.exe';
    args = [`/select,${sourcePath}`];
  } else if (process.platform === 'darwin') {
    command = 'open';
    args = ['-R', sourcePath];
  }
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      detached: true,
      stdio: 'ignore',
      windowsHide: hideWindow,
    });
    let settled = false;
    const finish = (callback) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      callback();
    };
    const timer = setTimeout(() => {
      child.unref();
      finish(() => resolve(`Opened folder ${folder}`));
    }, OPEN_PATH_TIMEOUT_MS);
    child.on('error', (error) => {
      finish(() => reject(new Error(`Open folder failed: ${error.message}`)));
    });
    child.on('close', (code) => {
      if (process.platform === 'win32' || code === 0) {
        finish(() => resolve(`Opened folder ${folder}`));
        return;
      }
      finish(() => reject(new Error(`Open folder exited with code ${code}`)));
    });
  });
}

// parseOpenClawOutput / parseOpenCodeOutput 及其辅助函数（extractOpenCodeSessionId /
// textFromOpenCodeContent / collectOpenCodeText）已删除（Switch 7）——逻辑全部搬到
// cli/openclaw.js 和 cli/opencode.js 的 spec.parseResult，自包含且测试覆盖。

function lineEndIndex(text, start) {
  const nl = text.indexOf('\n', start);
  return nl === -1 ? text.length : nl + 1;
}

function lineText(text, start, end) {
  const raw = text.slice(start, end);
  return raw.endsWith('\n') ? raw.slice(0, -1).replace(/\r$/, '') : raw.replace(/\r$/, '');
}

function findFenceClose(text, start, preferLast) {
  let pos = start;
  let first = null;
  let last = null;
  while (pos < text.length) {
    const end = lineEndIndex(text, pos);
    const line = lineText(text, pos, end);
    if (/^ {0,3}`{3,}\s*$/.test(line)) {
      const found = { start: pos, end };
      if (!first) first = found;
      last = found;
      if (!preferLast) return found;
    }
    pos = end;
  }
  return preferLast ? last : first;
}

function extractFencedBlocks(text) {
  const blocks = [];
  let pos = 0;
  while (pos < text.length) {
    const end = lineEndIndex(text, pos);
    const line = lineText(text, pos, end);
    const open = line.match(/^ {0,3}`{3,}([^`]*)$/);
    if (!open) {
      pos = end;
      continue;
    }

    const language = (open[1] || '').trim().toLowerCase();
    const preferLastClose = language === 'markdown' || language === 'md';
    const close = findFenceClose(text, end, preferLastClose);
    if (!close) break;

    blocks.push({
      language,
      content: text.slice(end, close.start),
      start: pos,
      end: close.end,
    });
    pos = close.end;
  }
  return blocks;
}

function unwrapSingleMarkdownFence(text, fencedBlocks) {
  if (fencedBlocks.length !== 1) return null;
  const block = fencedBlocks[0];
  if (block.language !== 'markdown' && block.language !== 'md') return null;
  if (text.slice(0, block.start).trim() || text.slice(block.end).trim()) return null;
  return block.content.replace(/\s+$/, '');
}

function extractMarkdownDocumentFence(fencedBlocks) {
  for (const block of fencedBlocks) {
    if (block.language !== 'markdown' && block.language !== 'md') continue;
    const content = block.content.replace(/\s+$/, '');
    if (looksLikeMarkdownDocument(content)) return content;
  }
  return null;
}

function looksLikeMarkdownDocument(text) {
  const src = text.trim();
  if (src.length < 40) return false;
  const headingMatches = src.match(/^#{1,3}\s+\S.+$/gm) || [];
  if (headingMatches.length === 0) return false;
  if (headingMatches.length >= 2) return true;
  return /(^|\n)(?:[-*]\s+\S|\|.+\||```)/.test(src);
}

function markdownDocumentTitle(text) {
  const match = text.match(/^#\s+(.+)$/m);
  return match ? match[1].trim() : 'Document Preview';
}

function parseArtifacts(text) {
  if (typeof text !== 'string' || !text.trim()) return [];
  const artifacts = [];
  const documentLanguages = new Set(['markdown', 'md', 'txt', 'text', 'json', 'csv']);
  const fencedBlocks = extractFencedBlocks(text);
  const wrappedMarkdown = unwrapSingleMarkdownFence(text, fencedBlocks);
  if (wrappedMarkdown && looksLikeMarkdownDocument(wrappedMarkdown)) {
    return [{
      type: 'document',
      language: 'markdown',
      filename: '',
      title: markdownDocumentTitle(wrappedMarkdown),
      content: wrappedMarkdown,
    }];
  }
  const embeddedMarkdown = extractMarkdownDocumentFence(fencedBlocks);
  if (embeddedMarkdown) {
    return [{
      type: 'document',
      language: 'markdown',
      filename: '',
      title: markdownDocumentTitle(embeddedMarkdown),
      content: embeddedMarkdown,
    }];
  }
  // 注意：不把整段回复当 markdown 文档猜测（原 looksLikeMarkdownDocument(text) 逻辑已移除）。
  // 产物协议约定 agent 必须用 ```markdown 显式标记才会识别为文档，
  // 避免普通带标题+列表的回复被误判成 Document Preview 卡片。

  // 1) 提取围栏代码块 ```lang\n...\n```
  // 第一行可能是 "// file: xxx" 或 "# file: xxx" 形式的文件名提示
  for (const block of fencedBlocks) {
    const language = block.language;
    let content = block.content;
    let filename = '';
    // 识别首行文件名注释：// file: xxx 、# file: xxx 、<!-- file: xxx -->
    const firstLine = content.split('\n', 1)[0] || '';
    const fileHint = firstLine.match(/^\s*(?:\/\/|#|<!--)\s*file:\s*(.+?)\s*(?:-->)?\s*$/i);
    if (fileHint) {
      filename = fileHint[1].trim();
      const nl = content.indexOf('\n');
      // 只在确有换行时剥离首行提示，否则整块就是提示行本身（无正文）
      content = nl === -1 ? '' : content.slice(nl + 1);
    }
    content = content.replace(/\s+$/, '');
    if (!content) continue;

    // language 为 html 时归类为 webpage（iframe 预览），否则为 code。
    // 只要标记为 html 即归类为 webpage——平台协议约定 agent 用 ```html 标记
    // 可预览的网页内容，不强制要求完整文档结构（片段也可预览）。
    if (language === 'html') {
      artifacts.push({ type: 'webpage', title: filename || 'HTML Preview', content });
    } else if (documentLanguages.has(language)) {
      artifacts.push({
        type: 'document',
        language: language || 'text',
        filename,
        title: filename || (language === 'json' ? 'JSON Document' : 'Document Preview'),
        content,
      });
    } else {
      artifacts.push({ type: 'code', language, filename, content });
    }
  }

  // 2) 提取裸 http/https URL（在代码块外，简单去重）
  // 先剔除围栏代码块，避免把代码里的 import/注释 URL 误当作网页产物
  let textOutsideFences = '';
  let lastIndex = 0;
  for (const block of fencedBlocks) {
    textOutsideFences += text.slice(lastIndex, block.start) + ' ';
    lastIndex = block.end;
  }
  textOutsideFences += text.slice(lastIndex);
  // 字符类排除空白、闭合括号/尖括号/引号，以及中文标点（。，、；）避免吞掉句末标点
  const urlRe = /\bhttps?:\/\/[^\s)\]<>"'，。、；：！？]+/g;
  const seen = new Set();
  let urlMatch;
  while ((urlMatch = urlRe.exec(textOutsideFences)) !== null) {
    const url = urlMatch[0].replace(/[.,;:!?]+$/, ''); // 去掉句末 ASCII 标点
    if (!url || seen.has(url)) continue;
    seen.add(url);
    artifacts.push({ type: 'webpage', url, title: '' });
  }

  return artifacts;
}

function runProcess(command, args, stdin, sessionId, cwd, extraEnv, meta = {}) {
  return new Promise((resolve, reject) => {
    const spec = processSpec(command, args);
    const detached = process.platform !== 'win32';
    const startedAt = Date.now();
    const child = spawn(spec.command, spec.args, {
      cwd: cwd || undefined,
      env: extraEnv ? { ...process.env, ...extraEnv } : undefined,
      windowsHide: true,
      stdio: ['pipe', 'pipe', 'pipe'],
      detached,
    });
    logFlow('info', 'process.spawn', {
      ...meta,
      command: spec.command,
      args_count: spec.args.length,
      stdin_len: typeof stdin === 'string' ? stdin.length : 0,
      session_id: sessionId,
      pid: child.pid,
    });
    if (sessionId) {
      activeSessions.set(sessionId, child);
      child.on('close', () => {
        if (activeSessions.get(sessionId) === child) {
          activeSessions.delete(sessionId);
        }
      });
    }
    let stdout = '';
    let stderr = '';
    const timer = setTimeout(() => {
      try {
        if (detached) {
          process.kill(-child.pid, 'SIGKILL');
        } else {
          child.kill();
        }
      } catch { /* ignore */ }
      logFlow('error', 'process.timeout', {
        ...meta,
        command: spec.command,
        pid: child.pid,
        session_id: sessionId,
        timeout_ms: EXEC_TIMEOUT_MS,
      });
      reject(new Error(`CLI execution timed out after ${EXEC_TIMEOUT_MS / 1000}s`));
    }, EXEC_TIMEOUT_MS);

    child.stdout.setEncoding('utf8');
    child.stderr.setEncoding('utf8');
    child.stdout.on('data', (chunk) => {
      stdout += chunk;
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk;
    });
    child.on('error', (error) => {
      clearTimeout(timer);
      logFlow('error', 'process.error', {
        ...meta,
        command: spec.command,
        pid: child.pid,
        session_id: sessionId,
        error: errorMessage(error),
      });
      reject(error);
    });
    child.on('close', (code) => {
      clearTimeout(timer);
      logFlow(code === 0 ? 'info' : 'warn', 'process.exit', {
        ...meta,
        command: spec.command,
        pid: child.pid,
        session_id: sessionId,
        exit_code: code,
        duration_ms: Date.now() - startedAt,
        stdout_len: stdout.length,
        stderr_len: stderr.length,
      });
      if (code === 0) {
        resolve({ stdout, stderr });
        return;
      }
      reject(new Error((stderr || stdout || `CLI exited with code ${code}`).trim()));
    });
    child.stdin.end(stdin || '');
  });
}

async function register(serverURL, apiKey) {
  const agents = scanAgents();
  logFlow('info', 'register.scan_completed', {
    machine_id: os.hostname(),
    agent_count: agents.length,
    agents: agents.map((agent) => `${agent.name}:${agent.cli_tool}`),
  });
  for (const agent of agents) {
    const version = agent.version ? ` ${String(agent.version).split('\n')[0].trim()}` : '';
    const skillCount = Array.isArray(agent.capabilities) ? agent.capabilities.length : 0;
    logFlow('info', 'register.agent_detected', {
      machine_id: os.hostname(),
      agent_name: agent.name,
      cli_tool: agent.cli_tool,
      version: version.trim(),
      skill_count: skillCount,
    });
  }
  const res = await requestJSON('POST', apiURL(serverURL, apiKey, '/daemon/register'), {
    machine_id: os.hostname(),
    agents,
    capabilities: detectCapabilities(),
  });
  if (res && res.data && res.data.daemon_token && !daemonConn.daemonToken) {
    daemonConn.daemonToken = res.data.daemon_token;
  }
  logFlow('info', 'register.completed', {
    machine_id: os.hostname(),
    agent_count: agents.length,
    daemon_token_received: Boolean(res && res.data && res.data.daemon_token),
  });
  logFlow('info', 'daemon.ready', { machine_id: os.hostname(), transport: WebSocket ? 'websocket' : 'polling' });
}

// 在任务执行期间定期发送心跳，告知 server 任务仍在进行中
function startHeartbeat(serverURL, apiKey, taskId) {
  const timer = setInterval(async () => {
    try {
      await requestJSON('POST', apiURL(serverURL, apiKey, `/daemon/tasks/${taskId}/heartbeat`), {});
    } catch (error) {
      logFlow('warn', 'task.heartbeat_failed', { task_id: taskId, error: errorMessage(error) });
    }
  }, HEARTBEAT_INTERVAL_MS);
  return () => clearInterval(timer);
}

async function pollTasks(serverURL, apiKey) {
  for (;;) {
    try {
      const response = await requestJSON('GET', apiURL(serverURL, apiKey, '/daemon/tasks'));
      const task = response ? response.data : null;
      if (!task) {
        await sleep(POLL_INTERVAL_MS);
        continue;
      }

      logFlow('info', 'poll.task_claimed', {
        task_id: task.id,
        cli_tool: task.cli_tool,
        agent_id: task.agent_id,
        conversation_id: task.conversation_id,
      });
      const stopHeartbeat = startHeartbeat(serverURL, apiKey, task.id);
      try {
        const result = await executeTaskOnce(task);
        if (result === null) {
          stopHeartbeat();
          continue;
        }
        stopHeartbeat();
        await requestJSON('POST', apiURL(serverURL, apiKey, `/daemon/tasks/${task.id}/complete`), { result });
        logFlow('info', 'poll.task_completed', {
          task_id: task.id,
          cli_tool: task.cli_tool,
          result_len: typeof result === 'string' ? result.length : 0,
        });
      } catch (error) {
        stopHeartbeat();
        await requestJSON('POST', apiURL(serverURL, apiKey, `/daemon/tasks/${task.id}/complete`), {
          error: error instanceof Error ? error.message : String(error),
        });
        logFlow('error', 'poll.task_failed', {
          task_id: task.id,
          cli_tool: task.cli_tool,
          error: errorMessage(error),
        });
      }
    } catch (error) {
      logFlow('warn', 'poll.failed', { error: errorMessage(error), retry_ms: POLL_INTERVAL_MS * 2 });
      await sleep(POLL_INTERVAL_MS * 2);
    }
  }
}
function stopAgentProcess(agent_id) {
  const entry = runningAgents.get(agent_id);
  if (!entry) return;
  try {
    if (process.platform === 'win32') {
      spawn('taskkill', ['/pid', String(entry.process.pid), '/T', '/F'], { windowsHide: true });
    } else {
      process.kill(-entry.process.pid, 'SIGKILL');
    }
    logFlow('info', 'agent.process_killed', {
      agent_id,
      session_id: entry.sessionId,
      pid: entry.process.pid,
      conversation_id: entry.currentConversationId,
    });
  } catch { /* already dead */ }
  runningAgents.delete(agent_id);
}

/**
 * Spawn a Claude Code process with stream-json transport.
 * Returns { child, sessionId, sendPrompt, events }.
 * If resume=true, uses --resume <sessionId>; otherwise --session-id <sessionId>.
 *
 * Switch 4 落地：函数体从 Claude-specific 的 stream-json 实现改为 thin wrapper，
 * 委托给 ClaudeCliSpec.spawnPersistent。事件流通过 AsyncIterable<AgentEvent> 暴露给
 * 未来 dispatcher 翻译成 WS 消息（本次不切换 WS 消息路径）。函数名暂保留，因 callsite
 * 仍在用（改名是 Switch 5 的事）。
 */
function spawnStreamJsonProcess(agentId, sessionId, systemPrompt, resume, conversationId, userId, taskCtx, eventRef, cliTool = 'claude') {
  const spec = cliTools.getCliTool(cliTool);
  if (!spec || typeof spec.spawnPersistent !== 'function') {
    throw new Error(`CLI "${cliTool}" does not support spawnPersistent`);
  }
  return spec.spawnPersistent(
    { agentId, sessionId, systemPrompt, resume, conversationId, userId, taskCtx, eventRef },
    initCliToolsCtx,
  );
}

/**
 * Unified persistent dispatch: per-agent process slot with conversation isolation.
 * - Same conversation → stdin inject (fast path)
 * - Cross-conversation → reuse live process (stream-json 支持多 turn)
 * - Dead / no process → spawn fresh
 *
 * taskCtx 随调用链传入，承载本任务的执行上下文（含 daemon 内部卡片队列等）。
 *
 * Switch 4 已落地：spawnStreamJsonProcess 是 thin wrapper，委托给 spec.spawnPersistent
 * （目前仅 ClaudeCliSpec 实现，其它 spec 加上 spawnPersistent 即自动接入）。
 *
 * Switch 5 落地：函数从 `dispatchToClaudeSlot` 改名为 `dispatchToPersistentSlot`，语义
 * 去 Claude 化——任何实现了 spawnPersistent 的 spec 都能通过本函数 dispatch。当前
 * 仅 Claude 实现 spawnPersistent，但函数内部不再 hardcode 假设只有 Claude（slot 内
 * cliTool 字段从 task.cli_tool 提取，便于将来扩展）。
 *
 * 注：WS event name（agent.*）是前端约定的对外协议，保持原样不动；只改函数名和内部
 * 通用性。前端不会感知到这个函数改名。
 */
async function dispatchToPersistentSlot(ws, agentId, conversationId, userId, prompt, systemPrompt, taskCtx, cliTool = 'claude', onEvent = null) {
  const sessionKey = `${agentId}:${conversationId}`;
  const savedSessionId = conversationSessions.get(sessionKey) || null;
  const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  const validSessionId = savedSessionId && UUID_RE.test(savedSessionId) ? savedSessionId : null;
  const slot = runningAgents.get(agentId);

  // Fast path: same conversation or unbound agent (null = accept any), process running
  if (slot?.sendPrompt && (slot.currentConversationId === conversationId || slot.currentConversationId === null)) {
    if (slot.currentConversationId === null) {
      logFlow('info', 'agent.dispatch_fast_path', {
        agent_id: agentId,
        conversation_id: conversationId,
        previous_conversation_id: 'unbound',
        session_id: slot.sessionId,
      });
      slot.currentConversationId = conversationId;
    } else {
      logFlow('info', 'agent.dispatch_fast_path', {
        agent_id: agentId,
        conversation_id: conversationId,
        previous_conversation_id: slot.currentConversationId,
        session_id: slot.sessionId,
      });
    }
    // 注入本次 task 的 onEvent 到 slot.eventRef（persistent agent 跨 task 复用）
    if (slot.eventRef && typeof onEvent === 'function') slot.eventRef.current = onEvent;
    try {
      const response = await slot.sendPrompt(prompt);
      if (response.error) throw new Error(response.error);
      return response.result;
    } finally {
      if (slot.eventRef) slot.eventRef.current = null;
    }
  }

  // 如果已有持久进程在运行且属于同一会话，直接复用（fast path）。
  // 不同对话也复用同一进程——stream-json 模式支持多 turn，
  // 杀进程会导致其他对话的任务失败。
  if (slot?.sendPrompt) {
    // 检查进程是否还活着，如果已退出则清理并走 spawn 路径
    if (slot.process && slot.process.exitCode !== null) {
      logFlow('warn', 'agent.process_dead_reuse', {
        agent_id: agentId,
        conversation_id: conversationId,
        exit_code: slot.process.exitCode,
      });
      runningAgents.delete(agentId);
      agentTurnStates.delete(agentId);
      // 落入下面的 spawn 逻辑
    } else {
      logFlow('info', 'agent.reuse_slot', {
        agent_id: agentId,
        conversation_id: conversationId,
        current_conversation_id: slot.currentConversationId,
        same_conv: slot.currentConversationId === conversationId,
      });
      // 更新当前对话 ID（用于日志追踪）
      slot.currentConversationId = conversationId;
      if (slot.eventRef && typeof onEvent === 'function') slot.eventRef.current = onEvent;
      try {
        const response = await slot.sendPrompt(prompt);
        if (response.error) throw new Error(response.error);
        return response.result;
      } finally {
        if (slot.eventRef) slot.eventRef.current = null;
      }
    }
  }

  // 没有已有进程——spawn 新的
  // 为新 slot 创建 eventRef，让 stdout.on('data') 闭包捕获，后续 dispatch 动态切换 current。
  const eventRef = { current: typeof onEvent === 'function' ? onEvent : null };
  let result;
  if (validSessionId) {
    try {
      result = spawnStreamJsonProcess(agentId, validSessionId, systemPrompt, true, conversationId, userId, taskCtx, eventRef, cliTool);
      // Wait briefly to detect immediate resume failure
      await sleep(2000);
      if (result.child.exitCode !== null) {
        throw new Error('Resume failed');
      }
    } catch {
      // Resume failed — spawn fresh with new session ID
      logFlow('warn', 'agent.resume_failed', {
        agent_id: agentId,
        conversation_id: conversationId,
        session_id: validSessionId,
      });
      result = spawnStreamJsonProcess(agentId, null, systemPrompt, false, conversationId, userId, taskCtx, eventRef, cliTool);
    }
  } else {
    result = spawnStreamJsonProcess(agentId, null, systemPrompt, false, conversationId, userId, taskCtx, eventRef, cliTool);
  }

  const { child, sessionId, sendPrompt } = result;

  // Handle process exit
  child.on('close', (code) => {
    const entry = runningAgents.get(agentId);
    if (entry?.process === child) {
      runningAgents.delete(agentId);
    }
    agentTurnStates.delete(agentId);
    logFlow(code === 0 ? 'info' : 'warn', 'agent.process_exit', {
      agent_id: agentId,
      conversation_id: conversationId,
      session_id: sessionId,
      pid: child.pid,
      exit_code: code,
    });
    safeSend(currentDaemonWs, JSON.stringify({ type: 'agent.stopped', data: { agent_id: agentId, exit_code: code } }));
  });

  // Register in runningAgents with conversation tracking.
  runningAgents.set(agentId, {
    process: child,
    sessionId,
    currentConversationId: conversationId,
    cliTool,
    sendPrompt,
    eventRef,
  });
  idleAgentConfigs.set(agentId, { cliTool, sessionId, systemPrompt: systemPrompt || '' });
  agentTurnStates.set(agentId, 'idle');

  // Persist session mapping
  conversationSessions.set(sessionKey, sessionId);
  saveSessionMap();

  logFlow('info', 'agent.slot_spawned', {
    agent_id: agentId,
    conversation_id: conversationId,
    session_id: sessionId,
    pid: child.pid,
    resumed: Boolean(validSessionId),
  });

  try {
    const response = await sendPrompt(prompt);
    if (response.error) throw new Error(response.error);
    return response.result;
  } finally {
    eventRef.current = null;
  }
}

function enqueueAgentStart(ws, payload) {
  agentStartQueue.push({ ws, payload });
  logFlow('info', 'agent.start_queued', {
    agent_id: payload && payload.agent_id,
    cli_tool: payload && payload.cli_tool,
    queue_len: agentStartQueue.length,
  });
  processStartQueue();
}

function processStartQueue() {
  if (agentStartQueue.length === 0) return;
  const now = Date.now();
  const elapsed = now - lastAgentStartAt;
  if (elapsed < START_QUEUE_INTERVAL_MS) {
    setTimeout(processStartQueue, START_QUEUE_INTERVAL_MS - elapsed);
    return;
  }
  const item = agentStartQueue.shift();
  if (item) {
    lastAgentStartAt = Date.now();
    handleAgentStart(item.ws, item.payload);
    if (agentStartQueue.length > 0) {
      setTimeout(processStartQueue, START_QUEUE_INTERVAL_MS);
    }
  }
}

async function handleAgentStart(ws, payload) {
  const { agent_id, cli_tool, system_prompt } = payload;
  if (!agent_id || !cli_tool) return;

  logFlow('info', 'agent.start_requested', {
    agent_id,
    cli_tool,
    system_prompt_len: typeof system_prompt === 'string' ? system_prompt.length : 0,
  });
  stopAgentProcess(agent_id);

  // Switch 6: 泛化 persistent-mode 检查 —— 任何实现了 spawnPersistent 的 spec 都允许启动。
  // 当前只有 claude spec 实现，所以语义与原 `cli_tool !== 'claude'` 等价，但不再 hardcode 字符串。
  // 未来给其它 spec 加上 spawnPersistent 即自动支持 persistent 模式。
  const startSpec = cliTools.getCliTool(cli_tool);
  if (!startSpec || typeof startSpec.spawnPersistent !== 'function') {
    logFlow('warn', 'agent.start_unsupported', { agent_id, cli_tool });
    safeSend(ws, JSON.stringify({ type: 'agent.started', data: { agent_id, error: `${cli_tool} does not support persistent mode` } }));
    return;
  }

  try {
    // agent.start 创建的 slot 必须带 eventRef，否则后续 task.dispatch 走 fast path 时
    // slot.eventRef.current = onEvent 注入失败 → Claude CLI 的 thinking/text/tool_use
    // delta 全部丢弃，用户看到"一次性出结果"而非流式。
    const eventRef = { current: null };
    const result = spawnStreamJsonProcess(agent_id, null, system_prompt, false, null, null, null, eventRef, cli_tool);
    const { child, sessionId, sendPrompt } = result;

    // Wait briefly to detect immediate startup failure (same pattern as dispatchToPersistentSlot)
    await sleep(2000);
    if (child.exitCode !== null) {
      throw new Error(`Agent process exited immediately (code=${child.exitCode})`);
    }

    child.on('error', (err) => {
      logFlow('error', 'agent.process_error', { agent_id, cli_tool, session_id: sessionId, pid: child.pid, error: errorMessage(err) });
    });

    // Handle process exit — cleanup and notify backend
    child.on('close', (code) => {
      if (runningAgents.get(agent_id)?.process === child) {
        runningAgents.delete(agent_id);
      }
      agentTurnStates.delete(agent_id);
      logFlow(code === 0 ? 'info' : 'warn', 'agent.process_exit', {
        agent_id,
        cli_tool,
        session_id: sessionId,
        pid: child.pid,
        exit_code: code,
      });
      safeSend(ws, JSON.stringify({ type: 'agent.stopped', data: { agent_id, exit_code: code } }));
    });

    runningAgents.set(agent_id, {
      process: child,
      sessionId,
      cliTool: cli_tool,
      sendPrompt,
      currentConversationId: null,
      eventRef,
    });
    idleAgentConfigs.set(agent_id, { cliTool: cli_tool, sessionId, systemPrompt: system_prompt || '' });
    agentTurnStates.set(agent_id, 'idle');

    logFlow('info', 'agent.started', { agent_id, cli_tool, session_id: sessionId, pid: child.pid });
    safeSend(ws, JSON.stringify({ type: 'agent.started', data: { agent_id } }));
  } catch (error) {
    logFlow('error', 'agent.start_failed', { agent_id, cli_tool, error: errorMessage(error) });
    safeSend(ws, JSON.stringify({ type: 'agent.started', data: { agent_id, error: errorMessage(error) } }));
  }
}

function handleAgentStop(ws, payload) {
  const { agent_id } = payload;
  if (!agent_id) return;
  stopAgentProcess(agent_id);
  runningAgents.delete(agent_id);
  idleAgentConfigs.delete(agent_id);
  agentTurnStates.delete(agent_id);
  logFlow('info', 'agent.stopped', { agent_id });
  safeSend(ws, JSON.stringify({ type: 'agent.stopped', data: { agent_id } }));
}

function handleAgentRestart(ws, payload) {
  handleAgentStop(ws, payload);
  handleAgentStart(ws, payload);
}

// ---------------------------------------------------------------------------
// task.dispatch 处理器（从 connectWS 内联代码提取）
// ---------------------------------------------------------------------------

async function handleTaskDispatch(ws, data) {
  const task = {
    id: data.task_id,
    cli_tool: data.cli_tool,
    prompt: data.prompt,
    context_messages: data.context_messages,
    agent_id: data.agent_id,
    conversation_id: data.conversation_id,
    user_id: data.user_id,
    // message_id 由后端可选传入——有则流式（createAgentReply 路径），无则批处理（orchestrator / 文件浏览等）。
    message_id: data.message_id,
  };
  if (!task.id) {
    // task_id 缺失——几乎一定是后端发的字段名和 daemon 期望的不一致
    // （历史 bug：后端发 data.id，daemon 读 data.task_id → 静默 return → 后端超时）。
    // 打出实际收到的 keys，便于一眼定位字段名漂移。
    logFlow('error', 'task.dispatch_missing_task_id', {
      received_keys: Object.keys(data || {}),
      hint: 'daemon 读 data.task_id；若后端发了 data.id 则字段名不匹配',
    });
    return true;
  }

  // Dedup guard：同一 task_id 在 3 秒内只执行一次。
  // 但若新 dispatch 携带 message_id 而旧 dispatch 未携带（orchestrator vs createAgentReply
  // 先后到达），则替换——以有 message_id 的 dispatch 为准，流式优先。
  const dedupKey = `${task.id}`;
  const now = Date.now();
  const lastDispatch = recentDispatches.get(dedupKey);
  if (lastDispatch && (now - lastDispatch.ts) < 3000) {
    if (task.message_id && !lastDispatch.hasMessageId) {
      // 旧 dispatch 没有 message_id，新 dispatch 有——替换，用流式版本。
      logFlow('info', 'task.dispatch_replace_for_streaming', {
        task_id: task.id,
        reason: 'new dispatch has message_id, old does not',
        delta_ms: now - lastDispatch.ts,
      });
      // fall through（不 return，继续执行：新 dispatch 取代旧 dispatch 的 slot）
      // 注：旧的 sendPrompt 已经在 queueTail 里排队，无法取消。
      // 替换仅影响后续 eventRef.current —— 流式事件会路由到新 dispatch 的 onEvent。
    } else {
      logFlow('warn', 'task.dispatch_dedup_skip', {
        task_id: task.id,
        agent_id: task.agent_id,
        conversation_id: task.conversation_id,
        reason: 'duplicate within 3s',
        delta_ms: now - lastDispatch.ts,
      });
      return true;
    }
  }
  recentDispatches.set(dedupKey, { ts: now, hasMessageId: Boolean(task.message_id) });

  const { systemPrompt, userPrompt } = buildPromptParts(task);

  logFlow('info', 'task.dispatch_received', {
    task_id: task.id,
    cli_tool: task.cli_tool || 'unknown',
    agent_id: task.agent_id,
    conversation_id: task.conversation_id,
    user_id: task.user_id,
    message_id: task.message_id,
    prompt_len: typeof task.prompt === 'string' ? task.prompt.length : 0,
    context_len: typeof task.context_messages === 'string' ? task.context_messages.length : 0,
    system_prompt_len: systemPrompt.length,
    user_prompt_len: userPrompt.length,
  });
  // 创建本任务执行上下文：实例化所有注册的输出收集器（cards 等）。
  // taskCtx 随执行链路传递，承载 per-task 的临时资源；任务结束统一清理。
  const taskCtx = new TaskContext(task);
  taskCtx.attachCollectors();

  const hasMsgId = Boolean(taskCtx.messageId);
  const shouldStream = hasMsgId;

  // 流式输出：仅当 backend 传了 message_id（createAgentReply 路径）时启用。
  // orchestrator / 文件浏览等路径不传 message_id，走批处理（不做流式），避免
  // 多个 dispatch 共享同一 agent slot 时，无 message_id 的 dispatch 抢占流式通道。
  const streamBuffer = shouldStream
    ? new StreamBuffer({
        onFlush: (events) => {
          if (!events || events.length === 0) return;
          bus.emit('task.progress', {
            task_id: task.id,
            message_id: taskCtx.messageId,
            agent_id: taskCtx.agentId,
            conversation_id: taskCtx.conversationId,
            events,
          });
        },
      })
    : null;
  const onEvent = shouldStream && streamBuffer ? (ev) => streamBuffer.push(ev) : null;

  try {
    let result;
    if (
      task.cli_tool === 'claude' &&
      task.agent_id &&
      task.conversation_id &&
      process.env.AGENTHUB_DAEMON_DISABLE_STREAM_SLOT !== '1'
    ) {
      logFlow('info', 'task.execution_start', {
        task_id: task.id,
        cli_tool: task.cli_tool,
        agent_id: task.agent_id,
        conversation_id: task.conversation_id,
        mode: 'persistent_slot',
      });
      result = await dispatchToPersistentSlot(ws, task.agent_id, task.conversation_id, task.user_id, userPrompt, systemPrompt, taskCtx, task.cli_tool, onEvent);
    } else {
      logFlow('info', 'task.execution_start', {
        task_id: task.id,
        cli_tool: task.cli_tool,
        agent_id: task.agent_id,
        conversation_id: task.conversation_id,
        mode: 'legacy_spawn',
      });
      result = await executeTaskOnce(task, taskCtx);
      if (result === null) return true;
    }
    const artifacts = parseArtifacts(result);
    // MCP subprocess（如 deploy_project）产出的卡片已通过 ctx.emitCard →
    // POST /api/internal/task-cards 直接上报后端 TaskCardQueue，
    // daemon 主进程不再参与卡片收集。
    // cards 字段保留为空数组以兼容老的 task.complete WS 协议（后端 handleTaskComplete 仍读它）。
    const cards = [];
    bus.emit('task.completed', {
      task_id: task.id,
      cli_tool: task.cli_tool,
      agent_id: task.agent_id,
      conversation_id: task.conversation_id,
      result,
      artifacts,
      cards,
    });
  } catch (error) {
    bus.emit('task.failed', {
      task_id: task.id,
      cli_tool: task.cli_tool,
      agent_id: task.agent_id,
      conversation_id: task.conversation_id,
      error: error instanceof Error ? error.message : String(error),
    });
  } finally {
    if (streamBuffer) streamBuffer.close();
    taskCtx.cleanup();
  }
  return true;
}

// ---------------------------------------------------------------------------
// 注册 WS 消息处理器——新增消息类型只需在此处加一行 registerWsHandler。
// ---------------------------------------------------------------------------

registerWsHandler('pong', () => true); // no-op
registerWsHandler('ping', (ws) => { safeSend(ws, JSON.stringify({ type: 'pong' })); return true; });
registerWsHandler('agent.start', (ws, data) => {
  bus.emit('agent.start_request', { ws, data });
  return true;
});
registerWsHandler('agent.stop', (ws, data) => {
  bus.emit('agent.stop_request', { ws, data });
  return true;
});
registerWsHandler('agent.restart', (ws, data) => {
  bus.emit('agent.restart_request', { ws, data });
  return true;
});
registerWsHandler('task.dispatch', (ws, data) => {
  bus.emit('task.dispatch', { ws, data });
  return true;
});
registerWsHandler('task.cancel', (ws, data) => {
  bus.emit('task.cancel', { ws, data });
  return true;
});

// 部署已改为 MCP 工具（deploy_project / stop_deploy），不经 WS 下发。
// daemon 主进程只负责 TTL 清理（scanAndCleanupDeploys，见 main()）。

// ---------------------------------------------------------------------------
// 事件订阅者——业务逻辑与 WS 接收解耦
// ---------------------------------------------------------------------------

// agent 生命周期：start/stop/restart 请求 → 实际执行
bus.on('agent.start_request', ({ ws, data }) => {
  logFlow('info', 'ws.control_received', { type: 'agent.start', agent_id: data.agent_id, cli_tool: data.cli_tool });
  enqueueAgentStart(ws, data);
});
bus.on('agent.stop_request', ({ ws, data }) => {
  logFlow('info', 'ws.control_received', { type: 'agent.stop', agent_id: data.agent_id });
  handleAgentStop(ws, data);
});
bus.on('agent.restart_request', ({ ws, data }) => {
  logFlow('info', 'ws.control_received', { type: 'agent.restart', agent_id: data.agent_id, cli_tool: data.cli_tool });
  handleAgentRestart(ws, data);
});

// task 执行：dispatch → execute → completed/failed
bus.on('task.dispatch', async ({ ws, data }) => {
  await handleTaskDispatch(ws, data);
});

// task 结果回传——独立于执行逻辑，方便未来加重试/metrics
bus.on('task.completed', (info) => {
  sendTaskComplete({
    task_id: info.task_id,
    result: info.result,
    artifacts: info.artifacts,
    cards: info.cards || [],
  });
});
bus.on('task.failed', (info) => {
  sendTaskComplete({
    task_id: info.task_id,
    error: info.error,
  });
});

// task 流式增量：StreamBuffer flush 时 emit，转发到后端 WS 为 task.progress。
// 后端收到后转为 message.streaming 广播给前端。
bus.on('task.progress', (info) => {
  sendTaskProgress(info);
});

// task 取消：前端"停止生成"按钮 → 后端发 task.cancel → daemon SIGINT agent 进程。
// 取消是"尽力而为"——进程已被杀时无副作用。
//
// PR5：SIGINT 后通过 slot.eventRef.current 推一个 cancel 类型事件给 StreamBuffer，
// 让前端 reducer 切到 status=canceled（而不是 error）。events.js 的 cancelEvent
// 与 reducer（前端 + Go）的 case 'cancel' 分支对齐。
// 注：eventRef.current 可能为 null（turn 已结束），此时静默跳过——后端
// FinalizeStreaming 会通过 task.complete 路径正常收尾。
bus.on('task.cancel', ({ data } = {}) => {
  const { task_id, agent_id } = data || {};
  logFlow('info', 'task.cancel_received', { task_id, agent_id });
  if (!agent_id) return;
  const slot = runningAgents.get(agent_id);
  if (!slot || !slot.process) {
    logFlow('warn', 'task.cancel_no_slot', { task_id, agent_id });
    return;
  }
  try {
    if (process.platform === 'win32') {
      spawn('taskkill', ['/pid', String(slot.process.pid), '/T', '/SIGINT'], { windowsHide: true });
    } else {
      // SIGINT 让 Claude CLI 优雅中断（发 result 事件 + 退出），而不是 SIGKILL 的强杀。
      process.kill(slot.process.pid, 'SIGINT');
    }
    logFlow('info', 'task.cancel_signal_sent', { task_id, agent_id, pid: slot.process.pid });
  } catch (err) {
    logFlow('warn', 'task.cancel_failed', { task_id, agent_id, error: errorMessage(err) });
  }
  // 通过 eventRef 推 cancel 事件，让 dispatcher（StreamBuffer）flush 一个 cancel
  // 事件到后端，前端 reducer 据此切 status=canceled（与 backend watchdog
  // 行为对齐）。eventRef.current 可能为 null（turn 已结束），此时静默跳过——后端
  // FinalizeStreaming 会通过 task.complete 路径正常收尾。
  if (slot.eventRef && typeof slot.eventRef.current === 'function') {
    try {
      const { cancelEvent } = require('../cli/events');
      slot.eventRef.current(cancelEvent('用户取消生成'));
    } catch {
      /* onEvent 错误不阻塞 SIGINT 流程 */
    }
  }
});

async function connectWS(serverURL, apiKey) {
  if (!WebSocket) {
    logFlow('warn', 'ws.unavailable_fallback_to_polling', {
      reason: 'WebSocket not available. Please install ws or use Node.js 22+',
    });
    return pollTasks(serverURL, apiKey);
  }

  const url = new URL(serverURL);
  const wsPath = `${url.pathname.replace(/\/$/, '')}/daemon/ws`;
  const protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsURL = `${protocol}//${url.host}${wsPath}?token=${encodeURIComponent(apiKey)}`;

  let reconnectAttempts = 0;

  function connect() {
    logFlow('info', 'ws.connect_start', {
      server: `${protocol}//${url.host}${wsPath}`,
      reconnect_attempt: reconnectAttempts,
      machine_id: os.hostname(),
    });
    const ws = new WebSocket(wsURL);
    let pingTimer = null;
    let watchdogTimer = null;

    function resetWatchdog() {
      if (watchdogTimer) clearTimeout(watchdogTimer);
      watchdogTimer = setTimeout(() => {
        logFlow('warn', 'ws.inbound_watchdog_timeout', {
          timeout_ms: INBOUND_WATCHDOG_MS,
          machine_id: os.hostname(),
        });
        try { ws.close(); } catch { /* ignore */ }
      }, INBOUND_WATCHDOG_MS);
    }

    onWebSocket(ws, 'open', () => {
      currentDaemonWs = ws;
      reconnectAttempts = 0;
      logFlow('info', 'ws.connected', { machine_id: os.hostname(), server: `${protocol}//${url.host}${wsPath}` });
      resetWatchdog();
      // Send register message over WS
      const agents = scanAgents();
      const capabilities = detectCapabilities();
      ws.send(JSON.stringify({
        type: 'daemon.register',
        data: { machine_id: os.hostname(), agents, capabilities },
      }));
      logFlow('info', 'ws.register_sent', {
        machine_id: os.hostname(),
        agent_count: agents.length,
        capabilities,
        agents: agents.map((agent) => `${agent.name}:${agent.cli_tool}`),
      });
      flushPendingTaskCompletions();
      // Start ping interval
      pingTimer = setInterval(() => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: 'ping' }));
        }
      }, WS_PING_INTERVAL_MS);
    });

    onWebSocket(ws, 'message', async (data) => {
      resetWatchdog();
      let envelope;
      try {
        envelope = JSON.parse(data.toString());
      } catch {
        logFlow('warn', 'ws.message_parse_failed', { bytes: Buffer.byteLength(data) });
        return;
      }

      if (dispatchWsMessage(ws, envelope)) return;

      logFlow('warn', 'ws.unknown_message', { type: envelope.type });
    });

    onWebSocket(ws, 'close', (code, reason) => {
      if (currentDaemonWs === ws) currentDaemonWs = null;
      if (pingTimer) clearInterval(pingTimer);
      if (watchdogTimer) clearTimeout(watchdogTimer);
      // Keep running agent processes alive across short control-channel reconnects.
      // In-flight task completions are buffered and flushed after reconnect.
      reconnectAttempts += 1;
      logFlow('warn', 'ws.closed', {
        code,
        reason: reason ? reason.toString() : '',
        reconnect_attempt: reconnectAttempts,
        retry_ms: WS_RECONNECT_DELAY_MS,
        running_agent_count: runningAgents.size,
        pending_completion_count: pendingTaskCompletions.size,
      });
      setTimeout(connect, WS_RECONNECT_DELAY_MS);
    });

    onWebSocket(ws, 'error', (error) => {
      logFlow('error', 'ws.error', { error: errorMessage(error), reconnect_attempt: reconnectAttempts });
      // close handler will trigger reconnect
    });
  }

  connect();

  // Keep process alive
  return new Promise(() => {});
}

// ── MCP 模式 ──────────────────────────────────────────────────────────────
// 以 stdio JSON-RPC（换行分隔）对外暴露一个 agenthub-platform MCP server，
// 让本机 Agent（Claude Code / Codex 等）可通过 MCP 工具操作 AgentHub 平台。
// 协议要求 stdout 只承载 JSON-RPC 报文，因此本模式所有日志改走 stderr。
// MCP 协议版本在顶部 CONFIG.mcpProtocolVersion 配置。

let cachedAgentJWT = null;
let cachedAgentJWTExp = 0;

// getAgentJWT 用机器 api-key 向后端换取 agent_management scoped JWT，并缓存到临近过期。
async function getAgentJWT(serverURL, apiKey, force) {
  const now = Date.now();
  if (!force && cachedAgentJWT && now < cachedAgentJWTExp - 30000) {
    return cachedAgentJWT;
  }
  const res = await requestJSON('GET', apiURL(serverURL, apiKey, '/daemon/agent-token'));
  const data = res && res.data ? res.data : res;
  if (!data || !data.token) {
    throw new Error('agent-token 响应缺少 token');
  }
  cachedAgentJWT = data.token;
  cachedAgentJWTExp = data.expires_at ? Date.parse(data.expires_at) : now + 4 * 60 * 1000;
  return cachedAgentJWT;
}

// apiURLWithToken 用指定 JWT 作为 token query 调用 /api（鉴权中间件支持 query token）。
function apiURLWithToken(serverURL, jwt, pathname, query) {
  const url = new URL(serverURL);
  url.pathname = `${url.pathname.replace(/\/$/, '')}${pathname}`;
  url.searchParams.set('token', jwt);
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== null) url.searchParams.set(key, String(value));
    }
  }
  url.hash = '';
  return url;
}

// callApi 携带 scoped JWT 调用平台 REST API，401 时刷新一次 token 重试。
async function callApi(serverURL, apiKey, method, pathname, options = {}) {
  let lastError;
  for (let attempt = 0; attempt < 2; attempt += 1) {
    const jwt = await getAgentJWT(serverURL, apiKey, attempt === 1);
    const url = apiURLWithToken(serverURL, jwt, pathname, options.query);
    try {
      return await requestJSON(method, url, options.body);
    } catch (error) {
      lastError = error;
      if (attempt === 0 && /HTTP 401/.test(error.message)) {
        cachedAgentJWT = null;
        continue;
      }
      throw error;
    }
  }
  throw lastError;
}

// callMcpApi 用 daemon token（非 JWT、非 machine API key）调用 /mcp/... 端点。
// daemon token 从 CLI --daemon-token 或环境变量获取，与后端 config daemon.token 一致。
async function callMcpApi(serverURL, daemonToken, method, pathname, options = {}, userId) {
  const url = new URL(serverURL);
  url.pathname = `${url.pathname.replace(/\/$/, '')}${pathname}`;
  if (options.query) {
    for (const [key, value] of Object.entries(options.query)) {
      if (value !== undefined && value !== null) url.searchParams.set(key, String(value));
    }
  }
  if (userId) url.searchParams.set('user_id', userId);
  return requestJSON(method, url, options.body, daemonToken);
}

const MCP_TOOLS = [
  {
    name: 'list_conversations',
    description: '列出当前用户的所有会话（私聊与群聊）。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: (args, ctx) => ctx.callApi('GET', '/api/conversations'),
  },
  {
    name: 'get_messages',
    description: '读取指定会话的历史消息，用于获取上下文。',
    inputSchema: {
      type: 'object',
      properties: {
        conversation_id: { type: 'string', description: '会话 ID' },
        limit: { type: 'integer', description: '返回条数，默认 50' },
      },
      required: ['conversation_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callApi(
      'GET',
      `/api/conversations/${encodeURIComponent(args.conversation_id)}/messages`,
      { query: { limit: args.limit || 50 } },
    ),
  },
  {
    name: 'create_group',
    description: '创建一个群聊。',
    inputSchema: {
      type: 'object',
      properties: {
        name: { type: 'string', description: '群名称（1-50 字符）' },
        member_ids: { type: 'array', items: { type: 'string' }, description: '初始成员用户 ID 列表（可选）' },
      },
      required: ['name'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('POST', '/mcp/groups', {
      body: { name: args.name, member_ids: args.member_ids || [] },
    }),
  },
  {
    name: 'list_agents',
    description: '列出当前用户可用的 Agent。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: (args, ctx) => ctx.callMcpApi('GET', '/mcp/agents'),
  },
  {
    name: 'get_agent_skill',
    description: '查看当前 Agent 已分配平台 Skill 的详细内容。先根据提示词中的 Skill 索引选择 name，再调用本工具渐进加载 detail。',
    inputSchema: {
      type: 'object',
      properties: {
        name: { type: 'string', description: '平台 Skill 名称，必须属于当前 Agent' },
      },
      required: ['name'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const name = typeof args.name === 'string' ? args.name.trim() : '';
      if (!name) throw new Error('name is required');
      const agent = await resolveCurrentAgent(ctx);
      if (!agent) throw new Error('current agent not found');
      const skills = parsePlatformSkills(agent.custom_skills);
      const skill = skills.find((item) => item.name.toLowerCase() === name.toLowerCase());
      if (!skill) throw new Error(`skill not found for current agent: ${name}`);
      return skill;
    },
  },
  {
    name: 'list_agent_candidates',
    description: '列出当前用户电脑上扫描到的 Agent 底座候选。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: (args, ctx) => ctx.callMcpApi('GET', '/mcp/daemon/agent-candidates'),
  },
  {
    name: 'list_conversation_agents',
    description: '列出指定会话中的智能体，用于了解当前会话可分派的 Agent。',
    inputSchema: {
      type: 'object',
      properties: {
        conversation_id: { type: 'string', description: '会话 ID（默认为当前会话）' },
      },
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const convId = args.conversation_id || ctx.conversationId || '';
      if (!convId) throw new Error('conversation_id is required (no conversation context)');
      const res = await ctx.callApi('GET', `/api/conversations/${encodeURIComponent(convId)}/agents`);
      if (!res || !Array.isArray(res.data)) return res;
      res.data = res.data.map(a => ({
        name: a.name,
        role: a.role,
        status: a.status,
        tags: a.tags || '',
      }));
      return res;
    },
  },
  {
    name: 'list_group_agents',
    description: '列出指定群聊中的智能体，包括名称、角色、状态和标签，用于 orchestrator 了解群内可分派 Agent。',
    inputSchema: {
      type: 'object',
      properties: {
        conversation_id: { type: 'string', description: '群聊会话 ID（默认为当前会话）' },
      },
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const convId = args.conversation_id || ctx.conversationId || '';
      const res = await ctx.callApi('GET', `/api/conversations/${encodeURIComponent(convId)}/agents`);
      if (!res || !Array.isArray(res.data)) return res;
      // 只返回 orch 需要的关键字段
      res.data = res.data.map(a => ({
        name: a.name,
        role: a.role,
        status: a.status,
        tags: a.tags || '',
      }));
      return res;
    },
  },
  // ── 任务管理（通过 /mcp/ 端点，daemon token 鉴权） ──
  {
    name: 'list_tasks',
    description: '列出任务。默认列出当前会话的任务，可指定 conversation_id 和 status 过滤。',
    inputSchema: {
      type: 'object',
      properties: {
        conversation_id: { type: 'string', description: '会话 ID（默认为当前会话）' },
        status: { type: 'string', description: '按状态过滤（todo/in_progress/blocked/done/cancelled）' },
      },
      additionalProperties: false,
    },
    run: (args, ctx) => {
      const query = {};
      query.conversation_id = args.conversation_id || ctx.conversationId || '';
      if (args.status) query.status = args.status;
      return ctx.callMcpApi('GET', '/mcp/tasks', { query });
    },
  },
  {
    name: 'create_task',
    description: '创建一个任务。默认关联到当前会话。',
    inputSchema: {
      type: 'object',
      properties: {
        title: { type: 'string', description: '任务标题' },
        description: { type: 'string', description: '任务描述（可选）' },
        conversation_id: { type: 'string', description: '会话 ID（默认为当前会话）' },
        assignee_id: { type: 'string', description: '指派给用户的 ID（可选）' },
        agent_id: { type: 'string', description: '关联的 Agent ID（可选）' },
        priority: { type: 'string', description: '优先级（low/medium/high，默认 medium）' },
      },
      required: ['title'],
      additionalProperties: false,
    },
    run: (args, ctx) => {
      const body = { title: args.title };
      body.conversation_id = args.conversation_id || ctx.conversationId || '';
      if (args.description) body.description = args.description;
      if (args.assignee_id) body.assignee_id = args.assignee_id;
      if (args.agent_id) body.agent_id = args.agent_id;
      if (args.priority) body.priority = args.priority;
      return ctx.callMcpApi('POST', '/mcp/tasks', { body });
    },
  },
  {
    name: 'update_task',
    description: '更新任务属性（标题、描述、优先级、指派人等）。',
    inputSchema: {
      type: 'object',
      properties: {
        id: { type: 'string', description: '任务 ID' },
        title: { type: 'string', description: '新标题' },
        description: { type: 'string', description: '新描述' },
        priority: { type: 'string', description: '优先级（low/medium/high）' },
        assignee_id: { type: 'string', description: '指派人 ID' },
        agent_id: { type: 'string', description: '关联 Agent ID' },
      },
      required: ['id'],
      additionalProperties: false,
    },
    run: (args, ctx) => {
      const body = {};
      if (args.title) body.title = args.title;
      if (args.description) body.description = args.description;
      if (args.priority) body.priority = args.priority;
      if (args.assignee_id) body.assignee_id = args.assignee_id;
      if (args.agent_id) body.agent_id = args.agent_id;
      return ctx.callMcpApi('PUT', `/mcp/tasks/${encodeURIComponent(args.id)}`, { body });
    },
  },
  {
    name: 'move_task_status',
    description: '移动任务状态。',
    inputSchema: {
      type: 'object',
      properties: {
        id: { type: 'string', description: '任务 ID' },
        status: { type: 'string', description: '目标状态（todo/in_progress/blocked/done/cancelled）' },
      },
      required: ['id', 'status'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi(
      'POST',
      `/mcp/tasks/${encodeURIComponent(args.id)}/status`,
      { body: { status: args.status } },
    ),
  },
  {
    name: 'delete_task',
    description: '删除一个任务。',
    inputSchema: {
      type: 'object',
      properties: {
        id: { type: 'string', description: '任务 ID' },
      },
      required: ['id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('DELETE', `/mcp/tasks/${encodeURIComponent(args.id)}`),
  },
  // ── 群组信息 ──
  {
    name: 'get_group_info',
    description: '获取群聊信息。默认获取当前会话对应的群组。',
    inputSchema: {
      type: 'object',
      properties: {
        group_id: { type: 'string', description: '群组 ID（默认为当前会话 ID）' },
      },
      additionalProperties: false,
    },
    run: (args, ctx) => {
      const gid = args.group_id || ctx.conversationId || '';
      if (!gid) throw new Error('group_id is required (no conversation context)');
      return ctx.callMcpApi('GET', `/mcp/groups/${encodeURIComponent(gid)}`);
    },
  },
  {
    name: 'list_group_members',
    description: '列出群聊成员。默认列出当前会话对应群组的成员。',
    inputSchema: {
      type: 'object',
      properties: {
        group_id: { type: 'string', description: '群组 ID（默认为当前会话 ID）' },
      },
      additionalProperties: false,
    },
    run: (args, ctx) => {
      const gid = args.group_id || ctx.conversationId || '';
      if (!gid) throw new Error('group_id is required (no conversation context)');
      return ctx.callMcpApi('GET', `/mcp/groups/${encodeURIComponent(gid)}/members`);
    },
  },
  {
    name: 'list_machines',
    description: '列出当前用户连接的电脑（daemon 机器）。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: (args, ctx) => ctx.callMcpApi('GET', '/mcp/daemon/machines'),
  },
  // ── Agent 管理 ──
  {
    name: 'get_agent_detail',
    description: '查询单个 Agent 的完整详情，包括名称、类型、CLI 工具、系统提示词、工具配置、状态、版本、机器名称等。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID' },
      },
      required: ['agent_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('GET', `/mcp/agents/${encodeURIComponent(args.agent_id)}`),
  },
  {
    name: 'update_agent_prompt',
    description: '更新 Agent 的系统提示词。会先获取当前完整信息，再只修改 system_prompt 字段。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID' },
        system_prompt: { type: 'string', description: '新的系统提示词' },
      },
      required: ['agent_id', 'system_prompt'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const agentId = args.agent_id;
      if (!agentId) throw new Error('agent_id is required');
      const systemPrompt = args.system_prompt;
      if (!systemPrompt) throw new Error('system_prompt is required');
      // 用 detail 端点获取完整信息（list 端点会裁剪 tools_config/custom_skills 等字段）
      const res = await ctx.callMcpApi('GET', `/mcp/agents/${encodeURIComponent(agentId)}`);
      const agent = res && res.data ? res.data : res;
      if (!agent) throw new Error(`agent not found: ${agentId}`);
      // 只改 system_prompt，其他字段原样传回。
      // 不转发 capabilities_json——detail 端点会清空它，写回会覆盖实际值。
      return ctx.callMcpApi('PUT', `/mcp/agents/${encodeURIComponent(agentId)}`, {
        body: {
          name: agent.name,
          cli_tool: agent.cli_tool,
          system_prompt: systemPrompt,
          tools_config: agent.tools_config,
          custom_skills: agent.custom_skills,
          enable_management_tools: agent.enable_management_tools,
        },
      });
    },
  },
  {
    name: 'start_agent',
    description: '启动指定的 Agent。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID' },
      },
      required: ['agent_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('POST', `/mcp/agents/${encodeURIComponent(args.agent_id)}/start`),
  },
  {
    name: 'stop_agent',
    description: '停止指定的 Agent。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID' },
      },
      required: ['agent_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('POST', `/mcp/agents/${encodeURIComponent(args.agent_id)}/stop`),
  },
  // ── 知识库 ──
  {
    name: 'list_knowledge_bases',
    description: '列出当前用户的知识库，包含 ID、名称、描述、可见性、文件数量等信息。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: (args, ctx) => ctx.callMcpApi('GET', '/mcp/knowledge-bases'),
  },
  {
    name: 'list_knowledge_files',
    description: '列出指定知识库中的文件，包含文件名、大小、类型、预览文本等信息。',
    inputSchema: {
      type: 'object',
      properties: {
        knowledge_base_id: { type: 'string', description: '知识库 ID' },
      },
      required: ['knowledge_base_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('GET', `/mcp/knowledge-bases/${encodeURIComponent(args.knowledge_base_id)}/files`),
  },
  {
    name: 'search_knowledge',
    description: '在指定知识库中按关键词搜索文件，基于文件的 preview_text 字段进行匹配过滤。',
    inputSchema: {
      type: 'object',
      properties: {
        knowledge_base_id: { type: 'string', description: '知识库 ID' },
        keyword: { type: 'string', description: '搜索关键词' },
      },
      required: ['knowledge_base_id', 'keyword'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const kbId = args.knowledge_base_id;
      const keyword = args.keyword;
      if (!kbId) throw new Error('knowledge_base_id is required');
      if (!keyword) throw new Error('keyword is required');
      const res = await ctx.callMcpApi('GET', `/mcp/knowledge-bases/${encodeURIComponent(kbId)}/files`);
      const files = res && Array.isArray(res.data) ? res.data : Array.isArray(res) ? res : [];
      const keywordLower = keyword.toLowerCase();
      return files.filter((f) => {
        const preview = typeof f.preview_text === 'string' ? f.preview_text : '';
        return preview.toLowerCase().includes(keywordLower);
      });
    },
  },
  {
    name: 'read_knowledge_file',
    description: '读取知识库文件的抽取文本内容。适合在搜索命中文件后按 file_id 获取完整可用上下文。',
    inputSchema: {
      type: 'object',
      properties: {
        knowledge_base_id: { type: 'string', description: '知识库 ID' },
        file_id: { type: 'string', description: '文件 ID' },
      },
      required: ['knowledge_base_id', 'file_id'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const kbId = args.knowledge_base_id;
      const fileId = args.file_id;
      if (!kbId) throw new Error('knowledge_base_id is required');
      if (!fileId) throw new Error('file_id is required');
      return ctx.callMcpApi('GET', `/mcp/knowledge-bases/${encodeURIComponent(kbId)}/files/${encodeURIComponent(fileId)}/text`);
    },
  },
  // ── 平台 Skills ──
  {
    name: 'list_platform_skills',
    description: '列出所有平台 Skill，包含名称、分类、描述和触发场景，用于为 Agent 分配 Skill。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: async (args, ctx) => {
      const res = await ctx.callMcpApi('GET', '/mcp/platform-skills');
      const skills = res && Array.isArray(res.data) ? res.data : Array.isArray(res) ? res : [];
      return skills.map((s) => ({ name: s.name, category: s.category, description: s.description, trigger: s.trigger }));
    },
  },
  // ── Agent 自建 ──
  {
    name: 'create_agent',
    description: '创建自建 Agent。需要提供名称和系统提示词，可选指定工具模板、CLI 工具和标签。',
    inputSchema: {
      type: 'object',
      properties: {
        name: { type: 'string', description: 'Agent 名称（必填）' },
        system_prompt: { type: 'string', description: '系统提示词（必填）' },
        toolset: { type: 'string', description: '工具模板名（none/basic/tasks/orchestrator/agent_builder/agent_manager/knowledge），默认 none' },
        cli_tool: { type: 'string', description: 'CLI 工具名，默认 claude' },
        tags: { type: 'string', description: '标签' },
      },
      required: ['name', 'system_prompt'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const name = args.name;
      if (!name) throw new Error('name is required');
      const systemPrompt = args.system_prompt;
      if (!systemPrompt) throw new Error('system_prompt is required');
      const cliTool = args.cli_tool || 'claude';
      const toolset = args.toolset || 'none';
      const tpl = TOOLSET_TEMPLATES[toolset] || [];
      const toolsConfig = JSON.stringify({ toolset, allowed_tools: tpl });
      const body = { name, cli_tool: cliTool, system_prompt: systemPrompt, tools_config: toolsConfig };
      if (args.tags) body.tags = args.tags;
      const candidatesRes = await ctx.callMcpApi('GET', '/mcp/daemon/agent-candidates');
      const candidates = candidatesRes && Array.isArray(candidatesRes.data) ? candidatesRes.data : Array.isArray(candidatesRes) ? candidatesRes : [];
      const candidate = candidates.find((item) => item && item.cli_tool === cliTool);
      if (candidate && candidate.id) {
        return ctx.callMcpApi('POST', `/mcp/daemon/agent-candidates/${encodeURIComponent(candidate.id)}/add`, { body });
      }
      return ctx.callMcpApi('POST', '/mcp/agents', { body });
    },
  },
  {
    name: 'update_agent',
    description: '更新 Agent 配置，只改传入的字段。可修改名称、系统提示词、工具模板、自定义工具列表、平台 Skill 分配和标签。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID（必填）' },
        name: { type: 'string', description: '新名称' },
        system_prompt: { type: 'string', description: '新系统提示词' },
        toolset: { type: 'string', description: '切换工具模板' },
        allowed_tools: { type: 'array', items: { type: 'string' }, description: '自定义工具列表' },
        skills: { type: 'array', items: { type: 'string' }, description: '平台 Skill 名称列表，传入后覆盖当前 Agent 的 Skill 分配。传空数组清空所有 Skill' },
        tags: { type: 'string', description: '新标签' },
      },
      required: ['agent_id'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const agentId = args.agent_id;
      if (!agentId) throw new Error('agent_id is required');
      // Fetch current agent
      const res = await ctx.callMcpApi('GET', `/mcp/agents/${encodeURIComponent(agentId)}`);
      const agent = res && res.data ? res.data : res;
      if (!agent || typeof agent !== 'object') throw new Error(`agent not found: ${agentId}`);
      const body = {
        name: agent.name,
        cli_tool: agent.cli_tool,
        system_prompt: agent.system_prompt,
        tools_config: agent.tools_config,
        // 不转发 capabilities_json——detail 端点清空它，写回会覆盖实际值
        custom_skills: agent.custom_skills,
        enable_management_tools: agent.enable_management_tools,
      };
      // Override only provided fields
      if (args.name) body.name = args.name;
      if (args.system_prompt) body.system_prompt = args.system_prompt;
      if (args.tags) body.tags = args.tags;
      if (args.toolset) {
        const tpl = TOOLSET_TEMPLATES[args.toolset] || [];
        body.tools_config = JSON.stringify({ toolset: args.toolset, allowed_tools: tpl });
      }
      if (Array.isArray(args.allowed_tools) && args.allowed_tools.length > 0) {
        const tools = args.allowed_tools.filter((t) => typeof t === 'string' && t);
        if (tools.length > 0) {
          body.tools_config = JSON.stringify({ toolset: '', allowed_tools: tools });
        }
      }
      // Skills: fetch platform skills, filter by name, build custom_skills
      if (Array.isArray(args.skills)) {
        const skillNames = new Set(args.skills.filter((s) => typeof s === 'string' && s));
        const psRes = await ctx.callMcpApi('GET', '/mcp/platform-skills');
        const allSkills = psRes && Array.isArray(psRes.data) ? psRes.data : Array.isArray(psRes) ? psRes : [];
        const matched = allSkills.filter((s) => skillNames.has(s.name));
        body.custom_skills = JSON.stringify(matched);
      }
      return ctx.callMcpApi('PUT', `/mcp/agents/${encodeURIComponent(agentId)}`, { body });
    },
  },
  {
    name: 'delete_agent',
    description: '删除自建 Agent。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID（必填）' },
      },
      required: ['agent_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('DELETE', `/mcp/agents/${encodeURIComponent(args.agent_id)}`),
  },
  {
    name: 'list_toolsets',
    description: '列出可用的工具模板及其描述，用于创建或更新 Agent 时选择合适的工具配置。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: () => {
      if (TOOLSET_TEMPLATES && TOOLSET_TEMPLATES.length > 0) return TOOLSET_TEMPLATES;
      return [
        { name: 'none', label: '无工具', description: '不分配任何平台工具' },
        { name: 'basic', label: '基础群聊', description: '基础群聊工具' },
        { name: 'tasks', label: '任务协作', description: '任务看板 CRUD' },
        { name: 'orchestrator', label: 'Orchestrator', description: '编排器模板' },
        { name: 'agent_builder', label: 'Agent 创建', description: 'Agent 创建工具' },
        { name: 'agent_manager', label: 'Agent 管理', description: 'Agent 管理工具' },
        { name: 'knowledge', label: '知识库', description: '知识库工具' },
      ];
    },
  },
  {
    name: 'deploy_project',
    description: '在本机用 Docker 部署项目目录到公网（cloudflared 隧道）。要求 source_dir 必须含 Dockerfile（由你负责编写：FROM + 业务构建步骤 + EXPOSE <端口>）。平台执行 docker build/run + 公网隧道，返回 URL（4 小时后自动清理）。port 参数对应 Dockerfile 的 EXPOSE 端口（默认 80）。需要本机安装 Docker。',
    inputSchema: {
      type: 'object',
      properties: {
        source_dir: {
          type: 'string',
          description: '代码目录绝对路径（必填，必须含 Dockerfile）。',
        },
        port: {
          type: 'number',
          description: '容器监听端口，对应 Dockerfile 的 EXPOSE（默认 80）。',
        },
      },
      required: ['source_dir'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      // 1. docker 可用性检查
      if (!detectDocker()) {
        return {
          deployed: false,
          error: '本机未安装 Docker 或 Docker 未运行。请先安装 Docker。',
        };
      }

      // 2. 校验 source_dir（agent 的真实代码目录，Dockerfile 必须在里面）
      const sourceDir = path.resolve(args.source_dir || '');
      if (!sourceDir || !fs.existsSync(sourceDir) || !fs.statSync(sourceDir).isDirectory()) {
        return { deployed: false, error: `源码目录不存在或未指定: ${sourceDir || '(空)'}。必须传 source_dir 指明代码目录。` };
      }
      const port = Number(args.port) || 80;

      // 3. 执行部署（平台纯执行器：build + run + 隧道；Dockerfile 由 agent 负责）
      const deployId = crypto.randomUUID();
      logFlow('info', 'deploy.tool_start', { deploy_id: deployId, source_dir: sourceDir, port });
      let result;
      try {
        result = await executeDeploy(deployId, sourceDir, port);
      } catch (e) {
        logFlow('error', 'deploy.tool_failed', { deploy_id: deployId, error: errorMessage(e) });
        return { deployed: false, deploy_id: deployId, error: errorMessage(e) };
      }

      // 5. 写状态文件（供 daemon 主进程 TTL 清理）
      const createdAt = Date.now();
      const expiresAt = createdAt + DEPLOY_TTL_MS;
      writeDeployState({
        deployId,
        containerName: result.containerName,
        port: result.port,
        tunnelPid: result.tunnelProcess ? result.tunnelProcess.pid : null,
        url: result.url,
        workDir: result.workDir,
        createdAt,
        expiresAt,
      });

      // 6. 部署成功卡片通过 ctx.emitCard 上报后端 task-card 队列，
      // daemon 主进程 createAgentReply 时 drain 合并到 message.cards_json。
      // 与旧的 ctx.taskContext.pushDaemonCard 不同，emitCard 走 HTTP
      // 跨进程传给后端，不依赖 daemon 主进程内存共享。
      const expiresLocal = new Date(expiresAt).toLocaleString('zh-CN', { hour12: false });
      const card = {
        type: 'info',
        title: '部署完成',
        fields: {
          '访问地址': result.url,
          '容器': result.containerName,
          '有效期': `${expiresLocal}（4 小时后自动停止）`,
          '部署 ID': deployId,
        },
      };
      if (ctx && typeof ctx.emitCard === 'function') {
        await ctx.emitCard(card);
      }

      logFlow('info', 'deploy.tool_done', { deploy_id: deployId, url: result.url, expires_at: expiresLocal });
      return {
        deployed: true,
        deploy_id: deployId,
        url: result.url,
        container: result.containerName,
        expires_at: expiresLocal,
      };
    },
  },
  {
    name: 'stop_deploy',
    description: '停止一个正在运行的部署（docker stop + 关闭隧道）。传入 deploy_project 返回的 deploy_id。',
    inputSchema: {
      type: 'object',
      properties: {
        deploy_id: {
          type: 'string',
          description: '要停止的部署 ID（deploy_project 返回的 deploy_id）',
        },
      },
      required: ['deploy_id'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      if (!args.deploy_id) {
        return { stopped: false, error: '缺少 deploy_id 参数' };
      }
      const stopped = stopDeploy(args.deploy_id);
      return {
        stopped,
        deploy_id: args.deploy_id,
        message: stopped ? '部署已停止' : '未找到该部署（可能已过期或已停止）',
      };
    },
  },
];

const DEFAULT_AGENT_TOOLS = [
  'list_group_agents',
  'get_messages',
  'get_agent_skill',
  'list_tasks',
  'create_task',
  'update_task',
  'move_task_status',
];
const NO_AGENT_TOOLS = [];
const MANAGEMENT_TOOL_NAMES = ['create_agent', 'update_agent', 'delete_agent'];

// TOOLSET_TEMPLATES is populated from the backend API at startup (see fetch below).
const TOOLSET_TEMPLATES = {};

function normalizeToolName(value) {
  return typeof value === 'string' ? value.trim() : '';
}

function uniqueToolNames(values) {
  return [...new Set(values.map(normalizeToolName).filter(Boolean))];
}

function parseToolsConfig(raw) {
  if (!raw || typeof raw !== 'string') return { ok: false, config: null };
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { ok: false, config: null };
    }
    return { ok: true, config: parsed };
  } catch {
    return { ok: false, config: null };
  }
}

function allowedToolsFromConfig(raw) {
  const parsed = parseToolsConfig(raw);
  if (!parsed.ok) return NO_AGENT_TOOLS;
  const config = parsed.config;
  if (!config) return NO_AGENT_TOOLS;
  if (Array.isArray(config.allowed_tools) && config.allowed_tools.length > 0) return uniqueToolNames(config.allowed_tools);
  if (Array.isArray(config.tools) && config.tools.length > 0) return uniqueToolNames(config.tools);
  if (typeof config.toolset === 'string' && Object.prototype.hasOwnProperty.call(TOOLSET_TEMPLATES, config.toolset)) {
    return TOOLSET_TEMPLATES[config.toolset];
  }
  return NO_AGENT_TOOLS;
}

async function resolveCurrentAgent(ctx) {
  if (!ctx.agentId) return null;
  if (ctx.currentAgent !== undefined) return ctx.currentAgent;
  const res = await ctx.callMcpApi('GET', `/mcp/agents/${encodeURIComponent(ctx.agentId)}`);
  const agent = res && res.data ? res.data : (res && typeof res === 'object' && !Array.isArray(res) ? res : null);
  ctx.currentAgent = (agent && agent.id === ctx.agentId) ? agent : null;
  return ctx.currentAgent;
}

function parsePlatformSkills(raw) {
  if (!raw || typeof raw !== 'string') return [];
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter((item) => item && typeof item === 'object' && typeof item.name === 'string' && item.name.trim())
      .map((item) => ({
        name: item.name.trim(),
        description: typeof item.description === 'string' ? item.description : '',
        trigger: typeof item.trigger === 'string' ? item.trigger : '',
        detail: typeof item.detail === 'string' ? item.detail : '',
      }));
  } catch {
    return [];
  }
}

async function resolveAllowedTools(ctx) {
  if (!ctx.agentId) return NO_AGENT_TOOLS;
  if (ctx.allowedTools !== null) return ctx.allowedTools;
  const agent = await resolveCurrentAgent(ctx);
  if (!agent) {
    ctx.allowedTools = NO_AGENT_TOOLS;
    return ctx.allowedTools;
  }
  let tools = allowedToolsFromConfig(agent.tools_config);
  // enable_management_tools 为 true 时自动追加管理类工具
  if (agent.enable_management_tools) {
    const toolSet = new Set(tools);
    for (const mt of MANAGEMENT_TOOL_NAMES) {
      toolSet.add(mt);
    }
    tools = [...toolSet];
  }
  ctx.allowedTools = tools;
  return ctx.allowedTools;
}

async function isToolAllowed(ctx, toolName) {
  const allowed = await resolveAllowedTools(ctx);
  return allowed.includes(toolName);
}

function writeMcp(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}

async function handleMcpMessage(line, toolMap, ctx) {
  let msg;
  try {
    msg = JSON.parse(line);
  } catch {
    return;
  }
  const { id, method, params } = msg;

  if (method === 'initialize') {
    writeMcp({
      jsonrpc: '2.0',
      id,
      result: {
        protocolVersion: MCP_PROTOCOL_VERSION,
        capabilities: { tools: {} },
        serverInfo: { name: 'agenthub-platform', version: '0.1.0' },
      },
    });
    return;
  }
  if (method === 'notifications/initialized' || method === 'initialized') {
    return;
  }
  if (method === 'ping') {
    writeMcp({ jsonrpc: '2.0', id, result: {} });
    return;
  }
  if (method === 'tools/list') {
    const allowed = await resolveAllowedTools(ctx);
    const tools = MCP_TOOLS
      .filter((tool) => allowed.includes(tool.name))
      .map(({ name, description, inputSchema }) => ({ name, description, inputSchema }));
    writeMcp({
      jsonrpc: '2.0',
      id,
      result: { tools },
    });
    return;
  }
  if (method === 'tools/call') {
    const toolName = params && params.name;
    const tool = toolMap.get(toolName);
    if (!tool) {
      writeMcp({ jsonrpc: '2.0', id, error: { code: -32602, message: `未知工具: ${toolName}` } });
      return;
    }
    if (!(await isToolAllowed(ctx, toolName))) {
      writeMcp({
        jsonrpc: '2.0',
        id,
        result: { content: [{ type: 'text', text: `工具未授权: ${toolName}` }], isError: true },
      });
      return;
    }
    try {
      const res = await tool.run((params && params.arguments) || {}, ctx);
      const data = res && Object.prototype.hasOwnProperty.call(res, 'data') ? res.data : res;
      writeMcp({
        jsonrpc: '2.0',
        id,
        result: { content: [{ type: 'text', text: JSON.stringify(data, null, 2) }] },
      });
    } catch (error) {
      writeMcp({
        jsonrpc: '2.0',
        id,
        result: { content: [{ type: 'text', text: `调用失败: ${error.message}` }], isError: true },
      });
    }
    return;
  }
  if (id !== undefined && id !== null) {
    writeMcp({ jsonrpc: '2.0', id, error: { code: -32601, message: `方法不存在: ${method}` } });
  }
}

async function runMcpServer(serverURL, apiKey) {
  const daemonToken = readArg('--daemon-token') || process.env.AGENTHUB_DAEMON_TOKEN || '';
  const ctx = {
    conversationId: readArg('--conversation-id') || process.env.AGENTHUB_CONVERSATION_ID || null,
    userId: readArg('--user-id') || process.env.AGENTHUB_USER_ID || null,
    agentId: readArg('--agent-id') || process.env.AGENTHUB_AGENT_ID || null,
    taskId: readArg('--task-id') || process.env.AGENTHUB_TASK_ID || null,
    allowedTools: null,
    currentAgent: undefined,
    callApi: (method, pathname, options) => callApi(serverURL, apiKey, method, pathname, options),
    callMcpApi: (method, pathname, options) => callMcpApi(serverURL, daemonToken, method, pathname, options, ctx.userId),
    // emitCard 把 MCP subprocess 工具产出的卡片推到后端 task-card 队列，
    // daemon 主进程 createAgentReply 时 Drain 合并到 message.cards_json。
    // 抽象点：工具不需要知道后端怎么传——只管把 card 对象给到 ctx。
    // 缺 taskId（CLI 未注入 --task-id）时仅记日志，不抛错，保证工具不因此失败。
    emitCard: async (card) => {
      if (!card) return;
      if (!ctx.taskId) {
        logFlow('warn', 'card.emit_no_task', { card_type: card.type || 'unknown' });
        return;
      }
      try {
        await callMcpApi(serverURL, daemonToken, 'POST', '/api/internal/task-cards', {
          body: { task_id: ctx.taskId, card },
        }, ctx.userId);
      } catch (err) {
        // 上报失败不应让工具失败——卡片是辅助产物，工具主结果仍应返回
        logFlow('warn', 'card.emit_failed', {
          task_id: ctx.taskId,
          card_type: card.type || 'unknown',
          error: errorMessage(err),
        });
      }
    },
  };
  const toolMap = new Map(MCP_TOOLS.map((tool) => [tool.name, tool]));

  // 从后端拉取工具集模板到 TOOLSET_TEMPLATES，失败时保持为空对象（工具集解析将回退到 NO_AGENT_TOOLS）
  try {
    const templatesRes = await callMcpApi(serverURL, daemonToken, 'GET', '/api/tools/builtin-templates', {}, ctx.userId);
    const data = templatesRes && templatesRes.data ? templatesRes.data : templatesRes;
    if (data && Array.isArray(data)) {
      for (const tpl of data) {
        if (tpl && typeof tpl.name === 'string' && Array.isArray(tpl.tool_names)) {
          TOOLSET_TEMPLATES[tpl.name] = tpl.tool_names;
        }
      }
      logFlow('info', 'mcp.templates_fetched', { count: data.length });
    }
  } catch (err) {
    logFlow('warn', 'mcp.templates_fetch_failed', { error: errorMessage(err), fallback: 'hardcoded' });
  }

  let buffer = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', (chunk) => {
    buffer += chunk;
    let index = buffer.indexOf('\n');
    while (index >= 0) {
      const line = buffer.slice(0, index).trim();
      buffer = buffer.slice(index + 1);
      if (line) void handleMcpMessage(line, toolMap, ctx);
      index = buffer.indexOf('\n');
    }
  });
  process.stdin.on('end', () => process.exit(0));
  logFlow('info', 'mcp.ready', { tool_count: MCP_TOOLS.length });
}

async function main() {
  const serverURL = readArg('--server-url');
  const apiKey = readArg('--api-key');
  if (!serverURL || !apiKey) {
    logFlow('error', 'cli.usage_error', { usage: 'npx @hust-agenthub/daemon --server-url <url> --api-key <key> [--mcp]' });
    process.exit(2);
  }

  if (process.argv.includes('--mcp')) {
    await runMcpServer(serverURL, apiKey);
    return;
  }

  daemonConn.serverURL = serverURL;
  daemonConn.apiKey = apiKey;
  daemonConn.daemonToken = readArg('--daemon-token') || process.env.AGENTHUB_DAEMON_TOKEN || '';
  loadSessionMap();
  // 部署 TTL 管理：启动时清理上次遗留的过期部署，之后每 5 分钟扫一次。
  // MCP 工具 deploy_project 发起的部署写状态文件到 ~/.agenthub/deploys/，
  // 由这里负责 4 小时后自动停止（docker stop + kill cloudflared）。
  scanAndCleanupDeploys();
  setInterval(scanAndCleanupDeploys, 5 * 60 * 1000).unref();
  await register(serverURL, apiKey);
  ensureGlobalMcpConfigs(serverURL, apiKey);
  await connectWS(serverURL, apiKey);
}

if (require.main === module) {
  main().catch((error) => {
    logFlow('error', 'daemon.failed', { error: errorMessage(error) });
    process.exit(1);
  });
}

module.exports = {
  commandForTask,
  conversationSessions,
  ensureGitRepoForTask,
  ensureAgentHubCodexMcpConfig,
  executeTaskOnce,
  ensureOpenCodeMcpConfig,
  daemonConn,
  onWebSocket,
};
