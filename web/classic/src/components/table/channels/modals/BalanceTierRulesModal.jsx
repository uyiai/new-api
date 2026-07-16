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
*/

import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Collapsible,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconArrowDown,
  IconArrowUp,
  IconDelete,
  IconPlus,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../../../helpers';
import { CHANNEL_OPTIONS } from '../../../../constants';

const { Text } = Typography;

const ANTHROPIC_CLAUDE_CHANNEL_TYPE = 14;
const DEFAULT_TIER_AMOUNTS = [5, 10];

const STRATEGIES = [
  {
    value: 'fixed_priority',
    label: '固定优先级',
    description: '命中后直接设置为指定优先级。',
  },
  {
    value: 'lower_effective_balance_higher_priority',
    label: '指标越低，优先级越高',
    description: '在金额或比例区间内，从最高优先级线性降到最低优先级。',
  },
  {
    value: 'higher_effective_balance_higher_priority',
    label: '指标越高，优先级越高',
    description: '在金额或比例区间内，从最低优先级线性升到最高优先级。',
  },
  {
    value: 'keep_priority',
    label: '保留原优先级',
    description: '规则命中但不修改优先级，可用于排除特定渠道。',
  },
];

const METRICS = [
  {
    value: 'effective_balance',
    label: '按剩余额度金额',
    description: '适合固定档位，例如 5 刀档内按还剩 0-5 刀计算。',
  },
  {
    value: 'remaining_ratio',
    label: '按剩余比例',
    description: '适合区间档位，例如 5-100 刀档统一按剩余百分比计算。',
  },
];

