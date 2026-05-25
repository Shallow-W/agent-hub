import React from 'react';
import styles from './Input.module.css';

interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
}

const Input: React.FC<InputProps> = ({ label, error, className, id, ...rest }) => {
  const inputId = id ?? (label ? `input-${label}` : undefined);

  return (
    <div className={styles.wrapper}>
      {label && (
        <label htmlFor={inputId} className={styles.label}>
          {label}
        </label>
      )}
      <input
        id={inputId}
        className={`${styles.input} ${error ? styles.inputError : ''} ${className ?? ''}`}
        {...rest}
      />
      {error && <span className={styles.errorMsg}>{error}</span>}
    </div>
  );
};

export default Input;
