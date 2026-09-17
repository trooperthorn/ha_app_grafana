import React from 'react';
import { InlineField, Select, Stack } from '@grafana/ui';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { DataSource } from '../datasource';
import { DEFAULT_QUERY, UnifiNetworkDataSourceOptions, UnifiNetworkQuery, UnifiNetworkSeries } from '../types';

type Props = QueryEditorProps<DataSource, UnifiNetworkQuery, UnifiNetworkDataSourceOptions>;

const SERIES: Array<SelectableValue<UnifiNetworkSeries>> = [
  { label: 'Clients', value: 'clients', description: 'One row per connected client' },
  { label: 'Devices', value: 'devices', description: 'One row per network infrastructure device' },
  { label: 'WAN', value: 'wan', description: 'A single derived WAN status row from the gateway device' },
];

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  const series = query.series ?? DEFAULT_QUERY.series;

  const onSeriesChange = (selected: SelectableValue<UnifiNetworkSeries>) => {
    onChange({ ...query, series: selected.value! });
    onRunQuery();
  };

  return (
    <Stack gap={0}>
      <InlineField label="Series" labelWidth={16} tooltip="Which table to return">
        <Select options={SERIES} value={series} onChange={onSeriesChange} width={28} />
      </InlineField>
    </Stack>
  );
}