const emptyRule = (tierAmount = null) => {
  const amount = tierAmount === undefined || tierAmount === null || tierAmount === ''
    ? null
    : Number(tierAmount);

  return {
    id: `rule-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    name: amount === null ? 'Anthropic Claude 档位规则' : `Anthropic Claude ${amount}刀档位`,
    enabled: true,
    channel_types: [ANTHROPIC_CLAUDE_CHANNEL_TYPE],
    groups: [],
    tags: [],
    balance_min: amount,
    balance_max: amount,
    metric: amount === null ? 'remaining_ratio' : 'effective_balance',
    effective_balance_min: amount === null ? null : 0,
    effective_balance_max: amount,
    remaining_ratio_min: 0,
    remaining_ratio_max: 100,
    strategy: 'fixed_priority',
    fixed_priority: 0,
    min_priority: 0,
    max_priority: 100,
  };
};

const numberValue = (value) =>
  value === undefined || value === null || value === '' ? null : Number(value);

const getExactTierAmount = (rule) => {
  if (
    rule.balance_min === undefined ||
    rule.balance_min === null ||
    rule.balance_max === undefined ||
    rule.balance_max === null
  ) {
    return null;
  }
  return Number(rule.balance_min) === Number(rule.balance_max)
    ? Number(rule.balance_min)
    : null;
};

const isAutoTierRuleName = (name) =>
  !name ||
  /^Anthropic Claude( .+刀)?档位规则$/.test(name) ||
  /^Anthropic Claude .+刀档位$/.test(name) ||
  /^Anthropic Claude .+刀区间$/.test(name) ||
  /^Anthropic Claude .+刀以上$/.test(name) ||
  /^Anthropic Claude .+刀以内$/.test(name);

const getTierDisplayName = (minimum, maximum) => {
  const hasMin = minimum !== undefined && minimum !== null && minimum !== '';
  const hasMax = maximum !== undefined && maximum !== null && maximum !== '';
  if (!hasMin && !hasMax) return 'Anthropic Claude 档位规则';
  if (hasMin && hasMax && Number(minimum) === Number(maximum)) {
    return `Anthropic Claude ${minimum}刀档位`;
  }
  if (hasMin && hasMax) return `Anthropic Claude ${minimum}-${maximum}刀区间`;
  if (hasMin) return `Anthropic Claude ${minimum}刀以上`;
  return `Anthropic Claude ${maximum}刀以内`;
};

const getTierTagText = (rule) => {
  const exactTierAmount = getExactTierAmount(rule);
  if (exactTierAmount !== null) return `${exactTierAmount} 刀档`;
  const hasMin = rule.balance_min !== undefined && rule.balance_min !== null;
  const hasMax = rule.balance_max !== undefined && rule.balance_max !== null;
  if (hasMin && hasMax) return `${rule.balance_min}-${rule.balance_max} 刀区间`;
  if (hasMin) return `≥ ${rule.balance_min} 刀`;
  if (hasMax) return `≤ ${rule.balance_max} 刀`;
  return '自定义档位';
};

const getTierMode = (rule) => (getExactTierAmount(rule) !== null ? 'fixed' : 'range');

const getRuleMetric = (rule) => rule.metric || 'effective_balance';

const getMetricConfig = (metric) => METRICS.find((item) => item.value === metric) || METRICS[0];

const BalanceTierRulesModal = ({
  visible,
  onCancel,
  groupOptions = [],
  onApplied,
}) => {
  const [setting, setSetting] = useState({ enabled: false, rules: [] });
  const [loading, setLoading] = useState(false);
  const [action, setAction] = useState('');
  const [result, setResult] = useState(null);
  const [advancedOpen, setAdvancedOpen] = useState(false);

  const typeOptions = useMemo(
    () =>
      CHANNEL_OPTIONS.map((item) => ({
        label: item.label,
        value: Number(item.value),
      })).filter((item) => Number.isFinite(item.value)),
    [],
  );

  const busy = loading || Boolean(action);

  const loadSetting = async () => {
    setLoading(true);
    setSetting({ enabled: false, rules: [] });
    setResult(null);
    try {
      const res = await API.get('/api/channel/balance-tier/setting');
      if (res?.data?.success) {
        setSetting({
          enabled: Boolean(res.data.data?.enabled),
          rules: res.data.data?.rules || [],
        });
        setResult(null);
      } else {
        showError(res?.data?.message || '加载余额档位规则失败');
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (visible) loadSetting();
  }, [visible]);

  const updateRule = (index, patch) => {
    setSetting((current) => ({
      ...current,
      rules: current.rules.map((rule, i) =>
        i === index ? { ...rule, ...patch } : rule,
      ),
    }));
  };

  const moveRule = (index, offset) => {
    setSetting((current) => {
      const target = index + offset;
      if (target < 0 || target >= current.rules.length) return current;
      const rules = [...current.rules];
      [rules[index], rules[target]] = [rules[target], rules[index]];
      return { ...current, rules };
    });
  };

  const addRule = (tierAmount = null) => {
    setSetting((current) => ({
      ...current,
      rules: [...current.rules, emptyRule(tierAmount)],
    }));
  };

  const updateTierAmount = (index, value) => {
    const amount = numberValue(value);
    setSetting((current) => ({
      ...current,
      rules: current.rules.map((rule, i) => {
        if (i !== index) return rule;
        const patch = {
          balance_min: amount,
          balance_max: amount,
        };
        if (isAutoTierRuleName(rule.name)) {
          patch.name = getTierDisplayName(amount, amount);
        }
        if (rule.effective_balance_min === null || rule.effective_balance_min === undefined) {
          patch.effective_balance_min = amount === null ? null : 0;
        }
        if (rule.effective_balance_max === null || rule.effective_balance_max === undefined || Number(rule.effective_balance_max) === Number(rule.balance_max)) {
          patch.effective_balance_max = amount;
        }
        return { ...rule, ...patch };
      }),
    }));
  };

  const updateTierMode = (index, mode) => {
    setSetting((current) => ({
      ...current,
      rules: current.rules.map((rule, i) => {
        if (i !== index) return rule;
        if (mode === 'fixed') {
          const amount = rule.balance_min ?? rule.balance_max ?? null;
          const patch = {
            balance_min: amount,
            balance_max: amount,
            metric: 'effective_balance',
          };
          if (isAutoTierRuleName(rule.name)) {
            patch.name = getTierDisplayName(amount, amount);
          }
          return { ...rule, ...patch };
        }
        const exactTierAmount = getExactTierAmount(rule);
        const patch = exactTierAmount === null
          ? { metric: 'remaining_ratio' }
          : { balance_min: 0, balance_max: exactTierAmount, metric: 'remaining_ratio' };
        if (rule.remaining_ratio_min === null || rule.remaining_ratio_min === undefined) {
          patch.remaining_ratio_min = 0;
        }
        if (rule.remaining_ratio_max === null || rule.remaining_ratio_max === undefined) {
          patch.remaining_ratio_max = 100;
        }
        if (isAutoTierRuleName(rule.name)) {
          patch.name = getTierDisplayName(
            patch.balance_min ?? rule.balance_min,
            patch.balance_max ?? rule.balance_max,
          );
        }
        return { ...rule, ...patch };
      }),
    }));
  };

  const updateTierRange = (index, field, value) => {
    const amount = numberValue(value);
    setSetting((current) => ({
      ...current,
      rules: current.rules.map((rule, i) => {
        if (i !== index) return rule;
        const patch = { [field]: amount };
        const nextMin = field === 'balance_min' ? amount : rule.balance_min;
        const nextMax = field === 'balance_max' ? amount : rule.balance_max;
        if (isAutoTierRuleName(rule.name)) {
          patch.name = getTierDisplayName(nextMin, nextMax);
        }
        return { ...rule, ...patch };
      }),
    }));
  };

  const updateRuleMetric = (index, metric) => {
    setSetting((current) => ({
      ...current,
      rules: current.rules.map((rule, i) => {
        if (i !== index) return rule;
        const patch = { metric };
        if (metric === 'remaining_ratio') {
          if (rule.remaining_ratio_min === null || rule.remaining_ratio_min === undefined) {
            patch.remaining_ratio_min = 0;
          }
          if (rule.remaining_ratio_max === null || rule.remaining_ratio_max === undefined) {
            patch.remaining_ratio_max = 100;
          }
        } else {
          if (rule.effective_balance_min === null || rule.effective_balance_min === undefined) {
            patch.effective_balance_min = 0;
          }
          if (rule.effective_balance_max === null || rule.effective_balance_max === undefined) {
            patch.effective_balance_max = rule.balance_max ?? rule.balance_min ?? null;
          }
        }
        return { ...rule, ...patch };
      }),
    }));
  };

  const saveSetting = async (showMessage = true) => {
    const res = await API.put('/api/channel/balance-tier/setting', setting);
    if (!res?.data?.success) {
      showError(res?.data?.message || '保存失败');
      return false;
    }
    setSetting(res.data.data);
    if (showMessage) showSuccess('余额档位规则已保存');
    return true;
  };

  const runAction = async (kind) => {
    setAction(kind);
    try {
      if (kind === 'save') {
        await saveSetting();
        return;
      }
      if (kind === 'preview') {
        const res = await API.post('/api/channel/balance-tier/preview', setting);
        if (!res?.data?.success) {
          showError(res?.data?.message || '预览失败');
          return;
        }
        setResult(res.data.data);
        showSuccess('预览完成');
        return;
      }
      const res = await API.post('/api/channel/balance-tier/apply', setting);
      if (!res?.data?.success) {
        showError(res?.data?.message || '应用失败');
        return;
      }
      setResult(res.data.data);
      showSuccess(`已更新 ${res.data.data?.summary?.changed_channels || 0} 个渠道`);
      if (onApplied) onApplied();
    } catch (error) {
      showError(error.message);
    } finally {
      setAction('');
    }
  };

  const columns = [
    { title: 'ID', dataIndex: 'channel_id', width: 70 },
    { title: '渠道', dataIndex: 'channel_name', width: 150 },
    { title: '类型', dataIndex: 'channel_type', width: 70 },
    { title: '分组', dataIndex: 'group', width: 110 },
    { title: '标签', dataIndex: 'tag', width: 100 },
    {
      title: '固定余额',
      dataIndex: 'balance',
      width: 100,
      render: (value) => Number(value).toFixed(4),
    },
    {
      title: '已用额度($)',
      dataIndex: 'used_quota_usd',
      width: 110,
      render: (value) => Number(value).toFixed(4),
    },
    {
      title: '有效余额',
      dataIndex: 'effective_balance',
      width: 110,
      render: (value) => Number(value).toFixed(4),
    },
    {
      title: '剩余比例',
      dataIndex: 'remaining_ratio',
      width: 100,
      render: (value) => `${Number(value).toFixed(2)}%`,
    },
    { title: '原优先级', dataIndex: 'old_priority', width: 90 },
    {
      title: '新优先级',
      dataIndex: 'new_priority',
      width: 90,
      render: (value, record) => (
        <Text strong={record.changed} type={record.changed ? 'success' : undefined}>
          {value}
        </Text>
      ),
    },
    {
      title: '命中规则',
      dataIndex: 'matched_rule_name',
      width: 140,
      render: (value) => value || '-',
    },
    { title: '原因', dataIndex: 'reason', width: 210 },
  ];

  return (
    <Modal
      title='余额档位调度规则'
      visible={visible}
      onCancel={onCancel}
      footer={null}
      width={1180}
      bodyStyle={{ maxHeight: '78vh', overflowY: 'auto' }}
    >
      <Banner
        type='info'
        closeIcon={null}
        description='先按“档位”匹配渠道管理里的固定余额，可选固定档位（例如正好 5 刀）或区间档位（例如 5-10 刀）；再按有效余额 = 固定余额 - 已用额度计算优先级。新建规则默认只作用于 Anthropic Claude；有效余额小于 0 的渠道始终跳过。'
        style={{ marginBottom: 12 }}
      />

      <div className='flex items-center justify-between mb-3'>
        <Space>
          <Switch
            checked={setting.enabled}
            disabled={busy}
            onChange={(enabled) => setSetting({ ...setting, enabled })}
          />
          <Text strong>启用余额档位规则</Text>
          <Button size='small' type='tertiary' onClick={() => setAdvancedOpen(!advancedOpen)}>
            {advancedOpen ? '收起高级筛选' : '高级筛选（可选）'}
          </Button>
        </Space>
        <Space>
          {DEFAULT_TIER_AMOUNTS.map((amount) => (
            <Button
              key={amount}
              disabled={busy}
              onClick={() => addRule(amount)}
            >
              添加 {amount} 刀档
            </Button>
          ))}
          <Button
            icon={<IconPlus />}
            disabled={busy}
            onClick={() => addRule()}
          >
            添加区间档
          </Button>
        </Space>
      </div>

      {setting.rules.map((rule, index) => {
        const strategy = STRATEGIES.find((item) => item.value === rule.strategy);
        const dynamic = rule.strategy.includes('effective_balance_higher_priority');
        const exactTierAmount = getExactTierAmount(rule);
        const tierMode = getTierMode(rule);
        const tierTagText = getTierTagText(rule);
        const ruleMetric = getRuleMetric(rule);
        const metricConfig = getMetricConfig(ruleMetric);
        const metricMinField = ruleMetric === 'remaining_ratio' ? 'remaining_ratio_min' : 'effective_balance_min';
        const metricMaxField = ruleMetric === 'remaining_ratio' ? 'remaining_ratio_max' : 'effective_balance_max';
        const metricUnit = ruleMetric === 'remaining_ratio' ? '%' : '美元';
        const metricLabel = ruleMetric === 'remaining_ratio' ? '剩余比例' : '剩余额度';
        return (
          <Card
            key={rule.id || index}
            title={
              <Space>
                <Switch
                  size='small'
                  checked={rule.enabled}
                  disabled={busy}
                  onChange={(enabled) => updateRule(index, { enabled })}
                />
                <Text strong>规则 {index + 1}</Text>
                <Tag color={tierMode === 'fixed' ? 'blue' : 'teal'}>{tierTagText}</Tag>
                {!rule.enabled && <Tag color='grey'>已停用</Tag>}
              </Space>
            }
            headerExtraContent={
              <Space>
                <Button
                  size='small'
                  icon={<IconArrowUp />}
                  disabled={busy || index === 0}
                  onClick={() => moveRule(index, -1)}
                />
                <Button
                  size='small'
                  icon={<IconArrowDown />}
                  disabled={busy || index === setting.rules.length - 1}
                  onClick={() => moveRule(index, 1)}
                />
                <Button
                  size='small'
                  type='danger'
                  icon={<IconDelete />}
                  disabled={busy}
                  onClick={() =>
                    setSetting({
                      ...setting,
                      rules: setting.rules.filter((_, i) => i !== index),
                    })
                  }
                />
              </Space>
            }
            style={{ marginBottom: 12 }}
          >
            <div className='grid grid-cols-1 md:grid-cols-4 gap-3'>
              <div>
                <Text size='small'>名称</Text>
                <Input
                  value={rule.name}
                  onChange={(name) => updateRule(index, { name })}
                />
              </div>
              <div>
                <Text size='small'>默认渠道</Text>
                <div className='mt-1'>
                  <Tag color='indigo'>Anthropic Claude</Tag>
                </div>
                <Text type='tertiary' size='small'>
                  高级筛选里可调整
                </Text>
              </div>
              <div>
                <Text size='small'>档位类型</Text>
                <Select
                  style={{ width: '100%' }}
                  value={tierMode}
                  optionList={[
                    { label: '固定档位', value: 'fixed' },
                    { label: '区间档位', value: 'range' },
                  ]}
                  onChange={(mode) => updateTierMode(index, mode)}
                />
                <Text type='tertiary' size='small'>
                  固定匹配单个金额，区间匹配一段余额
                </Text>
              </div>
              {tierMode === 'fixed' ? (
                <div>
                  <Text size='small'>固定档位金额（美元）</Text>
                  <InputNumber
                    value={exactTierAmount}
                    placeholder='例如 5'
                    onChange={(value) => updateTierAmount(index, value)}
                    style={{ width: '100%' }}
                  />
                  <Text type='tertiary' size='small'>
                    例如 5 表示只匹配固定余额为 5 刀
                  </Text>
                </div>
              ) : (
                <>
                  <div>
                    <Text size='small'>档位余额下限</Text>
                    <InputNumber
                      value={rule.balance_min}
                      placeholder='例如 5'
                      onChange={(value) => updateTierRange(index, 'balance_min', value)}
                      style={{ width: '100%' }}
                    />
                  </div>
                  <div>
                    <Text size='small'>档位余额上限</Text>
                    <InputNumber
                      value={rule.balance_max}
                      placeholder='例如 10'
                      onChange={(value) => updateTierRange(index, 'balance_max', value)}
                      style={{ width: '100%' }}
                    />
                    <Text type='tertiary' size='small'>
                      例如 5 到 10 表示匹配 5-10 刀档
                    </Text>
                  </div>
                </>
              )}
              <div>
                <Text size='small'>计算指标</Text>
                <Select
                  style={{ width: '100%' }}
                  value={ruleMetric}
                  optionList={METRICS}
                  onChange={(metric) => updateRuleMetric(index, metric)}
                />
                <Text type='tertiary' size='small'>
                  {metricConfig.description}
                </Text>
              </div>
              {[
                [metricMinField, `${metricLabel}下限（${metricUnit}）`],
                [metricMaxField, `${metricLabel}上限（${metricUnit}）`],
              ].map(([field, label]) => (
                <div key={field}>
                  <Text size='small'>{label}</Text>
                  <InputNumber
                    value={rule[field]}
                    placeholder={ruleMetric === 'remaining_ratio' ? (field === metricMinField ? '例如 0' : '例如 100') : undefined}
                    onChange={(value) =>
                      updateRule(index, { [field]: numberValue(value) })
                    }
                    style={{ width: '100%' }}
                  />
                </div>
              ))}
              <div className='md:col-span-2'>
                <Text size='small'>优先级策略</Text>
                <Select
                  style={{ width: '100%' }}
                  value={rule.strategy}
                  optionList={STRATEGIES}
                  onChange={(strategy) => updateRule(index, { strategy })}
                />
                <Text type='tertiary' size='small'>
                  {strategy?.description}
                </Text>
              </div>
              {rule.strategy === 'fixed_priority' && (
                <div>
                  <Text size='small'>固定优先级</Text>
                  <InputNumber
                    value={rule.fixed_priority}
                    onChange={(value) =>
                      updateRule(index, { fixed_priority: numberValue(value) })
                    }
                    style={{ width: '100%' }}
                  />
                </div>
              )}
              {dynamic && (
                <>
                  <div>
                    <Text size='small'>最低优先级</Text>
                    <InputNumber
                      value={rule.min_priority}
                      onChange={(value) =>
                        updateRule(index, { min_priority: numberValue(value) })
                      }
                      style={{ width: '100%' }}
                    />
                  </div>
                  <div>
                    <Text size='small'>最高优先级</Text>
                    <InputNumber
                      value={rule.max_priority}
                      onChange={(value) =>
                        updateRule(index, { max_priority: numberValue(value) })
                      }
                      style={{ width: '100%' }}
                    />
                  </div>
                </>
              )}
              <div className='md:col-span-4'>
                <Collapsible isOpen={advancedOpen} keepDOM>
                  <div className='grid grid-cols-1 md:grid-cols-3 gap-3 rounded-lg bg-gray-50 p-3'>
                    <div>
                      <Text size='small'>渠道类型</Text>
                      <Select
                        multiple
                        filter
                        style={{ width: '100%' }}
                        value={rule.channel_types || []}
                        optionList={typeOptions}
                        onChange={(channel_types) => updateRule(index, { channel_types })}
                      />
                    </div>
                    <div>
                      <Text size='small'>分组（可选）</Text>
                      <Select
                        multiple
                        filter
                        style={{ width: '100%' }}
                        value={rule.groups || []}
                        optionList={groupOptions}
                        onChange={(groups) => updateRule(index, { groups })}
                      />
                    </div>
                    <div>
                      <Text size='small'>Tag（可选，可直接输入创建）</Text>
                      <Select
                        multiple
                        filter
                        allowCreate
                        style={{ width: '100%' }}
                        value={rule.tags || []}
                        optionList={(rule.tags || []).map((tag) => ({
                          label: tag,
                          value: tag,
                        }))}
                        onChange={(tags) => updateRule(index, { tags })}
                      />
                    </div>
                    <div className='md:col-span-3'>
                      <Text type='tertiary' size='small'>
                        规则 ID 自动生成：{rule.id}
                      </Text>
                    </div>
                  </div>
                </Collapsible>
              </div>
            </div>
          </Card>
        );
      })}

      {setting.rules.length === 0 && (
        <div className='text-center text-gray-500 py-8'>暂无规则，请先添加规则</div>
      )}

      <Space style={{ marginTop: 4, marginBottom: 16 }}>
        <Button
          type='primary'
          loading={action === 'save'}
          disabled={busy}
          onClick={() => runAction('save')}
        >
          保存
        </Button>
        <Button
          loading={action === 'preview'}
          disabled={busy}
          onClick={() => runAction('preview')}
        >
          预览
        </Button>
        <Button
          type='warning'
          theme='solid'
          loading={action === 'apply'}
          disabled={busy}
          onClick={() => runAction('apply')}
        >
          保存并应用
        </Button>
      </Space>

      {result && (
        <>
          <Card style={{ marginBottom: 12 }}>
            <Space wrap>
              <Tag>启用渠道 {result.summary?.enabled_channels || 0}</Tag>
              <Tag color='blue'>命中 {result.summary?.matched_channels || 0}</Tag>
              <Tag color='green'>变化 {result.summary?.changed_channels || 0}</Tag>
              <Tag color='grey'>跳过 {result.summary?.skipped_channels || 0}</Tag>
            </Space>
          </Card>
          <Table
            rowKey='channel_id'
            columns={columns}
            dataSource={result.details || []}
            pagination={{ pageSize: 20 }}
            scroll={{ x: 1450 }}
            size='small'
          />
        </>
      )}

      {loading && <div className='text-center py-4'>加载中...</div>}
    </Modal>
  );
};

export default BalanceTierRulesModal;
