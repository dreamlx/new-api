/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useState } from 'react';
import {
  Button,
  Form,
  Modal,
  Space,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, copy, showError, showSuccess } from '../../helpers';

const { Text } = Typography;
const BASE = '/api/admin/v2/platforms';

const Platform = () => {
  const { t } = useTranslation();
  const [platforms, setPlatforms] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingPlatform, setEditingPlatform] = useState(null);
  const [skModalVisible, setSkModalVisible] = useState(false);
  const [createdCredential, setCreatedCredential] = useState(null);

  const loadPlatforms = async () => {
    setLoading(true);
    try {
      const res = await API.get(`${BASE}?page=1&page_size=100`);
      if (res.data?.success) {
        setPlatforms(res.data.data?.items || []);
      } else {
        showError(res.data?.message || t('加载平台列表失败'));
      }
    } catch (error) {
      showError(error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadPlatforms();
  }, []);

  const openCreate = () => {
    setEditingPlatform(null);
    setModalVisible(true);
  };

  const openEdit = (record) => {
    setEditingPlatform(record);
    setModalVisible(true);
  };

  const submitPlatform = async (values) => {
    const payload = {
      name: values.name,
      status: values.status,
      shadow_user_id: values.shadow_user_id
        ? Number(values.shadow_user_id)
        : undefined,
    };

    try {
      const res = editingPlatform
        ? await API.patch(`${BASE}/${editingPlatform.id}`, payload)
        : await API.post(BASE, {
            ...payload,
            platform_id: values.platform_id,
          });

      if (!res.data?.success) {
        showError(res.data?.message || t('操作失败'));
        return;
      }

      showSuccess(editingPlatform ? t('平台更新成功') : t('平台创建成功'));
      setModalVisible(false);
      await loadPlatforms();

      if (!editingPlatform && res.data.data?.platform_sk) {
        setCreatedCredential(res.data.data);
        setSkModalVisible(true);
      }
    } catch (error) {
      showError(error);
    }
  };

  const deletePlatform = (record) => {
    Modal.confirm({
      title: t('确认删除平台?'),
      content: `${t('将永久禁用平台')} ${record.platform_id} ${t('并拒绝其后续 API 调用。')}`,
      okType: 'danger',
      onOk: async () => {
        try {
          const res = await API.delete(`${BASE}/${record.id}`);
          if (res.data?.success) {
            showSuccess(
              t('平台已删除') +
                `, ${t('已禁用')} ${res.data.data?.tokens_disabled || 0} token`,
            );
            await loadPlatforms();
          } else {
            showError(res.data?.message || t('删除失败'));
          }
        } catch (error) {
          showError(error);
        }
      },
    });
  };

  const columns = [
    {
      title: t('平台 ID'),
      dataIndex: 'platform_id',
      render: (text) => (
        <Space>
          <Text strong>{text}</Text>
          <Button
            size='small'
            type='tertiary'
            onClick={async () => {
              if (await copy(text)) showSuccess(t('复制成功'));
            }}
          >
            {t('复制')}
          </Button>
        </Space>
      ),
    },
    {
      title: t('平台名称'),
      dataIndex: 'name',
      render: (text) => text || '-',
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (status) =>
        status === 1 ? (
          <Tag color='green'>{t('启用')}</Tag>
        ) : (
          <Tag color='grey'>{t('禁用')}</Tag>
        ),
    },
    {
      title: t('影子用户 ID'),
      dataIndex: 'shadow_user_id',
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_at',
      render: (text) => text || '-',
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      render: (_, record) => (
        <Space>
          <Button size='small' onClick={() => openEdit(record)}>
            {t('编辑')}
          </Button>
          <Button
            size='small'
            type='danger'
            onClick={() => deletePlatform(record)}
          >
            {t('删除')}
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <div className='mt-[60px] px-2'>
      <div className='mb-4 flex flex-col md:flex-row md:items-center md:justify-between gap-3'>
        <div>
          <Typography.Title heading={4}>{t('平台管理')}</Typography.Title>
          <Text type='tertiary'>{t('管理下游平台及其 API 凭证')}</Text>
        </div>
        <Space>
          <Button onClick={loadPlatforms}>{t('刷新')}</Button>
          <Button theme='solid' type='primary' onClick={openCreate}>
            {t('创建平台')}
          </Button>
        </Space>
      </div>

      <Table
        rowKey='id'
        columns={columns}
        dataSource={platforms}
        loading={loading}
        pagination={false}
      />

      <Modal
        title={editingPlatform ? t('编辑平台') : t('创建平台')}
        visible={modalVisible}
        footer={null}
        onCancel={() => setModalVisible(false)}
      >
        <Form
          initValues={
            editingPlatform || {
              platform_id: '',
              name: '',
              status: 1,
              shadow_user_id: '',
            }
          }
          onSubmit={submitPlatform}
        >
          {!editingPlatform && (
            <Form.Input
              field='platform_id'
              label={t('平台 ID')}
              placeholder='partner_a'
              rules={[{ required: true, message: t('请输入平台 ID') }]}
            />
          )}
          <Form.Input field='name' label={t('平台名称')} />
          <Form.Select field='status' label={t('状态')}>
            <Form.Select.Option value={1}>{t('启用')}</Form.Select.Option>
            <Form.Select.Option value={2}>{t('禁用')}</Form.Select.Option>
          </Form.Select>
          <Form.Input
            field='shadow_user_id'
            label={t('影子用户 ID')}
            placeholder={t('留空则自动创建影子用户')}
          />
          <div className='flex justify-end gap-2'>
            <Button onClick={() => setModalVisible(false)}>{t('取消')}</Button>
            <Button htmlType='submit' theme='solid' type='primary'>
              {t('保存')}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        title={t('平台凭证已创建')}
        visible={skModalVisible}
        onCancel={() => setSkModalVisible(false)}
        footer={
          <Button theme='solid' onClick={() => setSkModalVisible(false)}>
            {t('关闭')}
          </Button>
        }
      >
        <Text type='warning'>
          {t('明文 platform_sk 仅本次返回，请立即保存。')}
        </Text>
        <div className='mt-4 space-y-3'>
          <div>
            <Text strong>platform_id</Text>
            <div className='mt-1 break-all'>{createdCredential?.platform_id}</div>
          </div>
          <div>
            <Text strong>platform_sk</Text>
            <div className='mt-1 break-all'>{createdCredential?.platform_sk}</div>
          </div>
          <Button
            onClick={async () => {
              const text = `X-Platform-Id: ${createdCredential?.platform_id}\nX-Platform-Sk: ${createdCredential?.platform_sk}`;
              if (await copy(text)) showSuccess(t('复制成功'));
            }}
          >
            {t('复制凭证')}
          </Button>
        </div>
      </Modal>
    </div>
  );
};

export default Platform;
