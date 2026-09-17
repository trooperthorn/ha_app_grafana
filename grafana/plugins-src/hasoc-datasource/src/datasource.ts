import { DataSourceInstanceSettings, CoreApp } from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';

import { DEFAULT_QUERY, HASOCDataSourceOptions, HASOCQuery } from './types';

export class DataSource extends DataSourceWithBackend<HASOCQuery, HASOCDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<HASOCDataSourceOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<HASOCQuery> {
    return DEFAULT_QUERY;
  }
}
