import { useState, type ReactNode } from 'react';
import { X, Trash2, Plus, Copy, Box, Container, HardDrive, Globe, Cog, Database as DatabaseIcon, Folder } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Separator } from '@/components/ui/separator';
import { Badge } from '@/components/ui/badge';
import { useTopologyStore } from '@/stores/topologyStore';
import type { AppNodeData, DatabaseNodeData, DomainNodeData, VolumeNodeData, WorkerNodeData, TopologyNodeType, VolumeMount } from '@/types/topology';

interface ConfigPanelProps {
  selectedNode: { id: string; type: string; data: Record<string, unknown> } | null;
  onClose: () => void;
}

// Node type configurations
const NODE_CONFIG: Record<TopologyNodeType, {
  icon: React.ReactNode;
  color: string;
  label: string;
  isContainer: boolean;
  inputs: string[];
  outputs: string[];
}> = {
  app: {
    icon: <Container className="h-4 w-4" />,
    color: 'blue',
    label: 'Application',
    isContainer: true,
    inputs: ['Git Repository', 'Branch', 'Build Pack', 'Port', 'Replicas'],
    outputs: ['URL', 'Port Mapping'],
  },
  database: {
    icon: <DatabaseIcon className="h-4 w-4" />,
    color: 'green',
    label: 'Database',
    isContainer: true,
    inputs: ['Engine', 'Version', 'Size'],
    outputs: ['Connection String', 'Host', 'Port', 'Credentials'],
  },
  domain: {
    icon: <Globe className="h-4 w-4" />,
    color: 'purple',
    label: 'Domain',
    isContainer: false,
    inputs: ['FQDN', 'SSL'],
    outputs: ['Routes to Container'],
  },
  volume: {
    icon: <HardDrive className="h-4 w-4" />,
    color: 'orange',
    label: 'Volume',
    isContainer: false,
    inputs: ['Size', 'Mount Path'],
    outputs: ['Persistent Storage'],
  },
  worker: {
    icon: <Cog className="h-4 w-4" />,
    color: 'yellow',
    label: 'Worker',
    isContainer: true,
    inputs: ['Git Repository', 'Branch', 'Command', 'Replicas'],
    outputs: ['Background Processing'],
  },
};

