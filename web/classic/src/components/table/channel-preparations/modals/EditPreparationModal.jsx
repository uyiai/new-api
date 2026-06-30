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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Button,
  Input,
  InputNumber,
  Modal,
  Select,
  TextArea,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { CHANNEL_OPTIONS } from '../../../../constants/channel.constants';
import {
  getChannelModels,
  loadChannelModels,
  showError,
} from '../../../../helpers';
import { appendKeyFragment } from '../keyFragment';

const DEFAULT_TYPE = 14;

const emptyForm = {
  type: DEFAULT_TYPE,
  name: '',
  key: '',
  base_url: '',
  models: '',
  group: 'default',
  groups: ['default'],
  balance: 0,
  priority: 0,
  weight: 0,
  tag: '',
  remark: '',
  note: '',
};

const getModelText = (type) => getChannelModels(type).join(',');

const normalizeGroups = (value) => {
  const rawGroups = Array.isArray(value) ? value : [value];
  const groups = rawGroups
    .map((item) => String(item || '').trim())
    .filter(Boolean);
  const uniqueGroups = Array.from(new Set(groups));
  return uniqueGroups.length > 0 ? uniqueGroups : ['default'];
};

const EditPreparationModal = ({
  visible,
  preparation,
  groupOptions,
  onCancel,
  onSubmit,
}) => {
  const { t } = useTranslation();
  const [form, setForm] = useState(emptyForm);
  const [submitting, setSubmitting] = useState(false);
  const isEdit = Boolean(preparation?.id);

  useEffect(() => {
    if (!visible) return;
    loadChannelModels().catch(() => {});
  }, [visible]);

  useEffect(() => {
    if (!visible) return;
    if (preparation) {
      setForm({
        ...emptyForm,
        ...preparation,
        key: '',
        base_url: preparation.base_url || '',
        tag: preparation.tag || '',
        remark: preparation.remark || '',
        note: preparation.note || '',
        priority: preparation.priority ?? 0,
        weight: preparation.weight ?? 0,
        group: preparation.group || 'default',
        groups: [preparation.group || 'default'],
      });
    } else {
      setForm({ ...emptyForm });
    }
  }, [visible, preparation]);

  const typeOptions = useMemo(
    () =>
      CHANNEL_OPTIONS.map((option) => ({
        label: option.label,
        value: option.value,
      })),
    [],
  );

  const update = (key, value) => setForm((prev) => ({ ...prev, [key]: value }));

  const handleTypeChange = (value) => {
    const models = getModelText(value);
    setForm((prev) => ({
      ...prev,
      type: value,
      models: prev.models || models,
    }));
  };

  const handleSubmit = async () => {
    if (!form.name.trim()) {
      showError(t('名称不能为空'));
      return;
    }
    if (!isEdit && !form.key.trim()) {
      showError(t('Key 不能为空'));
      return;
    }
    setSubmitting(true);
    try {
      const groups = normalizeGroups(isEdit ? form.group : form.groups);
      const payload = {
        ...form,
        group: groups[0],
        groups: isEdit ? undefined : groups,
        name: appendKeyFragment(form.name.trim(), form.key),
        id: preparation?.id,
        type: Number(form.type),
        balance: Number(form.balance) || 0,
        priority: Number(form.priority) || 0,
        weight: Number(form.weight) || 0,
        base_url: form.base_url ? form.base_url : undefined,
        tag: form.tag ? form.tag : undefined,
        remark: form.remark ? form.remark : undefined,
      };
      await onSubmit(payload);
    } catch (error) {
      showError(error.message || t('保存失败'));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title={isEdit ? t('编辑候选渠道') : t('添加候选渠道')}
      visible={visible}
      onCancel={onCancel}
      footer={
        <div className='flex justify-end gap-2'>
          <Button onClick={onCancel}>{t('取消')}</Button>
          <Button type='primary' loading={submitting} onClick={handleSubmit}>
            {t('保存')}
          </Button>
        </div>
      }
      style={{ width: 720 }}
    >
      <div className='grid grid-cols-1 md:grid-cols-2 gap-3'>
        <div>
          <div className='mb-1 font-semibold'>{t('渠道类型')}</div>
          <Select
            value={form.type}
            optionList={typeOptions}
            onChange={handleTypeChange}
            style={{ width: '100%' }}
          />
        </div>
        <div>
          <div className='mb-1 font-semibold'>{t('名称')}</div>
          <Input
            value={form.name}
            onChange={(value) => update('name', value)}
          />
        </div>
        <div className='md:col-span-2'>
          <div className='mb-1 font-semibold'>Key</div>
          <TextArea
            value={form.key}
            onChange={(value) => update('key', value)}
            placeholder={isEdit ? t('留空则保留原 Key') : ''}
            rows={3}
          />
        </div>
        <div>
          <div className='mb-1 font-semibold'>Base URL</div>
          <Input
            value={form.base_url}
            onChange={(value) => update('base_url', value)}
          />
        </div>
        <div>
          <div className='mb-1 font-semibold'>{t('分组')}</div>
          <Select
            value={isEdit ? form.group : form.groups}
            optionList={groupOptions || []}
            multiple={!isEdit}
            allowCreate
            filter
            showClear={!isEdit}
            placeholder={t('请选择分组')}
            onChange={(value) =>
              isEdit
                ? update('group', value || 'default')
                : update('groups', normalizeGroups(value))
            }
            style={{ width: '100%' }}
          />
        </div>
        <div className='md:col-span-2'>
          <div className='mb-1 font-semibold'>{t('模型')}</div>
          <TextArea
            value={form.models}
            onChange={(value) => update('models', value)}
            rows={2}
            placeholder={
              getModelText(form.type) || t('不填则使用 Claude 默认模型')
            }
          />
        </div>
        <div>
          <div className='mb-1 font-semibold'>{t('余额')}</div>
          <InputNumber
            value={form.balance}
            onChange={(value) => update('balance', value ?? 0)}
            style={{ width: '100%' }}
          />
        </div>
        <div>
          <div className='mb-1 font-semibold'>{t('优先级')}</div>
          <InputNumber
            value={form.priority}
            onChange={(value) => update('priority', value ?? 0)}
            style={{ width: '100%' }}
          />
        </div>
        <div>
          <div className='mb-1 font-semibold'>{t('权重')}</div>
          <InputNumber
            value={form.weight}
            min={0}
            onChange={(value) => update('weight', value ?? 0)}
            style={{ width: '100%' }}
          />
        </div>
        <div>
          <div className='mb-1 font-semibold'>Tag</div>
          <Input value={form.tag} onChange={(value) => update('tag', value)} />
        </div>
        <div className='md:col-span-2'>
          <div className='mb-1 font-semibold'>{t('备注')}</div>
          <TextArea
            value={form.note || form.remark}
            onChange={(value) => update('note', value)}
            rows={2}
          />
        </div>
      </div>
    </Modal>
  );
};

export default EditPreparationModal;
