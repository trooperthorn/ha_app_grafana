import { DataSourcePlugin } from '@grafana/data';

import { DataSource } from './datasource';
import { ConfigEditor } from './components/ConfigEditor';
import { QueryEditor } from './components/QueryEditor';
import { TechnitiumQuery, TechnitiumDataSourceOptions } from './types';

export const plugin = new DataSourcePlugin<DataSource, TechnitiumQuery, TechnitiumDataSourceOptions>(DataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
