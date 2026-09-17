import { CoreApp, DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';

import { DEFAULT_QUERY, SwisDataSourceOptions, SwisQuery } from './types';
import { SwisVariableSupport } from './variables';

export class DataSource extends DataSourceWithBackend<SwisQuery, SwisDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<SwisDataSourceOptions>) {
    super(instanceSettings);
    this.variables = new SwisVariableSupport();
  }

  getDefaultQuery(_: CoreApp): Partial<SwisQuery> {
    return DEFAULT_QUERY;
  }

  /**
   * Dashboard variables are expanded here, in the browser, before the statement goes to
   * the backend. The Grafana time macros are left alone: the backend turns them into
   * bound parameters so the range never becomes literal text in the statement.
   */
  applyTemplateVariables(query: SwisQuery, scopedVars: ScopedVars): SwisQuery {
    return {
      ...query,
      swql: getTemplateSrv().replace(query.swql ?? '', scopedVars),
    };
  }

  filterQuery(query: SwisQuery): boolean {
    return !!query.swql?.trim();
  }

  /**
   * Call a SWIS verb through the backend. Arguments are positional: the order is the
   * verb's declared parameter order and nothing else. Look it up first with
   * `python3 tools/schema_query.py verb <Entity> <Verb>` in OrionGuides. The backend
   * refuses any verb that is not on the data source's allowlist, and any caller who is
   * not an Editor or Admin.
   */
  async invoke<T = unknown>(entity: string, verb: string, args: unknown[] = []): Promise<T> {
    return this.postResource<T>(`invoke/${encodeURIComponent(entity)}/${encodeURIComponent(verb)}`, args);
  }

  /** The Entity.Verb names this data source is allowed to invoke. */
  async allowedVerbs(): Promise<string[]> {
    const res = await this.getResource<{ invokeAllow: string[] }>('verbs');
    return res.invokeAllow ?? [];
  }
}
