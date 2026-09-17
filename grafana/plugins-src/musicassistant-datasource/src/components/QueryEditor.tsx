import React from 'react';
import { InlineField, Select, Stack } from '@grafana/ui';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { DataSource } from '../datasource';
import { DEFAULT_QUERY, MusicAssistantDataSourceOptions, MusicAssistantQuery, MusicAssistantSeries } from '../types';

type Props = QueryEditorProps<DataSource, MusicAssistantQuery, MusicAssistantDataSourceOptions>;

const SERIES: Array<SelectableValue<MusicAssistantSeries>> = [
  { label: 'Players', value: 'players', description: 'One row per player: power, playback state, volume' },
  { label: 'Now playing', value: 'nowPlaying', description: 'One row per player currently playing something' },
];

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  const series = query.series ?? DEFAULT_QUERY.series;

  const onSeriesChange = (selected: SelectableValue<MusicAssistantSeries>) => {
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
