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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Banner,
  Button,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { IconUpload } from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../../../helpers';
import {
  ANTHROPIC_IMPORT_PROFILE_OFFICIAL,
  getAnthropicImportFormat,
  getAnthropicImportPlaceholder,
  getImportCredentialPreview,
  getImportPreviewName,
  parseAnthropicImportText,
  useAnthropicImportProfiles,
} from '../../../../hooks/channels/useAnthropicImportProfiles';

const { Text } = Typography;
const DEFAULT_GROUP = 'default';

const normalizeGroups = (value) => {
  const values = Array.isArray(value) ? value : [value];
  const groups = [
    ...new Set(values.map((item) => String(item || '').trim())),
  ].filter(Boolean);
  return groups.length > 0 ? groups : [DEFAULT_GROUP];
};

const generateTimestamp = () => {
  const now = new Date();
  const pad = (value) => String(value).padStart(2, '0');
  return `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}${pad(now.getHours())}${pad(now.getMinutes())}`;
};

const BatchImportModal = ({
  visible,
  groupOptions = [],
  onCancel,
  onSuccess,
}) => {
  const { t } = useTranslation();
  const {
    profiles,
    selectedProfile,
    selectedProfileID,
    setSelectedProfileID,
    inputText,
    setInputText,
    resetDrafts,
    loading: profilesLoading,
    loadError,
  } = useAnthropicImportProfiles(visible);
  const [nameSuffix, setNameSuffix] = useState('');
  const [modelDrafts, setModelDrafts] = useState({});
  const [groups, setGroups] = useState([DEFAULT_GROUP]);
  const [tag, setTag] = useState('');
  const [priority, setPriority] = useState(0);
  const [weight, setWeight] = useState(0);
  const [importState, setImportState] = useState('idle');
  const [results, setResults] = useState([]);
  const [timestamp, setTimestamp] = useState(generateTimestamp());

  useEffect(() => {
    if (visible) setTimestamp(generateTimestamp());
  }, [visible]);

  const models = modelDrafts[selectedProfileID] || '';
  const setModels = useCallback(
    (value) => {
      setModelDrafts((current) => ({
        ...current,
        [selectedProfileID]: value,
      }));
    },
    [selectedProfileID],
  );
  const selectedGroups = useMemo(() => normalizeGroups(groups), [groups]);
  const parsed = useMemo(
    () => parseAnthropicImportText(inputText, selectedProfile),
    [inputText, selectedProfile],
  );
  const previewRows = useMemo(
    () =>
      parsed.entries.map((entry, index) => ({
        ...entry,
        index,
        name: getImportPreviewName(
          entry,
          selectedProfile,
          nameSuffix.trim(),
          timestamp,
        ),
        credentialPreview: getImportCredentialPreview(entry, selectedProfile),
      })),
    [parsed.entries, selectedProfile, nameSuffix, timestamp],
  );
  const totalImportCount = parsed.entries.length * selectedGroups.length;
  const officialNeedsSuffix =
    selectedProfileID === ANTHROPIC_IMPORT_PROFILE_OFFICIAL;
  const canImport =
    importState === 'idle' &&
    Boolean(selectedProfile) &&
    parsed.entries.length > 0 &&
    parsed.errors.length === 0 &&
    (!officialNeedsSuffix || nameSuffix.trim().length > 0);

  const resetState = useCallback(() => {
    resetDrafts();
    setNameSuffix('');
    setModelDrafts({});
    setGroups([DEFAULT_GROUP]);
    setTag('');
    setPriority(0);
    setWeight(0);
    setImportState('idle');
    setResults([]);
  }, [resetDrafts]);

  const handleCancel = useCallback(() => {
    if (importState === 'importing') return;
    resetState();
    onCancel();
  }, [importState, onCancel, resetState]);

  const handleImport = useCallback(async () => {
    if (!canImport) return;
    setImportState('importing');
    setResults([]);
    try {
      const finalModels =
        models.trim() || (selectedProfile.default_models || []).join(',');
      const response = await API.post('/api/channel/import', {
        profile_id: selectedProfileID,
        target: 'channel',
        rows: parsed.entries.map(({ balance, credentials }) => ({
          balance,
          credentials,
        })),
        groups: selectedGroups,
        tag: tag.trim(),
        priority: Number(priority) || 0,
        weight: Number(weight) || 0,
        models: finalModels,
        name_suffix: nameSuffix.trim(),
      });
      if (!response.data?.success) {
        throw new Error(response.data?.message || t('导入失败'));
      }
      const importResults = response.data.data?.results || [];
      setResults(importResults);
      setImportState('done');
      const createdCount = importResults.reduce(
        (sum, item) => sum + (item.ok ? Number(item.created_count || 0) : 0),
        0,
      );
      const failedCount = importResults.filter((item) => !item.ok).length;
      if (failedCount === 0) {
        showSuccess(t('成功导入 {{count}} 个渠道', { count: createdCount }));
      } else {
        showError(
          t('导入完成：成功 {{success}} 个，失败 {{fail}} 行', {
            success: createdCount,
            fail: failedCount,
          }),
        );
      }
      onSuccess?.();
    } catch (error) {
      setImportState('idle');
      showError(error.message || t('导入失败'));
    }
  }, [
    canImport,
    models,
    nameSuffix,
    onSuccess,
    parsed.entries,
    priority,
    selectedGroups,
    selectedProfile,
    selectedProfileID,
    t,
    tag,
    weight,
  ]);

  const columns = [
    {
      title: '#',
      dataIndex: 'index',
      width: 50,
      render: (value) => value + 1,
    },
    {
      title: t('渠道名称'),
      dataIndex: 'name',
      width: 220,
      render: (value) => <Text copyable>{value}</Text>,
    },
    {
      title: t('额度'),
      dataIndex: 'balance',
      width: 90,
    },
    {
      title: t('分组'),
      width: 150,
      render: () => selectedGroups.join(', '),
    },
    {
      title: t('凭证预览'),
      dataIndex: 'credentialPreview',
      width: 150,
    },
  ];
  if (importState === 'done') {
    columns.push({
      title: t('状态'),
      width: 90,
      render: (_, record) => {
        const result = results[record.index];
        return result?.ok ? (
          <Tag color='green'>{t('成功')}</Tag>
        ) : (
          <Tag
            color='red'
            style={{ cursor: 'pointer' }}
            onClick={() => showError(result?.error || t('导入失败'))}
          >
            {t('失败')}
          </Tag>
        );
      },
    });
  }

  return (
    <Modal
      title={
        <span>
          <IconUpload style={{ marginRight: 8 }} />
          {t('批量导入 Claude 渠道')}
        </span>
      }
      visible={visible}
      onCancel={handleCancel}
      maskClosable={importState !== 'importing'}
      closable={importState !== 'importing'}
      width={760}
      footer={
        <Space>
          <Button onClick={handleCancel} disabled={importState === 'importing'}>
            {importState === 'done' ? t('关闭') : t('取消')}
          </Button>
          {importState !== 'done' && (
            <Button
              theme='solid'
              type='primary'
              onClick={handleImport}
              disabled={!canImport}
              loading={importState === 'importing'}
            >
              {t('导入（将创建 {{count}} 个渠道）', {
                count: totalImportCount,
              })}
            </Button>
          )}
        </Space>
      }
    >
      <div className='flex flex-col gap-4'>
        {loadError ? <Banner type='danger' description={loadError} /> : null}

        <div>
          <div className='mb-1 font-semibold'>{t('导入来源')}</div>
          <Select
            value={selectedProfileID}
            loading={profilesLoading}
            optionList={profiles.map((profile) => ({
              value: profile.id,
              label: t(profile.label),
            }))}
            onChange={setSelectedProfileID}
            disabled={importState !== 'idle'}
            style={{ width: '100%' }}
          />
          <div className='mt-1 text-xs text-gray-500'>
            {t('当前格式：{{format}}', {
              format: getAnthropicImportFormat(selectedProfile),
            })}
          </div>
        </div>

        {officialNeedsSuffix ? (
          <div>
            <div className='mb-1 font-semibold'>{t('名称标签')}</div>
            <Input
              value={nameSuffix}
              onChange={setNameSuffix}
              disabled={importState !== 'idle'}
              placeholder={t('例如：liz')}
            />
          </div>
        ) : null}

        <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
          <div>
            <div className='mb-1 font-semibold'>{t('分组')}</div>
            <Select
              value={groups}
              optionList={groupOptions || []}
              multiple
              allowCreate
              filter
              showClear
              onChange={(value) => setGroups(normalizeGroups(value))}
              disabled={importState !== 'idle'}
              style={{ width: '100%' }}
            />
          </div>
          <div>
            <div className='mb-1 font-semibold'>{t('标签')}</div>
            <Input
              value={tag}
              onChange={setTag}
              disabled={importState !== 'idle'}
              placeholder={t('可选，整批渠道使用同一标签')}
            />
          </div>
          <div>
            <div className='mb-1 font-semibold'>{t('优先级')}</div>
            <InputNumber
              value={priority}
              onChange={(value) => setPriority(value ?? 0)}
              disabled={importState !== 'idle'}
              style={{ width: '100%' }}
            />
          </div>
          <div>
            <div className='mb-1 font-semibold'>{t('权重')}</div>
            <InputNumber
              value={weight}
              min={0}
              onChange={(value) => setWeight(value ?? 0)}
              disabled={importState !== 'idle'}
              style={{ width: '100%' }}
            />
          </div>
        </div>

        <div>
          <div className='mb-1 font-semibold'>{t('导入数据')}</div>
          <TextArea
            value={inputText}
            onChange={setInputText}
            disabled={importState !== 'idle'}
            placeholder={getAnthropicImportPlaceholder(selectedProfile)}
            autosize={{ minRows: 5, maxRows: 10 }}
            style={{ fontFamily: 'monospace', fontSize: 12 }}
          />
        </div>

        <div>
          <div className='mb-1 font-semibold'>{t('模型')}</div>
          <TextArea
            value={models}
            onChange={setModels}
            disabled={importState !== 'idle'}
            rows={3}
            placeholder={(selectedProfile?.default_models || []).join(',')}
          />
        </div>

        {parsed.errors.length > 0 ? (
          <Banner
            type='danger'
            description={parsed.errors
              .map((error) =>
                error.code === 'balance'
                  ? t('第 {{line}} 行额度无效', { line: error.line })
                  : error.code === 'account_id'
                    ? t('第 {{line}} 行 Cloudflare Account ID 无效', {
                        line: error.line,
                      })
                    : t('第 {{line}} 行格式错误，应为 {{format}}', {
                        line: error.line,
                        format: getAnthropicImportFormat(selectedProfile),
                      }),
              )
              .join('；')}
          />
        ) : null}

        {previewRows.length > 0 ? (
          <>
            <Text type='tertiary'>
              {t(
                '共 {{rows}} 行，将在 {{groups}} 个分组中创建 {{count}} 个渠道',
                {
                  rows: previewRows.length,
                  groups: selectedGroups.length,
                  count: totalImportCount,
                },
              )}
            </Text>
            <Table
              columns={columns}
              dataSource={previewRows}
              pagination={false}
              size='small'
              rowKey='index'
            />
          </>
        ) : null}
      </div>
    </Modal>
  );
};

export default BatchImportModal;
