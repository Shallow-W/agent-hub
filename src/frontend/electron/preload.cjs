const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('agentHubDesktop', {
  platform: process.platform,
  isDesktop: true,
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
