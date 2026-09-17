import React from 'react';
import { InlineField, Select, Stack } from '@grafana/ui';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { DataSource } from '../datasource';
import { DEFAULT_QUERY, TechnitiumDataSourceOptions, TechnitiumQuery, TechnitiumSeries, TechnitiumStatType } from '../types';

type Props = QueryEditorProps<DataSource, TechnitiumQuery, TechnitiumDataSourceOptions>;

const STAT_TYPES: Array<SelectableValue<TechnitiumStatType>> = [
  { label: 'Last hour', value: 'LastHour' },
  { label: 'Last day', value: 'LastDay' },
  { label: 'Last week', value: 'LastWeek' },
  { label: 'Last month', value: 'LastMonth' },
  { label: 'Last year', value: 'LastYear' },
];

const SERIES: Array<SelectableValue<TechnitiumSeries>> = [
  { label: 'Query volume', value: 'volume', description: 'Time series of totals: queries, no error, blocked, ...' },
  { label: 'Top clients', value: 'topClients', description: 'Table of client name/hits' },
  { label: 'Top domains', value: 'topDomains', description: 'Table of domain name/hits' },
  { label: 'Top blocked domains', value: 'topBlockedDomains', description: 'Table of blocked domain name/hits' },
];

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  const statType = query.statType ?? DEFAULT_QUERY.statType;
  const series = query.series ?? DEFAULT_QUERY.series;

  const onStatTypeChange = (selected: SelectableValue<TechnitiumStatType>) => {
    onChange({ ...query, statType: selected.value! });
    onRunQuery();
  };

  const onSeriesChange = (selected: SelectableValue<TechnitiumSeries>) => {
    onChange({ ...query, series: selected.value! });
    onRunQuery();
  };

  return (
    <Stack gap={0}>
      <InlineField label="Series" labelWidth={16} tooltip="Which part of the dashboard stats response to return">
        <Select options={SERIES} value={series} onChange={onSeriesChange} width={28} />
      </InlineField>
      <InlineField label="Time window" labelWidth={16} tooltip="Technitium's own duration buckets, not the dashboard time picker">
        <Select options={STAT_TYPES} value={statType} onChange={onStatTypeChange} width={20} />
      </InlineField>
    </Stack>
  );
}
