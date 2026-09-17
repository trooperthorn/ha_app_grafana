import React, { ChangeEvent } from 'react';
import { InlineField, Input, SecretInput } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { TechnitiumDataSourceOptions, TechnitiumSecureJsonData } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<TechnitiumDataSourceOptions, TechnitiumSecureJsonData> {}

export function ConfigEditor(props: Props) {
  const { onOptionsChange, options } = props;
  const { jsonData, secureJsonFields, secureJsonData } = options;

  const onURLChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        url: event.target.value,
      },
    });
  };

  // Secure field: sent to the backend only, never read back into the browser.
  const onAPITokenChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        apiToken: event.target.value,
      },
    });
  };

  const onResetAPIToken = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...options.secureJsonFields,
        apiToken: false,
      },
      secureJsonData: {
        ...options.secureJsonData,
        apiToken: '',
      },
    });
  };

  return (
    <>
      <InlineField label="Server URL" labelWidth={16} interactive tooltip="The Technitium DNS Server web console's own origin">
        <Input
          id="config-editor-url"
          onChange={onURLChange}
          value={jsonData.url}
          placeholder="http://192.168.1.10:5380"
          width={40}
        />
      </InlineField>
      <InlineField
        label="API Token"
        labelWidth={16}
        interactive
        tooltip="Create a non-expiring token in the Technitium console: Administration > Sessions > Create API Token"
      >
        <SecretInput
          required
          id="config-editor-api-token"
          isConfigured={secureJsonFields.apiToken}
          value={secureJsonData?.apiToken}
          placeholder="Enter the API token"
          width={40}
          onReset={onResetAPIToken}
          onChange={onAPITokenChange}
        />
      </InlineField>
    </>
  );
}
