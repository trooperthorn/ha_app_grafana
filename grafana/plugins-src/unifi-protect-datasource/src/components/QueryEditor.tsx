import React from 'react';
import { QueryEditorProps } from '@grafana/data';
import { DataSource } from '../datasource';
import { UnifiProtectDataSourceOptions, UnifiProtectQuery } from '../types';

type Props = QueryEditorProps<DataSource, UnifiProtectQuery, UnifiProtectDataSourceOptions>;

// Protect exposes only a camera inventory over its Integration API (no
// historical REST events/detections route; see this plugin's README and
// docs/decisions.md), so there is exactly one series and nothing for the
// operator to choose here.
export function QueryEditor(_: Props) {
  return <div>Returns the camera inventory: id, name, IP, MAC, recording state, last ring, channel count, online state, and a console deep link.</div>;
}
