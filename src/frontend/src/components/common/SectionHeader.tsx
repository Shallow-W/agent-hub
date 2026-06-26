import React, { type ReactNode } from 'react';
import styles from './SectionHeader.module.css';

interface SectionHeaderProps {
  icon?: ReactNode;
  title: string;
  description?: string;
  extra?: ReactNode;
  divider?: boolean;
}

export const SectionHeader: React.FC<SectionHeaderProps> = ({
  icon,
  title,
  description,
  extra,
  divider = true,
}) => {
  const headClass = `${styles.head} ${divider ? styles.withDivider : ''}`;
  return (
    <div className={headClass}>
      <div className={styles.titleBlock}>
        {icon && <span className={styles.icon}>{icon}</span>}
        <div className={styles.text}>
          <div className={styles.title}>{title}</div>
          {description && <div className={styles.description}>{description}</div>}
        </div>
      </div>
      {extra && <div className={styles.extra}>{extra}</div>}
    </div>
  );
};

export default SectionHeader;
