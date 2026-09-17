import { DataSourcePlugin } from '@grafana/data';

import { DataSource } from './datasource';
import { ConfigEditor } from './components/ConfigEditor';
import { QueryEditor } from './components/QueryEditor';
import { HomeAssistantQuery, HomeAssistantDataSourceOptions } from './types';

export const plugin = new DataSourcePlugin<DataSource, HomeAssistantQuery, HomeAssistantDataSourceOptions>(DataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
