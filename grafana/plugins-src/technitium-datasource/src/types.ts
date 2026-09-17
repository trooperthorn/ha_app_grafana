import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type TechnitiumStatType = 'LastHour' | 'LastDay' | 'LastWeek' | 'LastMonth' | 'LastYear';
export type TechnitiumSeries = 'volume' | 'topClients' | 'topDomains' | 'topBlockedDomains' | 'queryLogs';

export interface TechnitiumQuery extends DataQuery {
  statType: TechnitiumStatType;
  series: TechnitiumSeries;
  /** Optional filters for the "queryLogs" series; ignored otherwise. */
  qname?: string;
  clientIpAddress?: string;
}

export const DEFAULT_QUERY: Partial<TechnitiumQuery> = {
  statType: 'LastDay',
  series: 'volume',
};

/** Non-secret settings entered on the datasource's config page. */
export interface TechnitiumDataSourceOptions extends DataSourceJsonData {
  url?: string;
  /**
   * The installed name of the "Query Logs (Sqlite)" DNS app, for servers
   * that renamed it. Empty uses the store default, "Query Logs (Sqlite)".
   */
  queryLogsAppName?: string;
}

/** Secret settings, sent to the backend only, never back to the browser. */
export interface TechnitiumSecureJsonData {
  apiToken?: string;
}
