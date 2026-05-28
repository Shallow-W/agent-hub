import React, { useEffect, useState } from 'react';
import { Input, Modal } from 'antd';

interface NewConversationModalProps {
  open: boolean;
  onCancel: () => void;
  onCreate: (title: string) => Promise<void>;
}

const NewConversationModal: React.FC<NewConversationModalProps> = ({
  open,
  onCancel,
  onCreate,
}) => {
  const [title, setTitle] = useState('');

  useEffect(() => {
    if (open) {
      setTitle('');
    }
  }, [open]);

  const submit = async () => {
    await onCreate(title.trim() || '新对话');
  };

  return (
    <Modal
      title="新建对话"
      open={open}
      onOk={submit}
      onCancel={onCancel}
      okText="创建"
      cancelText="取消"
      destroyOnClose
    >
      <Input
        placeholder="对话标题（可选）"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        onPressEnter={submit}
        maxLength={50}
        autoFocus
      />
    </Modal>
  );
};

export default NewConversationModal;
