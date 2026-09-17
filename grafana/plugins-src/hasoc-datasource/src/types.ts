import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type HASOCSeries = 'posture' | 'risk' | 'audit';

export interface HASOCQuery extends DataQuery {
  series: HASOCSeries;
  auditLimit?: number;
}

export const DEFAULT_QUERY: Partial<HASOCQuery> = {
  series: 'posture',
  auditLimit: 200,
};

/** Non-secret settings entered on the datasource's config page. */
export interface HASOCDataSourceOptions extends DataSourceJsonData {
  url?: string;
  verifySSL?: boolean;
}

/** Secret settings, sent to the backend only, never back to the browser. */
export interface HASOCSecureJsonData {
  accessToken?: string;
}
