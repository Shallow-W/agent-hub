const { contextBridge, ipcRenderer } = require('electron');

const DEFAULT_BACKEND_URL = 'http://10.11.221.79:8080';
const backendBaseURL = (process.env.AGENTHUB_BACKEND_URL || DEFAULT_BACKEND_URL).replace(/\/+$/, '');

contextBridge.exposeInMainWorld('agentHubDesktop', {
  platform: process.platform,
  isDesktop: true,
  backendBaseURL,
  // 窗口控制
  minimize: () => ipcRenderer.send('window:minimize'),
  maximize: () => ipcRenderer.send('window:maximize'),
  close: () => ipcRenderer.send('window:close'),
  // 查询窗口状态
  onMaximizeChange: (callback) => {
    const handler = (_event, isMaximized) => callback(isMaximized);
    ipcRenderer.on('window:maximized', handler);
    return () => ipcRenderer.removeListener('window:maximized', handler);
  },
});
