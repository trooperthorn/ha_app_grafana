import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type MusicAssistantSeries = 'players' | 'nowPlaying';

export interface MusicAssistantQuery extends DataQuery {
  series: MusicAssistantSeries;
}

export const DEFAULT_QUERY: Partial<MusicAssistantQuery> = {
  series: 'players',
};

/** Non-secret settings entered on the datasource's config page. */
export interface MusicAssistantDataSourceOptions extends DataSourceJsonData {
  url?: string;
}

/** Secret settings, sent to the backend only, never back to the browser. */
export interface MusicAssistantSecureJsonData {
  apiToken?: string;
}
