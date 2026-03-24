import { useState } from 'react';
import {
  Database, Plus, Copy, Check, CircleDot, Square,
} from 'lucide-react';
import { api } from '@/api/client';
import { useApi } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Select } from '@/components/ui/select';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';

interface DatabaseInstance {
  id: string;
  name: string;
  engine: string;
  version: string;
  status: string;
  connection_string: string;
  size_mb: number;
  created_at: string;
}

const engines = [
  { id: 'postgres', name: 'PostgreSQL', versions: ['17', '16', '15'], icon: Database },
  { id: 'mysql', name: 'MySQL', versions: ['8.4', '8.0'], icon: Database },
  { id: 'mariadb', name: 'MariaDB', versions: ['11', '10.11'], icon: Database },
  { id: 'redis', name: 'Redis', versions: ['7'], icon: Database },
  { id: 'mongodb', name: 'MongoDB', versions: ['7'], icon: Database },
];

function StatusBadge({ status }: { status: string }) {
  switch (status) {
    case 'running':
      return (
        <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400">
          <CircleDot size={12} /> Running
        </Badge>
      );
    case 'stopped':
      return (
        <Badge variant="secondary">
          <Square size={12} /> Stopped
        </Badge>
      );
    default:
      return <Badge variant="outline">{status}</Badge>;
  }
}

export function Databases() {
  const { data: databases, loading, refetch } = useApi<DatabaseInstance[]>('/databases');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [newName, setNewName] = useState('');
  const [engine, setEngine] = useState('postgres');
  const [version, setVersion] = useState('');
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const selectedEngine = engines.find((e) => e.id === engine);

  const handleCreate = async () => {
    if (!newName || !engine) return;
    await api.post('/databases', {
      name: newName,
      engine,
      version: version || selectedEngine?.versions[0],
    });
    setNewName('');
    setEngine('postgres');
    setVersion('');
    setDialogOpen(false);
    refetch();
  };

  const handleCopy = (id: string, connStr: string) => {
    navigator.clipboard.writeText(connStr);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const list = databases || [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Databases</h1>
          <p className="text-sm text-muted-foreground mt-1">Managed database instances</p>
        </div>
        <Button onClick={() => setDialogOpen(true)}>
          <Plus /> Create Database
        </Button>
      </div>

      {/* Create Database Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent onClose={() => setDialogOpen(false)}>
          <DialogHeader>
            <DialogTitle>Create Database</DialogTitle>
            <DialogDescription>
              Provision a new managed database instance.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="db-name">Database Name</Label>
              <Input
                id="db-name"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                placeholder="my-database"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="db-engine">Engine</Label>
              <Select
                id="db-engine"
                value={engine}
                onChange={(e) => {
                  setEngine(e.target.value);
                  setVersion('');
                }}
              >
                {engines.map((e) => (
                  <option key={e.id} value={e.id}>{e.name}</option>
                ))}
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="db-version">Version</Label>
              <Select
                id="db-version"
                value={version || (selectedEngine?.versions[0] ?? '')}
                onChange={(e) => setVersion(e.target.value)}
              >
                {(selectedEngine?.versions || []).map((v) => (
                  <option key={v} value={v}>v{v}</option>
                ))}
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleCreate} disabled={!newName}>Create</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Loading */}
      {loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <Card key={i}>
              <CardContent className="space-y-3">
                <Skeleton className="h-6 w-32" />
                <Skeleton className="h-4 w-24" />
                <Skeleton className="h-8 w-full" />
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* Empty State */}
      {!loading && list.length === 0 && (
        <Card className="py-16">
          <CardContent className="flex flex-col items-center text-center">
            <Database className="mb-4 text-muted-foreground" size={48} />
            <h2 className="text-lg font-medium mb-2">No databases yet</h2>
            <p className="text-muted-foreground max-w-sm">
              Create a managed database to get started.
            </p>
          </CardContent>
        </Card>
      )}

      {/* Database Grid */}
      {!loading && list.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {list.map((db) => {
            const engineInfo = engines.find((e) => e.id === db.engine);
            const EngineIcon = engineInfo?.icon || Database;

            return (
              <Card key={db.id}>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className="flex size-10 items-center justify-center rounded-lg bg-primary/10">
                        <EngineIcon size={20} className="text-primary" />
                      </div>
                      <div>
                        <CardTitle className="text-base">{db.name}</CardTitle>
                        <CardDescription>
                          {engineInfo?.name || db.engine} v{db.version}
                        </CardDescription>
                      </div>
                    </div>
                    <StatusBadge status={db.status} />
                  </div>
                </CardHeader>
                <CardContent>
                  {db.connection_string && (
                    <div className="space-y-1.5">
                      <Label className="text-xs text-muted-foreground">Connection String</Label>
                      <div className="flex items-center gap-2">
                        <code className="flex-1 truncate rounded-md border bg-muted/50 px-3 py-1.5 font-mono text-xs">
                          {db.connection_string}
                        </code>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="shrink-0"
                          onClick={() => handleCopy(db.id, db.connection_string)}
                        >
                          {copiedId === db.id ? (
                            <Check size={14} className="text-emerald-500" />
                          ) : (
                            <Copy size={14} />
                          )}
                        </Button>
                      </div>
                    </div>
                  )}
                </CardContent>
                <CardFooter className="text-xs text-muted-foreground justify-between">
                  <span>{db.size_mb ? `${db.size_mb} MB` : '--'}</span>
                  <span>Created {new Date(db.created_at).toLocaleDateString()}</span>
                </CardFooter>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
