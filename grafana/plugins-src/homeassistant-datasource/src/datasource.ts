import { DataSourceInstanceSettings, CoreApp } from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';

import { DEFAULT_QUERY, HomeAssistantDataSourceOptions, HomeAssistantQuery } from './types';

export class DataSource extends DataSourceWithBackend<HomeAssistantQuery, HomeAssistantDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<HomeAssistantDataSourceOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<HomeAssistantQuery> {
    return DEFAULT_QUERY;
  }
}
