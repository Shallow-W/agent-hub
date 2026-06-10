import { app, BrowserWindow, shell, Menu, ipcMain } from 'electron';
import { appendFileSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { resolveFrontendURL } from './runtime.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

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
  // 隐藏系统菜单栏
  Menu.setApplicationMenu(null);

  const window = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 960,
    minHeight: 640,
    title: 'AgentHub',
    // 无边框窗口，与前端内容融合为一体
    frame: false,
    // 透明窗口，让 CSS border-radius 正确裁切圆角
    transparent: true,
    // 任务栏图标，与标题栏图标统一
    icon: path.join(__dirname, '..', 'public', 'favicon.ico'),
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  // 通知前端窗口最大化状态变化
  window.on('maximize', () => {
    window.webContents.send('window:maximized', true);
  });
  window.on('unmaximize', () => {
    window.webContents.send('window:maximized', false);
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

app.whenReady().then(async () => {
  // 注册窗口控制 IPC
  ipcMain.on('window:minimize', (event) => {
    BrowserWindow.fromWebContents(event.sender)?.minimize();
  });
  ipcMain.on('window:maximize', (event) => {
    const win = BrowserWindow.fromWebContents(event.sender);
    if (!win) return;
    if (win.isMaximized()) {
      win.unmaximize();
    } else {
      win.maximize();
    }
  });
  ipcMain.on('window:close', (event) => {
    BrowserWindow.fromWebContents(event.sender)?.close();
  });

  writeDesktopLog('desktop frontend only mode');
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
