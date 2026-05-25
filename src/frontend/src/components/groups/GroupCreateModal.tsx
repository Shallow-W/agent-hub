import React, { useState } from 'react';
import { Modal, Form, Input, Select } from 'antd';
import { useFriendStore } from '@/store/friendStore';

interface GroupCreateModalProps {
  open: boolean;
  onCancel: () => void;
  onOk: (name: string, memberIds: string[]) => void;
}

const GroupCreateModal: React.FC<GroupCreateModalProps> = ({
  open,
  onCancel,
  onOk,
}) => {
  const { friends } = useFriendStore();
  const [form] = Form.useForm();
  const [confirmLoading, setConfirmLoading] = useState(false);

  const handleOk = async () => {
    try {
      const values = await form.validateFields();
      setConfirmLoading(true);
      onOk(values.groupName, values.members ?? []);
    } finally {
      setConfirmLoading(false);
    }
  };

  const handleAfterClose = () => {
    form.resetFields();
  };

  const friendOptions = friends.map((f) => ({
    label: f.friend_name ?? '未知用户',
    value: f.friend_id,
  }));

  return (
    <Modal
      title="创建群聊"
      open={open}
      onOk={handleOk}
      onCancel={onCancel}
      confirmLoading={confirmLoading}
      okText="创建"
      cancelText="取消"
      afterClose={handleAfterClose}
      destroyOnClose
    >
      <Form form={form} layout="vertical" autoComplete="off">
        <Form.Item
          name="groupName"
          label="群聊名称"
          rules={[{ required: true, message: '请输入群聊名称' }]}
        >
          <Input placeholder="请输入群聊名称" maxLength={50} />
        </Form.Item>
        <Form.Item name="members" label="选择成员">
          <Select
            mode="multiple"
            placeholder="搜索并选择好友"
            options={friendOptions}
            allowClear
            showSearch
            filterOption={(input, option) =>
              (option?.label as string)?.toLowerCase().includes(input.toLowerCase()) ?? false
            }
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default GroupCreateModal;
