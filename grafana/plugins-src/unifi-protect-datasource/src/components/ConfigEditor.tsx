import React, { ChangeEvent } from 'react';
import { InlineField, InlineSwitch, Input, SecretInput } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { UnifiProtectDataSourceOptions, UnifiProtectSecureJsonData } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<UnifiProtectDataSourceOptions, UnifiProtectSecureJsonData> {}

export function ConfigEditor(props: Props) {
  const { onOptionsChange, options } = props;
  const { jsonData, secureJsonFields, secureJsonData } = options;

  const onHostChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        host: event.target.value,
      },
    });
  };

  const onVerifySSLChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        verifySSL: event.target.checked,
      },
    });
  };

  // Secure field: sent to the backend only, never read back into the browser.
  const onAPIKeyChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        apiKey: event.target.value,
      },
    });
  };

  const onResetAPIKey = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...options.secureJsonFields,
        apiKey: false,
      },
      secureJsonData: {
        ...options.secureJsonData,
        apiKey: '',
      },
    });
  };

  return (
    <>
      <InlineField label="Console host" labelWidth={18} interactive tooltip="e.g. 192.168.1.1 or https://192.168.1.1">
        <Input
          id="config-editor-host"
          onChange={onHostChange}
          value={jsonData.host}
          placeholder="192.168.1.1"
          width={40}
        />
      </InlineField>
      <InlineField
        label="Verify TLS certificate"
        labelWidth={18}
        interactive
        tooltip="Off by default: most local consoles present a self-signed certificate"
      >
        <InlineSwitch id="config-editor-verify-ssl" value={Boolean(jsonData.verifySSL)} onChange={onVerifySSLChange} />
      </InlineField>
      <InlineField
        label="API Key"
        labelWidth={18}
        interactive
        tooltip="A local Integration API key from the console's Settings > Control Plane > Integrations"
      >
        <SecretInput
          required
          id="config-editor-api-key"
          isConfigured={secureJsonFields.apiKey}
          value={secureJsonData?.apiKey}
          placeholder="Enter the API key"
          width={40}
          onReset={onResetAPIKey}
          onChange={onAPIKeyChange}
        />
      </InlineField>
    </>
  );
}
