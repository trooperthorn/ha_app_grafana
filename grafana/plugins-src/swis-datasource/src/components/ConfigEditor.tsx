import React, { ChangeEvent } from 'react';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { FieldSet, InlineField, InlineSwitch, Input, SecretInput, SecretTextArea, TextArea } from '@grafana/ui';

import { SwisDataSourceOptions, SwisSecureJsonData } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<SwisDataSourceOptions, SwisSecureJsonData> {}

const LABEL_WIDTH = 22;

export function ConfigEditor({ onOptionsChange, options }: Props) {
  const { jsonData, secureJsonFields, secureJsonData } = options;

  const setJson = (patch: Partial<SwisDataSourceOptions>) =>
    onOptionsChange({ ...options, jsonData: { ...jsonData, ...patch } });

  const setSecure = (patch: Partial<SwisSecureJsonData>) =>
    onOptionsChange({ ...options, secureJsonData: { ...secureJsonData, ...patch } });

  const resetSecure = (key: keyof SwisSecureJsonData) =>
    onOptionsChange({
      ...options,
      secureJsonFields: { ...secureJsonFields, [key]: false },
      secureJsonData: { ...secureJsonData, [key]: '' },
    });

  return (
    <>
      <FieldSet label="Connection">
        <InlineField
          label="Orion server"
          labelWidth={LABEL_WIDTH}
          tooltip="Hostname or IP of the SolarWinds Platform server. No scheme, no port."
        >
          <Input
            id="swis-host"
            width={40}
            value={jsonData.host ?? ''}
            placeholder="orion.example.com"
            onChange={(e: ChangeEvent<HTMLInputElement>) => setJson({ host: e.target.value })}
          />
        </InlineField>
        <InlineField
          label="REST port"
          labelWidth={LABEL_WIDTH}
          tooltip="17774 from platform release 2023.1 onward. 17778 is the deprecated pre-2023 port. 17777 is SOAP and will not work here."
        >
          <Input
            id="swis-port"
            width={12}
            type="number"
            value={jsonData.port ?? 17774}
            onChange={(e: ChangeEvent<HTMLInputElement>) => setJson({ port: parseInt(e.target.value, 10) || 17774 })}
          />
        </InlineField>
      </FieldSet>

      <FieldSet label="Authentication">
        <InlineField
          label="Username"
          labelWidth={LABEL_WIDTH}
          tooltip="An Orion account. Give Grafana its own read-only account with the narrowest limitation that still shows what the dashboards need."
        >
          <Input
            id="swis-username"
            width={40}
            value={jsonData.username ?? ''}
            onChange={(e: ChangeEvent<HTMLInputElement>) => setJson({ username: e.target.value })}
          />
        </InlineField>
        <InlineField
          label="Password"
          labelWidth={LABEL_WIDTH}
          tooltip="Stored encrypted by Grafana; never sent back to the browser."
        >
          <SecretInput
            id="swis-password"
            width={40}
            isConfigured={!!secureJsonFields?.password}
            value={secureJsonData?.password ?? ''}
            onReset={() => resetSecure('password')}
            onChange={(e: ChangeEvent<HTMLInputElement>) => setSecure({ password: e.target.value })}
          />
        </InlineField>
      </FieldSet>

      <FieldSet label="TLS">
        <InlineField
          label="CA certificate (PEM)"
          labelWidth={LABEL_WIDTH}
          tooltip="SWIS ships with a self-signed certificate. Paste it (or the CA that issued it) here so verification stays on. openssl s_client -connect orion:17774 -showcerts prints it. With the stock certificate, also turn on Ignore certificate name."
        >
          <SecretTextArea
            id="swis-cacert"
            cols={60}
            rows={6}
            isConfigured={!!secureJsonFields?.caCert}
            value={secureJsonData?.caCert ?? ''}
            placeholder="-----BEGIN CERTIFICATE-----"
            onReset={() => resetSecure('caCert')}
            onChange={(e: ChangeEvent<HTMLTextAreaElement>) => setSecure({ caCert: e.target.value })}
          />
        </InlineField>
        <InlineField
          label="Ignore certificate name"
          labelWidth={LABEL_WIDTH}
          tooltip="Keep verifying that the server presents the pasted certificate, but do not require its name to match the host. The certificate SWIS ships with is issued to a fixed name with no subject alternative names, so this is what pinning it needs. Any other certificate is still refused."
        >
          <InlineSwitch
            id="swis-ignorehostname"
            value={!!jsonData.tlsIgnoreHostname}
            onChange={(e: React.FormEvent<HTMLInputElement>) => setJson({ tlsIgnoreHostname: e.currentTarget.checked })}
          />
        </InlineField>
        <InlineField
          label="Skip TLS verification"
          labelWidth={LABEL_WIDTH}
          tooltip="Lab only. Anything on the network path can then read the credentials Grafana sends."
        >
          <InlineSwitch
            id="swis-skipverify"
            value={!!jsonData.tlsSkipVerify}
            onChange={(e: React.FormEvent<HTMLInputElement>) => setJson({ tlsSkipVerify: e.currentTarget.checked })}
          />
        </InlineField>
      </FieldSet>

      <FieldSet label="Limits">
        <InlineField
          label="Max rows per query"
          labelWidth={LABEL_WIDTH}
          tooltip="Rows beyond this are dropped and the panel shows a warning. It is a safety net, not a substitute for TOP n in the statement."
        >
          <Input
            id="swis-maxrows"
            width={12}
            type="number"
            value={jsonData.maxRows ?? 10000}
            onChange={(e: ChangeEvent<HTMLInputElement>) => setJson({ maxRows: parseInt(e.target.value, 10) || 10000 })}
          />
        </InlineField>
        <InlineField label="Timeout (seconds)" labelWidth={LABEL_WIDTH}>
          <Input
            id="swis-timeout"
            width={12}
            type="number"
            value={jsonData.timeoutSeconds ?? 60}
            onChange={(e: ChangeEvent<HTMLInputElement>) =>
              setJson({ timeoutSeconds: parseInt(e.target.value, 10) || 60 })
            }
          />
        </InlineField>
      </FieldSet>

      <FieldSet label="Invoke (verbs)">
        <InlineField
          label="Allowed verbs"
          labelWidth={LABEL_WIDTH}
          tooltip="One Entity.Verb per line, for example Orion.Nodes.PollNow. Leave empty to disable Invoke entirely. Only Grafana Editors and Admins can call an allowed verb, and every call is written to the Grafana server log with the caller and the arguments."
        >
          <TextArea
            id="swis-invoke-allow"
            cols={60}
            rows={4}
            value={(jsonData.invokeAllow ?? []).join('\n')}
            placeholder={'Orion.Nodes.PollNow\nOrion.AlertActive.Acknowledge'}
            onChange={(e: ChangeEvent<HTMLTextAreaElement>) =>
              setJson({
                invokeAllow: e.target.value
                  .split('\n')
                  .map((s) => s.trim())
                  .filter(Boolean),
              })
            }
          />
        </InlineField>
      </FieldSet>
    </>
  );
}
