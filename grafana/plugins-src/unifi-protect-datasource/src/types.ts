import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type UnifiProtectSeries = 'cameras';

export interface UnifiProtectQuery extends DataQuery {
  series: UnifiProtectSeries;
}

export const DEFAULT_QUERY: Partial<UnifiProtectQuery> = {
  series: 'cameras',
};

/** Non-secret settings entered on the datasource's config page. */
export interface UnifiProtectDataSourceOptions extends DataSourceJsonData {
  host?: string;
  verifySSL?: boolean;
}

/** Secret settings, sent to the backend only, never back to the browser. */
export interface UnifiProtectSecureJsonData {
  apiKey?: string;
}
