import React, { ChangeEvent } from 'react';
import { InlineField, Input, Select, Stack } from '@grafana/ui';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { DataSource } from '../datasource';
import {
  DEFAULT_QUERY,
  HomeAssistantDataSourceOptions,
  HomeAssistantPeriod,
  HomeAssistantQuery,
  HomeAssistantSeries,
} from '../types';

type Props = QueryEditorProps<DataSource, HomeAssistantQuery, HomeAssistantDataSourceOptions>;

const SERIES: Array<SelectableValue<HomeAssistantSeries>> = [
  { label: 'Statistics', value: 'statistics', description: 'Long-term statistics for one or more statistic ids' },
  { label: 'System health', value: 'system_health', description: 'One row per domain/key health fact' },
];

const PERIODS: Array<SelectableValue<HomeAssistantPeriod>> = [
  { label: '5 minutes', value: '5minute' },
  { label: 'Hour', value: 'hour' },
  { label: 'Day', value: 'day' },
  { label: 'Week', value: 'week' },
  { label: 'Month', value: 'month' },
  { label: 'Year', value: 'year' },
];

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  const series = query.series ?? DEFAULT_QUERY.series;
  const period = query.period ?? DEFAULT_QUERY.period;

  const onSeriesChange = (selected: SelectableValue<HomeAssistantSeries>) => {
    onChange({ ...query, series: selected.value! });
    onRunQuery();
  };

  const onPeriodChange = (selected: SelectableValue<HomeAssistantPeriod>) => {
    onChange({ ...query, period: selected.value! });
    onRunQuery();
  };

  const onStatisticIdsChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...query, statisticIds: event.target.value });
  };

  return (
    <Stack gap={0} direction="column">
      <Stack gap={0}>
        <InlineField label="Series" labelWidth={16} tooltip="Which table to return">
          <Select options={SERIES} value={series} onChange={onSeriesChange} width={24} />
        </InlineField>
        {series === 'statistics' && (
          <InlineField label="Period" labelWidth={12} tooltip="Home Assistant's own statistics bucket size">
            <Select options={PERIODS} value={period} onChange={onPeriodChange} width={16} />
          </InlineField>
        )}
      </Stack>
      {series === 'statistics' && (
        <InlineField
          label="Statistic IDs"
          labelWidth={16}
          tooltip="Comma-separated, e.g. sensor.outdoor_temperature,sensor.energy_total"
        >
          <Input
            id="query-editor-statistic-ids"
            onChange={onStatisticIdsChange}
            onBlur={onRunQuery}
            value={query.statisticIds ?? ''}
            placeholder="sensor.outdoor_temperature"
            width={60}
          />
        </InlineField>
      )}
    </Stack>
  );
}
