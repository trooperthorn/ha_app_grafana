import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type UnifiNetworkSeries = 'clients' | 'devices' | 'wan';

export interface UnifiNetworkQuery extends DataQuery {
  series: UnifiNetworkSeries;
}

export const DEFAULT_QUERY: Partial<UnifiNetworkQuery> = {
  series: 'clients',
};

/** Non-secret settings entered on the datasource's config page. */
export interface UnifiNetworkDataSourceOptions extends DataSourceJsonData {
  host?: string;
  verifySSL?: boolean;
}

/** Secret settings, sent to the backend only, never back to the browser. */
export interface UnifiNetworkSecureJsonData {
  apiKey?: string;
}
