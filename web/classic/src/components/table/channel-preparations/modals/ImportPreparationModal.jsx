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
import {
  Banner,
  Button,
  Input,
  InputNumber,
  Modal,
  Select,
  Table,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { showError } from '../../../../helpers';
import {
  ANTHROPIC_IMPORT_PROFILE_OFFICIAL,
  getAnthropicImportFormat,
  getAnthropicImportPlaceholder,
  getImportCredentialPreview,
  getImportPreviewName,
  parseAnthropicImportText,
  useAnthropicImportProfiles,
} from '../../../../hooks/channels/useAnthropicImportProfiles';

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

const ImportPreparationModal = ({
  visible,
  groupOptions = [],
  onCancel,
  onSubmit,
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
  const [importing, setImporting] = useState(false);
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
    !importing &&
    Boolean(selectedProfile) &&
    parsed.entries.length > 0 &&
    parsed.errors.length === 0 &&
    (!officialNeedsSuffix || nameSuffix.trim().length > 0);

  const reset = useCallback(() => {
    resetDrafts();
    setNameSuffix('');
    setModelDrafts({});
    setGroups([DEFAULT_GROUP]);
    setTag('');
    setPriority(0);
    setWeight(0);
    setResults([]);
    setImporting(false);
  }, [resetDrafts]);

  const handleCancel = () => {
    if (importing) return;
    reset();
    onCancel();
  };

  const handleImport = async () => {
    if (!canImport) return;
    setImporting(true);
    setResults([]);
    try {
      const finalModels =
        models.trim() || (selectedProfile.default_models || []).join(',');
      const importResults = await onSubmit({
        profile_id: selectedProfileID,
        target: 'preparation',
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
      setResults(importResults);
    } catch (error) {
      showError(error.message || t('导入失败'));
    } finally {
      setImporting(false);
    }
  };

  const failedResults = results.filter((item) => !item.ok);
  const createdCount = results.reduce(
    (sum, item) => sum + (item.ok ? Number(item.created_count || 0) : 0),
    0,
  );
  const columns = [
    {
      title: '#',
      dataIndex: 'index',
      width: 50,
      render: (value) => value + 1,
    },
    { title: t('名称'), dataIndex: 'name', key: 'name' },
    { title: t('额度'), dataIndex: 'balance', key: 'balance', width: 90 },
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

  return (
    <Modal
      title={t('导入候选渠道')}
      visible={visible}
      onCancel={handleCancel}
      maskClosable={!importing}
      closable={!importing}
      footer={
        <div className='flex justify-end gap-2'>
          <Button onClick={handleCancel} disabled={importing}>
            {t('关闭')}
          </Button>
          <Button
            type='primary'
            loading={importing}
            disabled={!canImport}
            onClick={handleImport}
          >
            {t('导入到备货池（{{count}} 个）', { count: totalImportCount })}
          </Button>
        </div>
      }
      style={{ width: 900 }}
    >
      <div className='space-y-4'>
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
            disabled={importing}
            style={{ width: '100%' }}
          />
          <Typography.Text type='secondary' size='small'>
            {t('当前格式：{{format}}', {
              format: getAnthropicImportFormat(selectedProfile),
            })}
          </Typography.Text>
        </div>

        {officialNeedsSuffix ? (
          <div>
            <div className='mb-1 font-semibold'>{t('名称后缀')}</div>
            <Input
              value={nameSuffix}
              onChange={setNameSuffix}
              disabled={importing}
            />
          </div>
        ) : null}

        <TextArea
          value={inputText}
          onChange={setInputText}
          rows={8}
          disabled={importing}
          placeholder={getAnthropicImportPlaceholder(selectedProfile)}
          style={{ fontFamily: 'monospace' }}
        />

        <div className='grid grid-cols-1 gap-3 md:grid-cols-4'>
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
              disabled={importing}
              style={{ width: '100%' }}
            />
          </div>
          <div>
            <div className='mb-1 font-semibold'>{t('标签')}</div>
            <Input value={tag} onChange={setTag} disabled={importing} />
          </div>
          <div>
            <div className='mb-1 font-semibold'>{t('优先级')}</div>
            <InputNumber
              value={priority}
              onChange={(value) => setPriority(value ?? 0)}
              disabled={importing}
              style={{ width: '100%' }}
            />
          </div>
          <div>
            <div className='mb-1 font-semibold'>{t('权重')}</div>
            <InputNumber
              value={weight}
              min={0}
              onChange={(value) => setWeight(value ?? 0)}
              disabled={importing}
              style={{ width: '100%' }}
            />
          </div>
        </div>

        <div>
          <div className='mb-1 font-semibold'>{t('模型')}</div>
          <TextArea
            value={models}
            onChange={setModels}
            rows={3}
            disabled={importing}
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
            <Typography.Text type='secondary'>
              {t(
                '共 {{rows}} 行，将在 {{groups}} 个分组中创建 {{count}} 个候选渠道',
                {
                  rows: previewRows.length,
                  groups: selectedGroups.length,
                  count: totalImportCount,
                },
              )}
            </Typography.Text>
            <Table
              columns={columns}
              dataSource={previewRows}
              pagination={false}
              size='small'
              rowKey='index'
            />
          </>
        ) : null}

        {results.length > 0 ? (
          <Banner
            type={failedResults.length === 0 ? 'success' : 'warning'}
            description={
              failedResults.length === 0
                ? t('成功创建 {{count}} 个候选渠道', {
                    count: createdCount,
                  })
                : t(
                    '成功创建 {{success}} 个候选渠道，失败 {{fail}} 行：{{details}}',
                    {
                      success: createdCount,
                      fail: failedResults.length,
                      details: failedResults
                        .slice(0, 5)
                        .map((item) => `#${item.index + 1} ${item.error}`)
                        .join('；'),
                    },
                  )
            }
          />
        ) : null}
      </div>
    </Modal>
  );
};

export default ImportPreparationModal;
