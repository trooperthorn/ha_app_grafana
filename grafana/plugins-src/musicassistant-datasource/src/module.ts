import { DataSourcePlugin } from '@grafana/data';

import { DataSource } from './datasource';
import { ConfigEditor } from './components/ConfigEditor';
import { QueryEditor } from './components/QueryEditor';
import { MusicAssistantQuery, MusicAssistantDataSourceOptions } from './types';

export const plugin = new DataSourcePlugin<DataSource, MusicAssistantQuery, MusicAssistantDataSourceOptions>(DataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
