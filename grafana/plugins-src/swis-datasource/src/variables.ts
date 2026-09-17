import { DataSourceVariableSupport } from '@grafana/data';

import { DataSource } from './datasource';
import { SwisDataSourceOptions, SwisQuery } from './types';

/**
 * Lets a dashboard variable be a SWQL statement, using the same query editor a panel
 * uses. Grafana reads the resulting frame the way it does for every backend data source:
 * a column aliased `__text` is the label and one aliased `__value` is the value; with a
 * single column, that column is both.
 */
export class SwisVariableSupport extends DataSourceVariableSupport<DataSource, SwisQuery, SwisDataSourceOptions> {}
