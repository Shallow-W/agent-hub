const { contextBridge } = require('electron');

contextBridge.exposeInMainWorld('agentHubDesktop', {
  platform: process.platform,
  isDesktop: true,
});
