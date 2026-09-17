import React from 'react';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { CodeEditor, InlineField, InlineFieldRow, Select, Stack } from '@grafana/ui';

import { DataSource } from '../datasource';
import { EXAMPLES } from '../examples';
import { SwisDataSourceOptions, SwisFormat, SwisQuery } from '../types';

type Props = QueryEditorProps<DataSource, SwisQuery, SwisDataSourceOptions>;

const FORMATS: Array<SelectableValue<SwisFormat>> = [
  { label: 'Table', value: 'table', description: 'Rows as returned. Use for tables, stats, bar gauges and variables.' },
  {
    label: 'Time series',
    value: 'timeseries',
    description: 'Needs a DateTime column. String columns become series labels, one series per distinct value.',
  },
];

const EXAMPLE_OPTIONS: Array<SelectableValue<number>> = EXAMPLES.map((e, i) => ({ label: e.label, value: i }));

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  const swql = query.swql ?? '';

  return (
    <Stack direction="column" gap={1}>
      <CodeEditor
        language="sql"
        height="180px"
        showMiniMap={false}
        showLineNumbers={true}
        value={swql}
        onBlur={(value) => {
          if (value !== swql) {
            onChange({ ...query, swql: value });
            onRunQuery();
          }
        }}
        onSave={(value) => {
          onChange({ ...query, swql: value });
          onRunQuery();
        }}
      />
      <InlineFieldRow>
        <InlineField label="Format" labelWidth={12}>
          <Select
            inputId="swis-format"
            width={24}
            options={FORMATS}
            value={query.format ?? 'table'}
            onChange={(v) => {
              onChange({ ...query, format: v.value ?? 'table' });
              onRunQuery();
            }}
          />
        </InlineField>
        <InlineField label="Example" labelWidth={12} tooltip="Replaces the statement with a validated starting point.">
          <Select
            inputId="swis-example"
            width={40}
            placeholder="Insert an example query"
            options={EXAMPLE_OPTIONS}
            value={null}
            onChange={(v) => {
              const example = EXAMPLES[v.value ?? -1];
              if (example) {
                onChange({ ...query, swql: example.swql, format: example.format });
                onRunQuery();
              }
            }}
          />
        </InlineField>
      </InlineFieldRow>
      <div className="grafana-info-box" style={{ fontSize: '12px', opacity: 0.85 }}>
        Macros: <code>$__timeFilter(alias.Column)</code>, <code>$__timeFrom()</code>, <code>$__timeTo()</code> bind the
        dashboard range as UTC parameters. Dashboard variables (<code>$node</code>,{' '}
        <code>
          ${'{'}node:csv{'}'}
        </code>
        ) are expanded before the statement is sent. Statistics and history entities are the largest tables on the
        server, so keep <code>TOP n</code> and a time bound on them. Ctrl+S or leaving the editor runs the query.
      </div>
    </Stack>
  );
}
