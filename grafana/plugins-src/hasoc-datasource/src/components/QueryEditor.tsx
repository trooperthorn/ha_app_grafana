import React, { ChangeEvent } from 'react';
import { InlineField, Input, Select, Stack } from '@grafana/ui';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { DataSource } from '../datasource';
import { DEFAULT_QUERY, HASOCDataSourceOptions, HASOCQuery, HASOCSeries } from '../types';

type Props = QueryEditorProps<DataSource, HASOCQuery, HASOCDataSourceOptions>;

const SERIES: Array<SelectableValue<HASOCSeries>> = [
  { label: 'Posture', value: 'posture', description: 'The whole-install security score, grade, and its factor breakdown' },
  { label: 'Risk', value: 'risk', description: 'One row per user: score, band, and top risk factor' },
  { label: 'Audit', value: 'audit', description: 'Audit log events within the dashboard time range' },
];

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  const series = query.series ?? DEFAULT_QUERY.series;

  const onSeriesChange = (selected: SelectableValue<HASOCSeries>) => {
    onChange({ ...query, series: selected.value! });
    onRunQuery();
  };

  const onAuditLimitChange = (event: ChangeEvent<HTMLInputElement>) => {
    const parsed = parseInt(event.target.value, 10);
    onChange({ ...query, auditLimit: Number.isNaN(parsed) ? undefined : parsed });
  };

  return (
    <Stack gap={0}>
      <InlineField label="Series" labelWidth={16} tooltip="Which table to return">
        <Select options={SERIES} value={series} onChange={onSeriesChange} width={24} />
      </InlineField>
      {series === 'audit' && (
        <InlineField label="Limit" labelWidth={10} tooltip="Maximum audit events to return (HA SOC's own default is 200)">
          <Input
            id="query-editor-audit-limit"
            onChange={onAuditLimitChange}
            onBlur={onRunQuery}
            value={query.auditLimit ?? DEFAULT_QUERY.auditLimit}
            type="number"
            width={12}
          />
        </InlineField>
      )}
    </Stack>
  );
}
