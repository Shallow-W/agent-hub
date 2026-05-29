import { useNavigate } from 'react-router-dom';
import { Button } from 'antd';
import styles from './NotFoundView.module.css';

export default function NotFoundView() {
  const navigate = useNavigate();

  return (
    <div className={styles.container}>
      <h1 className={styles.code}>404</h1>
      <p className={styles.message}>页面不存在</p>
      <Button type="primary" onClick={() => navigate('/')}>
        返回首页
      </Button>
    </div>
  );
}
