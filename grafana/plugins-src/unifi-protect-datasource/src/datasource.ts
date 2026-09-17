import { DataSourceInstanceSettings, CoreApp } from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';

import { DEFAULT_QUERY, UnifiProtectDataSourceOptions, UnifiProtectQuery } from './types';

export class DataSource extends DataSourceWithBackend<UnifiProtectQuery, UnifiProtectDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<UnifiProtectDataSourceOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<UnifiProtectQuery> {
    return DEFAULT_QUERY;
  }
}
