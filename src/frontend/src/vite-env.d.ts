declare module '*.module.css' {
  const classes: Record<string, string>;
  export default classes;
}

interface Window {
  agentHubDesktop?: {
    platform: string;
    isDesktop: boolean;
    backendBaseURL?: string;
    minimize: () => void;
    maximize: () => void;
    close: () => void;
    onMaximizeChange: (cb: (maximized: boolean) => void) => () => void;
  };
}
