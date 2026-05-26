import type { ThemeConfig } from 'antd';

const theme: ThemeConfig = {
  token: {
    colorPrimary: '#1677ff',
    borderRadius: 10,
    fontFamily: `-apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif`,
    colorBgContainer: '#ffffff',
    colorBgLayout: '#f0f2f5',
    colorText: '#1a1a1a',
    colorTextSecondary: '#666',
    colorBorder: '#e5e7eb',
    colorBorderSecondary: '#f0f0f0',
    boxShadow: '0 2px 8px rgba(0, 0, 0, 0.06)',
  },
  components: {
    Button: {
      borderRadius: 10,
      controlHeight: 36,
    },
    Input: {
      borderRadius: 10,
    },
    Card: {
      borderRadius: 14,
    },
    Modal: {
      borderRadius: 14,
    },
    Menu: {
      itemBorderRadius: 8,
      itemMarginInline: 8,
      itemHeight: 40,
      iconSize: 16,
    },
    Avatar: {
      borderRadius: 50,
    },
    Badge: {
      dotSize: 8,
    },
  },
};

export default theme;
