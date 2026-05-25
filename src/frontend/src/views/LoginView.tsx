import React, { useState, useCallback } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import AuthLayout from '@/layout/AuthLayout';
import Input from '@/components/common/Input';
import Button from '@/components/common/Button';
import { useAuth } from '@/hooks/useAuth';
import styles from './LoginView.module.css';

const LoginView: React.FC = () => {
  const navigate = useNavigate();
  const { login, loading, error } = useAuth();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [localError, setLocalError] = useState('');

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setLocalError('');
      if (!username.trim() || !password.trim()) {
        setLocalError('请输入用户名和密码');
        return;
      }
      try {
        await login(username.trim(), password);
        navigate('/');
      } catch {
        setLocalError('用户名或密码错误');
      }
    },
    [username, password, login, navigate],
  );

  return (
    <AuthLayout>
      <form className={styles.form} onSubmit={handleSubmit}>
        {(error || localError) && (
          <div className={styles.error}>{localError || error}</div>
        )}
        <Input
          label="用户名"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          placeholder="请输入用户名"
          autoComplete="username"
        />
        <Input
          label="密码"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder="请输入密码"
          autoComplete="current-password"
        />
        <Button type="submit" disabled={loading}>
          {loading ? '登录中...' : '登录'}
        </Button>
        <div className={styles.footer}>
          还没有账号？<Link to="/register">注册</Link>
        </div>
      </form>
    </AuthLayout>
  );
};

export default LoginView;
