import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type HomeAssistantSeries = 'statistics' | 'system_health';
export type HomeAssistantPeriod = '5minute' | 'hour' | 'day' | 'week' | 'month' | 'year';

export interface HomeAssistantQuery extends DataQuery {
  series: HomeAssistantSeries;
  statisticIds?: string;
  period?: HomeAssistantPeriod;
}

export const DEFAULT_QUERY: Partial<HomeAssistantQuery> = {
  series: 'statistics',
  period: 'hour',
};

/** Non-secret settings entered on the datasource's config page. */
export interface HomeAssistantDataSourceOptions extends DataSourceJsonData {
  url?: string;
  verifySSL?: boolean;
}

/** Secret settings, sent to the backend only, never back to the browser. */
export interface HomeAssistantSecureJsonData {
  accessToken?: string;
}
