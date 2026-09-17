import React, { ChangeEvent } from 'react';
import { InlineField, InlineSwitch, Input, SecretInput } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { HomeAssistantDataSourceOptions, HomeAssistantSecureJsonData } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<HomeAssistantDataSourceOptions, HomeAssistantSecureJsonData> {}

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
  const onAccessTokenChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        accessToken: event.target.value,
      },
    });
  };

  const onResetAccessToken = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...options.secureJsonFields,
        accessToken: false,
      },
      secureJsonData: {
        ...options.secureJsonData,
        accessToken: '',
      },
    });
  };

  return (
    <>
      <InlineField label="Instance URL" labelWidth={16} interactive tooltip="Home Assistant's own origin">
        <Input
          id="config-editor-url"
          onChange={onURLChange}
          value={jsonData.url}
          placeholder="http://homeassistant.local:8123"
          width={40}
        />
      </InlineField>
      <InlineField
        label="Verify TLS certificate"
        labelWidth={16}
        interactive
        tooltip="Off by default: turn on once your instance has a certificate to verify"
      >
        <InlineSwitch
          id="config-editor-verify-ssl"
          value={Boolean(jsonData.verifySSL)}
          onChange={onVerifySSLChange}
        />
      </InlineField>
      <InlineField
        label="Access Token"
        labelWidth={16}
        interactive
        tooltip="A long-lived access token from your Home Assistant profile (Security tab)"
      >
        <SecretInput
          required
          id="config-editor-access-token"
          isConfigured={secureJsonFields.accessToken}
          value={secureJsonData?.accessToken}
          placeholder="Enter the access token"
          width={40}
          onReset={onResetAccessToken}
          onChange={onAccessTokenChange}
        />
      </InlineField>
    </>
  );
}
