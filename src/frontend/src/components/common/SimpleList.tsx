import React from 'react';
import { Empty, Spin } from 'antd';
import styles from './SimpleList.module.css';

interface SimpleListProps<T> {
  dataSource?: readonly T[];
  renderItem?: (item: T, index: number) => React.ReactNode;
  loading?: boolean;
  locale?: { emptyText?: React.ReactNode };
  split?: boolean;
  header?: React.ReactNode;
  footer?: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
  children?: React.ReactNode;
}

interface SimpleListItemProps {
  actions?: React.ReactNode[];
  className?: string;
  style?: React.CSSProperties;
  onClick?: React.MouseEventHandler<HTMLDivElement>;
  children?: React.ReactNode;
}

interface SimpleListItemMetaProps {
  avatar?: React.ReactNode;
  title?: React.ReactNode;
  description?: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
}

interface SimpleListItemComponent extends React.FC<SimpleListItemProps> {
  Meta: React.FC<SimpleListItemMetaProps>;
}

interface SimpleListComponent {
  <T>(props: SimpleListProps<T>): React.ReactElement;
  Item: SimpleListItemComponent;
}

const SimpleListItemMeta: React.FC<SimpleListItemMetaProps> = ({
  avatar,
  title,
  description,
  className,
  style,
}) => (
  <div className={`${styles.meta} ${className ?? ''}`} style={style}>
    {avatar && <div className={styles.avatar}>{avatar}</div>}
    <div className={styles.metaBody}>
      {title && <div className={styles.title}>{title}</div>}
      {description && <div className={styles.description}>{description}</div>}
    </div>
  </div>
);

const SimpleListItem = (({
  actions,
  className,
  style,
  onClick,
  children,
}: SimpleListItemProps) => {
  const visibleActions = actions?.filter(Boolean) ?? [];
  return (
    <div
      className={`${styles.item} ${onClick ? styles.clickable : ''} ${className ?? ''}`}
      style={style}
      role={onClick ? 'button' : 'listitem'}
      tabIndex={onClick ? 0 : undefined}
      onClick={onClick}
      onKeyDown={(event) => {
        if (!onClick) return;
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          event.currentTarget.click();
        }
      }}
    >
      <div className={styles.content}>{children}</div>
      {visibleActions.length > 0 && <div className={styles.actions}>{visibleActions}</div>}
    </div>
  );
}) as SimpleListItemComponent;

SimpleListItem.Meta = SimpleListItemMeta;

const SimpleListBase = <T,>({
  dataSource,
  renderItem,
  loading,
  locale,
  split = true,
  header,
  footer,
  className,
  style,
  children,
}: SimpleListProps<T>) => {
  if (loading) {
    return (
      <div className={styles.loading}>
        <Spin />
      </div>
    );
  }

  const items = dataSource && renderItem
    ? dataSource.map((item, index) => (
      <React.Fragment key={index}>{renderItem(item, index)}</React.Fragment>
    ))
    : children;

  const isEmpty = dataSource && dataSource.length === 0;
  if (isEmpty) {
    return <div className={styles.empty}>{locale?.emptyText ?? <Empty />}</div>;
  }

  return (
    <div className={`${styles.list} ${split ? styles.split : ''} ${className ?? ''}`} style={style} role="list">
      {header && <div className={styles.header}>{header}</div>}
      {items}
      {footer && <div className={styles.footer}>{footer}</div>}
    </div>
  );
};

export const SimpleList = SimpleListBase as SimpleListComponent;
SimpleList.Item = SimpleListItem;
