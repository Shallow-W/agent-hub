import { app, BrowserWindow, shell } from 'electron';
import { spawn } from 'node:child_process';
import { appendFileSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  buildBackendEnv,
  resolveBackendBinary,
  resolveConfigPath,
  resolveFrontendDist,
  resolveFrontendURL,
  shouldLaunchBackend,
  waitForHTTP,
} from './runtime.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
let backendProcess = null;

function writeDesktopLog(message) {
  try {
    const logDir = path.join(app.getPath('userData'), 'logs');
    mkdirSync(logDir, { recursive: true });
    appendFileSync(path.join(logDir, 'desktop.log'), `${new Date().toISOString()} ${message}\n`);
  } catch {
    // 日志写入失败不能影响桌面端启动。
  }
}

function createWindow() {
  const window = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 960,
    minHeight: 640,
    title: 'AgentHub',
    backgroundColor: '#f6f7fb',
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  window.webContents.setWindowOpenHandler(({ url }) => {
    if (/^https?:\/\//.test(url)) {
      shell.openExternal(url);
    }
    return { action: 'deny' };
  });

  window.loadURL(resolveFrontendURL({
    env: process.env,
    appPath: app.getAppPath(),
    resourcesPath: process.resourcesPath,
  }));
}

function startBackendIfNeeded() {
  if (!shouldLaunchBackend(process.env)) {
    return;
  }

  const binary = resolveBackendBinary({
    resourcesPath: process.resourcesPath,
    platform: process.platform,
  });
  const configPath = resolveConfigPath({ resourcesPath: process.resourcesPath });
  const frontendDist = resolveFrontendDist({ resourcesPath: process.resourcesPath });
  writeDesktopLog(`starting backend binary=${binary} config=${configPath} frontendDist=${frontendDist}`);
  backendProcess = spawn(binary, [], {
    cwd: process.resourcesPath,
    env: buildBackendEnv({
      baseEnv: process.env,
      configPath,
      frontendDist,
    }),
    stdio: ['ignore', 'pipe', 'pipe'],
    windowsHide: true,
  });

  backendProcess.stdout?.on('data', (chunk) => {
    writeDesktopLog(`backend stdout: ${chunk.toString().trim()}`);
  });

  backendProcess.stderr?.on('data', (chunk) => {
    writeDesktopLog(`backend stderr: ${chunk.toString().trim()}`);
  });

  backendProcess.on('error', (error) => {
    writeDesktopLog(`backend spawn error: ${error.message}`);
    backendProcess = null;
  });

  backendProcess.on('exit', (code, signal) => {
    writeDesktopLog(`backend exited code=${code ?? ''} signal=${signal ?? ''}`);
    backendProcess = null;
  });
}

app.whenReady().then(async () => {
  startBackendIfNeeded();
  if (backendProcess) {
    try {
      await waitForHTTP('http://127.0.0.1:8080/health');
    } catch (error) {
      // 后端启动失败时仍打开窗口，让用户看到前端的网络错误与登录状态。
      writeDesktopLog(`backend health wait failed: ${error instanceof Error ? error.message : String(error)}`);
    }
  }
  createWindow();

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('before-quit', () => {
  if (backendProcess) {
    backendProcess.kill();
    backendProcess = null;
  }
});
