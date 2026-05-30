import { useState } from 'react';
import { Rocket, Copy, AlertCircle, Loader2 } from 'lucide-react';
import type { Template } from '@/api/marketplace';
import type { ServerNode } from '@/api/servers';
import { useApi } from '@/hooks';
import { marketplaceAPI } from '@/api/marketplace';
import { generatedSecretEntries, formatGeneratedSecrets } from '@/lib/generatedSecrets';
import { toast } from '@/stores/toastStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';
import { PasswordInput } from '@/components/Marketplace/PasswordInput';

interface DeployFormProps {
  template: Template;
  onDeployed: (appId: string) => void;
}

export function DeployForm({ template, onDeployed }: DeployFormProps) {
  const [name, setName] = useState(template.slug);
  const [domain, setDomain] = useState('');
  const [serverID, setServerID] = useState('local');
  const [config, setConfig] = useState<Record<string, string>>(() => {
    const defaults: Record<string, string> = {};
    if (template.config_schema?.properties) {
      for (const [key, prop] of Object.entries(template.config_schema!.properties!)) {
        defaults[key] = (prop as { default?: string }).default || '';
      }
    }
    return defaults;
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [generatedSecrets, setGeneratedSecrets] = useState<Record<string, string>>({});
  const [deployedAppId, setDeployedAppId] = useState('');
  const [credentialsReadyToLeave, setCredentialsReadyToLeave] = useState(false);
  const { data: serversResp } = useApi<{ data: ServerNode[]; total: number } | ServerNode[]>('/servers');
  const serverList = Array.isArray(serversResp) ? serversResp : serversResp?.data ?? [];
  const remoteServers = serverList.filter((server) => server.id !== 'local');

  const configFields = [];
  if (template.config_schema?.properties) {
    const schema = template.config_schema;
    const required = new Set(schema.required || []);
    for (const [key, prop] of Object.entries(schema.properties!)) {
      configFields.push({
        key,
        title: prop.title || key,
        description: prop.description,
        format: prop.format,
        required: required.has(key),
      });
    }
  }

  const handleDeploy = async () => {
    if (!name) return;
    setLoading(true);
    setError('');
    try {
      const result = await marketplaceAPI.deploy({
        slug: template.slug,
        name,
        domain: domain.trim() || undefined,
        server_id: serverID === 'local' ? undefined : serverID,
        config,
      });
      const secrets = result.generated_secrets || {};
      if (Object.keys(secrets).length > 0) {
        setGeneratedSecrets(secrets);
        setDeployedAppId(result.app_id);
        setCredentialsReadyToLeave(false);
      } else {
        onDeployed(result.app_id);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Deployment failed');
    } finally {
      setLoading(false);
    }
  };

  const copyGeneratedSecrets = async () => {
    try {
      await navigator.clipboard.writeText(formatGeneratedSecrets(generatedSecrets));
      setCredentialsReadyToLeave(true);
      toast.success('Credentials copied');
    } catch {
      setCredentialsReadyToLeave(true);
      toast.error('Failed to copy credentials');
    }
  };

  if (Object.keys(generatedSecrets).length > 0) {
    return (
      <div className="space-y-4">
        <div className="space-y-3 rounded-lg border bg-muted/30 p-4">
          <Label className="text-sm font-medium">Generated Credentials</Label>
          <p className="text-xs text-muted-foreground">
            These generated values are shown once. Copy or save them before opening the app.
          </p>
          <div className="space-y-2">
            {generatedSecretEntries(generatedSecrets).map(([key, value]) => (
              <div key={key} className="grid gap-1 rounded-md border bg-background px-3 py-2">
                <span className="text-xs font-medium text-muted-foreground">{key}</span>
                <code className="break-all text-sm">{value}</code>
              </div>
            ))}
          </div>
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          <Button variant="outline" onClick={copyGeneratedSecrets}>
            <Copy className="size-4" />
            Copy Credentials
          </Button>
          <Button onClick={() => onDeployed(deployedAppId)} disabled={!credentialsReadyToLeave}>
            Open App
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="deploy-name">Stack Name</Label>
        <Input id="deploy-name" type="text" value={name} onChange={(e) => setName(e.target.value)} placeholder="my-app" />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="deploy-domain">Domain</Label>
        <Input id="deploy-domain" type="text" value={domain} onChange={(e) => setDomain(e.target.value)} placeholder="app.example.com" />
        <p className="text-[11px] text-muted-foreground">Optional. Leave empty to add a domain later.</p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="deploy-server">Target Server</Label>
        <Select id="deploy-server" value={serverID} onChange={(e) => setServerID(e.target.value)}>
          <option value="local">Local server</option>
          {remoteServers.map((server) => (
            <option key={server.id} value={server.id} disabled={server.connected === false}>
              {server.hostname || server.id}
              {server.connected === false ? ' (agent disconnected)' : ''}
            </option>
          ))}
        </Select>
      </div>
      {configFields.length > 0 && (
        <div className="space-y-3">
          <Label className="text-sm font-medium">Configuration</Label>
          {configFields.map((field) => (
            <div key={field.key} className="space-y-1.5">
              <Label htmlFor={`config-${field.key}`}>
                {field.title}
                {field.required && <span className="text-destructive ml-1">*</span>}
              </Label>
              {field.format === 'password' ? (
                <PasswordInput
                  id={`config-${field.key}`}
                  value={config[field.key] || ''}
                  onChange={(val) => setConfig({ ...config, [field.key]: val })}
                  placeholder={field.description || field.title}
                />
              ) : (
                <Input
                  id={`config-${field.key}`}
                  type="text"
                  value={config[field.key] || ''}
                  onChange={(e) => setConfig({ ...config, [field.key]: e.target.value })}
                  placeholder={field.description || field.title}
                />
              )}
              {field.description && <p className="text-[11px] text-muted-foreground">{field.description}</p>}
            </div>
          ))}
        </div>
      )}
      {error && (
        <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2.5 text-sm text-destructive">
          <AlertCircle className="size-4 shrink-0" />
          {error}
        </div>
      )}
      <Button onClick={handleDeploy} disabled={loading || !name} className="w-full">
        {loading ? (
          <>
            <Loader2 className="size-4 animate-spin" />
            Deploying...
          </>
        ) : (
          <>
            <Rocket className="size-4" />
            Deploy {template.name}
          </>
        )}
      </Button>
    </div>
  );
}