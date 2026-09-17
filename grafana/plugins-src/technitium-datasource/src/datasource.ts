import { DataSourceInstanceSettings, CoreApp } from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';

import { DEFAULT_QUERY, TechnitiumDataSourceOptions, TechnitiumQuery } from './types';

export class DataSource extends DataSourceWithBackend<TechnitiumQuery, TechnitiumDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<TechnitiumDataSourceOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<TechnitiumQuery> {
    return DEFAULT_QUERY;
  }
}
