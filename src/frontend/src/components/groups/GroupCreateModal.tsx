import React, { useState } from 'react';
import { Modal, Form, Input, Checkbox, Empty } from 'antd';
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
  const friends = useFriendStore((s) => s.friends) ?? [];
  const [form] = Form.useForm();
  const [confirmLoading, setConfirmLoading] = useState(false);
  const [memberSearch, setMemberSearch] = useState('');

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
    setMemberSearch('');
  };

  const filteredFriends = memberSearch
    ? friends.filter((f) =>
        (f.friend_name ?? '').toLowerCase().includes(memberSearch.toLowerCase()),
      )
    : friends;

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
          <>
            <Input.Search
              placeholder="搜索好友..."
              allowClear
              value={memberSearch}
              onChange={(e) => setMemberSearch(e.target.value)}
              onClear={() => setMemberSearch('')}
              style={{ marginBottom: 8 }}
            />
            {filteredFriends.length === 0 ? (
              <Empty
                description="没有匹配的好友"
                image={Empty.PRESENTED_IMAGE_SIMPLE}
              />
            ) : (
              <Checkbox.Group
                style={{ display: 'flex', flexDirection: 'column', gap: 8, maxHeight: 200, overflowY: 'auto' }}
              >
                {filteredFriends.map((f) => (
                  <Checkbox key={f.friend_id} value={f.friend_id}>
                    {f.friend_name ?? '未知用户'}
                  </Checkbox>
                ))}
              </Checkbox.Group>
            )}
          </>
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default GroupCreateModal;
