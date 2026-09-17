import { DataSourceInstanceSettings, CoreApp } from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';

import { DEFAULT_QUERY, UnifiNetworkDataSourceOptions, UnifiNetworkQuery } from './types';

export class DataSource extends DataSourceWithBackend<UnifiNetworkQuery, UnifiNetworkDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<UnifiNetworkDataSourceOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<UnifiNetworkQuery> {
    return DEFAULT_QUERY;
  }
}
