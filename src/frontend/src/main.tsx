import React from 'react';
import ReactDOM from 'react-dom/client';
import { ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import App from './App';
import theme from './theme/antd';
import './styles/globals.css';

class ErrorBoundary extends React.Component<
  { children: React.ReactNode },
  { hasError: boolean }
> {
  state = { hasError: false };

  static getDerivedStateFromError() {
    return { hasError: true };
  }

  render() {
    if (this.state.hasError) {
      return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 12 }}>
          <h2 style={{ color: '#666' }}>页面出现错误</h2>
          <button
            onClick={() => { this.setState({ hasError: false }); window.location.reload(); }}
            style={{ padding: '8px 24px', borderRadius: 6, border: 'none', background: '#1677ff', color: '#fff', cursor: 'pointer' }}
          >
            刷新页面
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

const rootEl = document.getElementById('root');
if (!rootEl) {
  throw new Error('Root element not found');
}

ReactDOM.createRoot(rootEl).render(
  <React.StrictMode>
    <ConfigProvider theme={theme} locale={zhCN}>
      <ErrorBoundary><App /></ErrorBoundary>
    </ConfigProvider>
  </React.StrictMode>,
);
