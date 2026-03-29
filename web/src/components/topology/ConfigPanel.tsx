import { useState } from 'react';
import { X, Trash2, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Separator } from '@/components/ui/separator';
import { useTopologyStore } from '@/stores/topologyStore';
import type { AppNodeData, DatabaseNodeData, DomainNodeData, VolumeNodeData, WorkerNodeData } from '@/types/topology';

interface ConfigPanelProps {
  selectedNode: { id: string; type: string; data: Record<string, unknown> } | null;
  onClose: () => void;
}

export function ConfigPanel({ selectedNode, onClose }: ConfigPanelProps) {
  const { updateNode, removeNode } = useTopologyStore();
  const [newEnvKey, setNewEnvKey] = useState('');
  const [newEnvValue, setNewEnvValue] = useState('');

  if (!selectedNode) {
    return (
      <div className="flex h-full w-72 flex-col border-l bg-card">
        <div className="flex items-center justify-between border-b p-4">
          <h3 className="text-sm font-semibold">Configuration</h3>
        </div>
        <div className="flex flex-1 items-center justify-center p-4">
          <p className="text-center text-sm text-muted-foreground">
            Select a component to configure its settings
          </p>
        </div>
      </div>
    );
  }

  const handleChange = (field: string, value: unknown) => {
    updateNode(selectedNode.id, { [field]: value } as Partial<AppNodeData>);
  };

  const handleAddEnvVar = () => {
    if (newEnvKey.trim()) {
      const currentEnvVars = (selectedNode.data.envVars as Record<string, string>) || {};
      handleChange('envVars', {
        ...currentEnvVars,
        [newEnvKey.trim()]: newEnvValue,
      });
      setNewEnvKey('');
      setNewEnvValue('');
    }
  };

  const handleRemoveEnvVar = (key: string) => {
    const currentEnvVars = (selectedNode.data.envVars as Record<string, string>) || {};
    const newEnvVars = { ...currentEnvVars };
    delete newEnvVars[key];
    handleChange('envVars', newEnvVars);
  };

  const handleDelete = () => {
    removeNode(selectedNode.id);
    onClose();
  };

  const renderAppConfig = () => {
    const data = selectedNode.data as Partial<AppNodeData>;
    const envVars = (data.envVars as Record<string, string>) || {};

    return (
      <>
        <div className="space-y-2">
          <Label htmlFor="gitUrl">Git Repository</Label>
          <Input
            id="gitUrl"
            placeholder="https://github.com/user/repo"
            value={data.gitUrl || ''}
            onChange={(e) => handleChange('gitUrl', e.target.value)}
          />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-2">
            <Label htmlFor="branch">Branch</Label>
            <Input
              id="branch"
              placeholder="main"
              value={data.branch || 'main'}
              onChange={(e) => handleChange('branch', e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="port">Port</Label>
            <Input
              id="port"
              type="number"
              placeholder="3000"
              value={data.port || 3000}
              onChange={(e) => handleChange('port', parseInt(e.target.value) || 3000)}
            />
          </div>
        </div>
        <div className="space-y-2">
          <Label htmlFor="buildPack">Build Pack</Label>
          <Select
            value={data.buildPack || 'auto'}
            onChange={(e) => handleChange('buildPack', e.target.value)}
          >
            <option value="auto">Auto Detect</option>
            <option value="nodejs">Node.js</option>
            <option value="nextjs">Next.js</option>
            <option value="go">Go</option>
            <option value="python">Python</option>
            <option value="rust">Rust</option>
            <option value="dockerfile">Dockerfile</option>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="replicas">Replicas</Label>
          <Input
            id="replicas"
            type="number"
            min={1}
            max={20}
            value={data.replicas || 1}
            onChange={(e) => handleChange('replicas', parseInt(e.target.value) || 1)}
          />
        </div>
        <Separator />
        <div className="space-y-2">
          <Label>Environment Variables</Label>
          <div className="space-y-2">
            {Object.entries(envVars).map(([key, value]) => (
              <div key={key} className="flex items-center gap-2">
                <Input value={key} className="flex-1" disabled />
                <Input value={value} className="flex-1" disabled />
                <Button variant="ghost" size="icon" onClick={() => handleRemoveEnvVar(key)}>
                  <X className="h-4 w-4" />
                </Button>
              </div>
            ))}
            <div className="flex items-center gap-2">
              <Input
                placeholder="KEY"
                value={newEnvKey}
                onChange={(e) => setNewEnvKey(e.target.value)}
                className="flex-1"
              />
              <Input
                placeholder="value"
                value={newEnvValue}
                onChange={(e) => setNewEnvValue(e.target.value)}
                className="flex-1"
              />
              <Button variant="ghost" size="icon" onClick={handleAddEnvVar}>
                <Plus className="h-4 w-4" />
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Use {`{db.DATABASE_URL}`} to reference database connections
            </p>
          </div>
        </div>
      </>
    );
  };

  const renderDatabaseConfig = () => {
    const data = selectedNode.data as Partial<DatabaseNodeData>;

    return (
      <>
        <div className="space-y-2">
          <Label htmlFor="engine">Database Engine</Label>
          <Select
            value={data.engine || 'postgres'}
            onChange={(e) => handleChange('engine', e.target.value)}
          >
            <option value="postgres">PostgreSQL</option>
            <option value="mysql">MySQL</option>
            <option value="mariadb">MariaDB</option>
            <option value="mongodb">MongoDB</option>
            <option value="redis">Redis</option>
          </Select>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-2">
            <Label htmlFor="version">Version</Label>
            <Select
              value={data.version || '16'}
              onChange={(e) => handleChange('version', e.target.value)}
            >
              <option value="16">16</option>
              <option value="15">15</option>
              <option value="14">14</option>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="sizeGB">Size (GB)</Label>
            <Input
              id="sizeGB"
              type="number"
              min={1}
              max={1000}
              value={data.sizeGB || 10}
              onChange={(e) => handleChange('sizeGB', parseInt(e.target.value) || 10)}
            />
          </div>
        </div>
      </>
    );
  };

  const renderDomainConfig = () => {
    const data = selectedNode.data as Partial<DomainNodeData>;

    return (
      <>
        <div className="space-y-2">
          <Label htmlFor="fqdn">Domain Name</Label>
          <Input
            id="fqdn"
            placeholder="app.example.com"
            value={data.fqdn || ''}
            onChange={(e) => handleChange('fqdn', e.target.value)}
          />
        </div>
        <div className="flex items-center justify-between">
          <Label htmlFor="sslEnabled">Enable SSL</Label>
          <Switch
            id="sslEnabled"
            checked={data.sslEnabled ?? true}
            onCheckedChange={(v) => handleChange('sslEnabled', v)}
          />
        </div>
      </>
    );
  };

  const renderVolumeConfig = () => {
    const data = selectedNode.data as Partial<VolumeNodeData>;

    return (
      <>
        <div className="space-y-2">
          <Label htmlFor="sizeGB">Size (GB)</Label>
          <Input
            id="sizeGB"
            type="number"
            min={1}
            max={1000}
            value={data.sizeGB || 10}
            onChange={(e) => handleChange('sizeGB', parseInt(e.target.value) || 10)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="mountPath">Mount Path</Label>
          <Input
            id="mountPath"
            placeholder="/data"
            value={data.mountPath || '/data'}
            onChange={(e) => handleChange('mountPath', e.target.value)}
          />
        </div>
      </>
    );
  };

  const renderWorkerConfig = () => {
    const data = selectedNode.data as Partial<WorkerNodeData>;

    return (
      <>
        <div className="space-y-2">
          <Label htmlFor="gitUrl">Git Repository</Label>
          <Input
            id="gitUrl"
            placeholder="https://github.com/user/repo"
            value={data.gitUrl || ''}
            onChange={(e) => handleChange('gitUrl', e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="branch">Branch</Label>
          <Input
            id="branch"
            placeholder="main"
            value={data.branch || 'main'}
            onChange={(e) => handleChange('branch', e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="command">Start Command</Label>
          <Input
            id="command"
            placeholder="npm run worker"
            value={data.command || ''}
            onChange={(e) => handleChange('command', e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="replicas">Instances</Label>
          <Input
            id="replicas"
            type="number"
            min={1}
            max={20}
            value={data.replicas || 1}
            onChange={(e) => handleChange('replicas', parseInt(e.target.value) || 1)}
          />
        </div>
      </>
    );
  };

  const configRenderers: Record<string, () => React.ReactNode> = {
    app: renderAppConfig,
    database: renderDatabaseConfig,
    domain: renderDomainConfig,
    volume: renderVolumeConfig,
    worker: renderWorkerConfig,
  };

  const nodeData = selectedNode.data as { name?: string };

  return (
    <div className="flex h-full w-72 flex-col border-l bg-card">
      {/* Header */}
      <div className="flex items-center justify-between border-b p-4">
        <div>
          <h3 className="text-sm font-semibold">{nodeData?.name || 'Component'}</h3>
          <p className="text-xs text-muted-foreground capitalize">{selectedNode.type}</p>
        </div>
        <Button variant="ghost" size="icon" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      {/* Content */}
      <div className="flex-1 space-y-4 overflow-y-auto p-4">
        <div className="space-y-2">
          <Label htmlFor="name">Name</Label>
          <Input
            id="name"
            value={String(nodeData?.name || '')}
            onChange={(e) => handleChange('name', e.target.value)}
          />
        </div>

        {configRenderers[selectedNode.type]?.()}
      </div>

      {/* Footer */}
      <div className="border-t p-4">
        <Button variant="destructive" className="w-full" onClick={handleDelete}>
          <Trash2 className="mr-2 h-4 w-4" />
          Delete Component
        </Button>
      </div>
    </div>
  );
}
