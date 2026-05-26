import type { ThemeConfig } from 'antd';

/** Ant Design 全局主题配置 */
const theme: ThemeConfig = {
  token: {
    colorPrimary: '#1677ff',
    borderRadius: 8,
    fontFamily: `-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif`,
    colorBgContainer: '#ffffff',
    colorBgLayout: '#f5f5f5',
    colorText: '#1a1a1a',
    colorTextSecondary: '#666',
  },
  components: {
    Button: {
      borderRadius: 8,
    },
    Input: {
      borderRadius: 8,
    },
    Card: {
      borderRadius: 12,
    },
    Modal: {
      borderRadius: 12,
    },
  },
};

export default theme;