export function ConfigPanel({ selectedNode, onClose }: ConfigPanelProps) {
  const { updateNode, removeNode, nodes } = useTopologyStore();
  const [newEnvKey, setNewEnvKey] = useState('');
  const [newEnvValue, setNewEnvValue] = useState('');
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  if (!selectedNode) {
    return (
      <div className="flex h-full w-72 flex-col border-l bg-card">
        <div className="flex items-center justify-center p-8">
          <Box className="h-8 w-8 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">Select a component to configure</p>
        </div>
      </div>
    );
  }

  const config = NODE_CONFIG[selectedNode.type as TopologyNodeType];
  const nodeData = selectedNode.data as { name?: string };

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

  const copyToClipboard = (text: string, key: string) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  // Generate connection string for database
  const getConnectionString = () => {
    const data = selectedNode.data as Partial<DatabaseNodeData>;
    const name = nodeData?.name?.toLowerCase().replace(/[^a-z0-9]/g, '') || 'db';
    switch (data.engine) {
      case 'postgres':
        return `postgresql://postgres:postgres@${name}:5432/${name}`;
      case 'mysql':
      return `mysql://root:root@${name}:3306/${name}`;
      case 'mariadb':
        return `mariadb://root:root@${name}:3306/${name}`;
      case 'mongodb':
        return `mongodb://root:root@${name}:27017/${name}`;
      case 'redis':
        return `redis://${name}:6379`;
      default:
        return `${data.engine}://${name}`;
    }
  };


  const renderAppConfig = () => {
    const data = selectedNode.data as Partial<AppNodeData>;
    const envVars = (data.envVars as Record<string, string>) || {};

    // Get volume mounts for this container
    const volumeMounts = (data.volumeMounts as VolumeMount[]) || [];
    const mountedVolumeIds = volumeMounts.map((vm) => vm.volumeId);

    // Available volumes = all volumes not currently mounted to this container
    const availableVolumes = nodes.filter(
      (n) => n.type === 'volume' && !mountedVolumeIds.includes(n.id)
    );

    // Get mounted volume nodes with their mount paths
    const getMountedVolumeWithMount = () => {
      return volumeMounts.map((vm) => {
        const volNode = nodes.find((n) => n.id === vm.volumeId);
        const volData = volNode?.data as VolumeNodeData | undefined;
        return {
          ...vm,
          node: volNode,
          data: volData,
        };
      }).filter((vm) => vm.node);
    };

    const handleMountVolume = (volumeId: string) => {
      const volNode = nodes.find((n) => n.id === volumeId);
      const volData = volNode?.data as VolumeNodeData | undefined;
      const defaultPath = volData?.mountPath || '/data';

      const newMounts: VolumeMount[] = [
        ...volumeMounts,
        { volumeId, mountPath: defaultPath },
      ];
      handleChange('volumeMounts', newMounts);
    };

    const handleUnmountVolume = (volumeId: string) => {
      const newMounts = volumeMounts.filter((vm) => vm.volumeId !== volumeId);
      handleChange('volumeMounts', newMounts.length > 0 ? newMounts : undefined);
    };

    const handleUpdateMountPath = (volumeId: string, mountPath: string) => {
      const newMounts = volumeMounts.map((vm) =>
        vm.volumeId === volumeId ? { ...vm, mountPath } : vm
      );
      handleChange('volumeMounts', newMounts);
    };

    return (
      <>
        {/* Inputs Section */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <Box className="h-3 w-3" /> Inputs
          </h4>
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
        </div>

        <Separator />

        {/* Mounted Volumes Section */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <HardDrive className="h-3 w-3" /> Mounted Volumes
          </h4>
          <p className="text-xs text-muted-foreground">
            Persistent storage mounted inside this container
          </p>
          <div className="space-y-2">
            {getMountedVolumeWithMount().map(({ volumeId, mountPath, data: volData }) => (
              <div key={volumeId} className="rounded-md border border-orange-500/30 bg-orange-500/5 p-2 space-y-2">
                <div className="flex items-center gap-2">
                  <div className="flex h-6 w-6 items-center justify-center rounded bg-orange-500 text-white">
                    <HardDrive className="h-3 w-3" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="text-xs font-medium truncate">{volData?.name || 'Volume'}</div>
                    <div className="text-[10px] text-muted-foreground">
                      {volData?.sizeGB}GB
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-6 w-6"
                    onClick={() => handleUnmountVolume(volumeId)}
                  >
                    <X className="h-3 w-3" />
                  </Button>
                </div>
                <div className="flex items-center gap-1">
                  <Folder className="h-3 w-3 text-muted-foreground" />
                  <Input
                    value={mountPath}
                    onChange={(e) => handleUpdateMountPath(volumeId, e.target.value)}
                    placeholder="/data"
                    className="h-6 text-xs font-mono"
                  />
                </div>
              </div>
            ))}
            {availableVolumes.length > 0 && (
              <Select
                value=""
                onChange={(e) => e.target.value && handleMountVolume(e.target.value)}
              >
                <option value="">+ Mount a volume</option>
                {availableVolumes.map((vol) => {
                  const volData = vol.data as VolumeNodeData;
                  return (
                    <option key={vol.id} value={vol.id}>
                      {volData.name} ({volData.sizeGB}GB)
                    </option>
                  );
                })}
              </Select>
            )}
            {availableVolumes.length === 0 && volumeMounts.length === 0 && (
              <p className="text-xs text-muted-foreground italic">
                No volumes available. Drag a Volume from the palette.
              </p>
            )}
          </div>
        </div>

        <Separator />

        {/* Environment Variables with Reference Support */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <Box className="h-3 w-3" /> Environment Variables
          </h4>
          <p className="text-xs text-muted-foreground">
            Use <code className="text-primary">{'${db.DATABASE_URL}'}</code> to auto-link database connections
          </p>
          <div className="space-y-2">
            {Object.entries(envVars).map(([key, value]) => {
              const isRef = value.includes('${');
              return (
                <div key={key} className="flex items-center gap-2">
                  <Input value={key} className="flex-1 font-mono text-xs" disabled />
                  <Input
                    value={value}
                    className={`flex-1 font-mono text-xs ${isRef ? 'text-primary' : ''}`}
                    disabled
                  />
                  {isRef && (
                    <Badge variant="outline" className="text-xs">linked</Badge>
                  )}
                  <Button variant="ghost" size="icon" onClick={() => handleRemoveEnvVar(key)}>
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              );
            })}
            <div className="flex items-center gap-2">
              <Input
                placeholder="KEY"
                value={newEnvKey}
                onChange={(e) => setNewEnvKey(e.target.value)}
                className="flex-1 font-mono text-xs"
              />
              <Input
                placeholder="value or ${db.DATABASE_URL}"
                value={newEnvValue}
                onChange={(e) => setNewEnvValue(e.target.value)}
                className="flex-1 font-mono text-xs"
              />
              <Button variant="ghost" size="icon" onClick={handleAddEnvVar}>
                <Plus className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>

        <Separator />

        {/* Outputs Section */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <Box className="h-3 w-3" /> Outputs
          </h4>
          <div className="rounded-lg bg-muted/50 p-3 space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">URL:</span>
              <code className="text-primary">https://{nodeData?.name || 'app'}.deploy.monster</code>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Port:</span>
              <code className="text-primary">:{data.port || 3000} → container</code>
            </div>
          </div>
        </div>
      </>
    );
  };

  const renderDatabaseConfig = () => {
    const data = selectedNode.data as Partial<DatabaseNodeData>;
    const connectionString = getConnectionString();

    return (
      <>
        {/* Inputs Section */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <Box className="h-3 w-3" /> Inputs
          </h4>
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
                <option value="17">17</option>
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
        </div>

        <Separator />

        {/* Outputs Section */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <Box className="h-3 w-3" /> Outputs
          </h4>
          <div className="rounded-lg bg-muted/50 p-3 space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">Connection String:</span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => copyToClipboard(connectionString, 'connection')}
                className="h-7"
              >
                {copiedKey === 'connection' ? (
                  <span className="text-xs text-green-500">Copied!</span>
                ) : (
                  <Copy className="h-3 w-3" />
                )}
              </Button>
            </div>
            <code className="block text-xs bg-background p-2 rounded break-all">
              {connectionString}
            </code>
          </div>
          <p className="text-xs text-muted-foreground">
            Apps can reference this with <code className="text-primary">{'${name}.CONNECTION_STRING}'}</code>
          </p>
        </div>
      </>
    );
  };

  const renderDomainConfig = () => {
    const data = selectedNode.data as Partial<DomainNodeData>;

    return (
      <>
        {/* Inputs Section */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <Box className="h-3 w-3" /> Inputs
          </h4>
          <div className="space-y-2">
            <Label htmlFor="fqdn">Domain Name</Label>
            <Input
              id="fqdn"
              placeholder="app.example.com"
              value={data.fqdn || ''}
              onChange={(e) => handleChange('fqdn', e.target.value)}
            />
          </div>
          <div className="flex items-center justify-between rounded-lg bg-muted/50 p-3">
            <div>
              <Label htmlFor="sslEnabled" className="font-normal">Enable SSL</Label>
              <p className="text-xs text-muted-foreground">Automatic Let's Encrypt certificate</p>
            </div>
            <Switch
              id="sslEnabled"
              checked={data.sslEnabled ?? true}
              onCheckedChange={(v) => handleChange('sslEnabled', v)}
            />
          </div>
        </div>

        <Separator />

        {/* Outputs Section */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <Box className="h-3 w-3" /> Outputs
          </h4>
          <div className="rounded-lg bg-muted/50 p-3">
            <p className="text-sm text-muted-foreground">
              Routes traffic to the linked container via HTTPS
            </p>
            <div className="flex items-center gap-2 mt-2">
              <Badge variant="outline">HTTPS</Badge>
              {data.sslEnabled && <Badge variant="outline">SSL</Badge>}
            </div>
          </div>
        </div>
      </>
    );
  };

  const renderVolumeConfig = () => {
    const data = selectedNode.data as Partial<VolumeNodeData>;

    return (
      <>
        {/* Inputs Section */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <Box className="h-3 w-3" /> Inputs
          </h4>
          <div className="grid grid-cols-2 gap-3">
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
          </div>
        </div>

        <Separator />

        {/* Outputs Section */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <Box className="h-3 w-3" /> Outputs
          </h4>
          <div className="rounded-lg bg-muted/50 p-3">
            <p className="text-sm text-muted-foreground">
              Persistent storage mounted to containers at <code>{data.mountPath || '/data'}</code>
            </p>
            <div className="flex items-center gap-2 mt-2">
              <Badge variant="outline">{data.sizeGB || 10} GB</Badge>
            </div>
          </div>
        </div>
      </>
    );
  };

  const renderWorkerConfig = () => {
    const data = selectedNode.data as Partial<WorkerNodeData>;

    return (
      <>
        {/* Inputs Section */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <Box className="h-3 w-3" /> Inputs
          </h4>
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
        </div>

        <Separator />

        {/* Outputs Section */}
        <div className="space-y-3">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-1">
            <Box className="h-3 w-3" /> Outputs
          </h4>
          <div className="rounded-lg bg-muted/50 p-3">
            <p className="text-sm text-muted-foreground">
              Background worker process (no HTTP endpoint)
            </p>
            <div className="flex items-center gap-2 mt-2">
              <Badge variant="outline">{data.replicas || 1} instance(s)</Badge>
            </div>
          </div>
        </div>
      </>
    );
  };

  const configRenderers: Record<string, () => ReactNode> = {
    app: renderAppConfig,
    database: renderDatabaseConfig,
    domain: renderDomainConfig,
    volume: renderVolumeConfig,
    worker: renderWorkerConfig,
  };

  return (
    <div className="flex h-full w-72 flex-col border-l bg-card">
      {/* Header */}
      <div className="flex items-center justify-between border-b p-4">
        <div className="flex items-center gap-3">
          <div className={`p-2 rounded-lg bg-${config.color}-500/10`}>
            {config.icon}
          </div>
          <div>
            <h3 className="text-sm font-semibold">{nodeData?.name || 'Component'}</h3>
            <div className="flex items-center gap-2">
              <p className="text-xs text-muted-foreground capitalize">{selectedNode.type}</p>
              {config.isContainer && (
                <Badge variant="outline" className="text-xs">Container</Badge>
              )}
            </div>
          </div>
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
