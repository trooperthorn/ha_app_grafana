import { DataSourceInstanceSettings, CoreApp } from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';

import { DEFAULT_QUERY, MusicAssistantDataSourceOptions, MusicAssistantQuery } from './types';

export class DataSource extends DataSourceWithBackend<MusicAssistantQuery, MusicAssistantDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<MusicAssistantDataSourceOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<MusicAssistantQuery> {
    return DEFAULT_QUERY;
  }
}
