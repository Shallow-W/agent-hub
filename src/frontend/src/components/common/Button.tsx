import React from 'react';
import styles from './Button.module.css';

type ButtonVariant = 'primary' | 'secondary' | 'danger';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
}

const Button: React.FC<ButtonProps> = ({
  variant = 'primary',
  className,
  children,
  ...rest
}) => {
  const variantClass = {
    primary: styles.primary ?? '',
    secondary: styles.secondary ?? '',
    danger: styles.danger ?? '',
  };
  const cls = [styles.button, variantClass[variant], className]
    .filter(Boolean)
    .join(' ');

  return (
    <button className={cls} {...rest}>
      {children}
    </button>
  );
};

export default Button;
