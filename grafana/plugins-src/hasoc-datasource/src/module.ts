import { DataSourcePlugin } from '@grafana/data';

import { DataSource } from './datasource';
import { ConfigEditor } from './components/ConfigEditor';
import { QueryEditor } from './components/QueryEditor';
import { HASOCQuery, HASOCDataSourceOptions } from './types';

export const plugin = new DataSourcePlugin<DataSource, HASOCQuery, HASOCDataSourceOptions>(DataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
