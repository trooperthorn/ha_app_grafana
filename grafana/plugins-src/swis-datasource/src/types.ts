import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type SwisFormat = 'table' | 'timeseries';

/** What a panel stores for one query. `swql` is sent to the backend after template variables are expanded. */
export interface SwisQuery extends DataQuery {
  swql?: string;
  format?: SwisFormat;
  /** Extra bound parameters (name -> value) referenced as @name in the statement. */
  parameters?: Record<string, unknown>;
}

export const DEFAULT_QUERY: Partial<SwisQuery> = {
  swql: '',
  format: 'table',
};

/** Data source settings stored in Grafana's jsonData. Nothing here is secret. */
export interface SwisDataSourceOptions extends DataSourceJsonData {
  host?: string;
  port?: number;
  username?: string;
  tlsSkipVerify?: boolean;
  /** Verify the chain against the pasted certificate but not the name. Needed for the stock SWIS certificate. */
  tlsIgnoreHostname?: boolean;
  maxRows?: number;
  timeoutSeconds?: number;
  /** Entity.Verb names the Invoke resource may call, for example "Orion.Nodes.PollNow". */
  invokeAllow?: string[];
}

/** Encrypted by Grafana, sent to the backend only, never returned to the browser. */
export interface SwisSecureJsonData {
  password?: string;
  caCert?: string;
}
