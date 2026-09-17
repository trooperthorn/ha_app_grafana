import { DataSourcePlugin } from '@grafana/data';

import { DataSource } from './datasource';
import { ConfigEditor } from './components/ConfigEditor';
import { QueryEditor } from './components/QueryEditor';
import { UnifiNetworkQuery, UnifiNetworkDataSourceOptions } from './types';

export const plugin = new DataSourcePlugin<DataSource, UnifiNetworkQuery, UnifiNetworkDataSourceOptions>(DataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
