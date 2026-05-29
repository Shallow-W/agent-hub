import type { ThemeConfig } from 'antd';

const theme: ThemeConfig = {
  token: {
    colorPrimary: '#16a365',
    borderRadius: 8,
    fontFamily: `-apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif`,
    colorBgContainer: '#ffffff',
    colorBgLayout: '#eef1f4',
    colorText: '#1f2328',
    colorTextSecondary: '#68727d',
    colorBorder: '#e7eaee',
    colorBorderSecondary: '#f1f3f5',
    boxShadow: '0 2px 8px rgba(15, 23, 42, 0.06)',
  },
  components: {
    Button: {
      borderRadius: 8,
      controlHeight: 36,
    },
    Input: {
      borderRadius: 8,
    },
    Card: {
      borderRadius: 14,
    },
    Modal: {
      borderRadius: 14,
    },
    Menu: {
      itemBorderRadius: 8,
      itemMarginInline: 4,
      itemHeight: 38,
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
