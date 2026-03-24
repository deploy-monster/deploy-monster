import { useState } from 'react';
import {
  Lock, Plus, Eye, EyeOff, Trash2, Copy, Check, ShieldCheck,
} from 'lucide-react';
import { api } from '@/api/client';
import { useApi } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
import { Select } from '@/components/ui/select';
import {
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
} from '@/components/ui/table';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from '@/components/Toast';
interface SecretEntry {
  id: string;
  name: string;
  scope: string;
  created_at: string;
  updated_at: string;
}
function ScopeBadge({ scope }: { scope: string }) {
  switch (scope) {
    case 'global':
      return <Badge>Global</Badge>;
    case 'tenant':
      return <Badge variant="secondary">Tenant</Badge>;
    case 'project':
      return <Badge variant="outline">Project</Badge>;
    case 'app':
      return <Badge variant="outline">App</Badge>;
    default:
      return <Badge variant="outline">{scope}</Badge>;
  }
}
export function Secrets() {
  const { data: secrets, loading, refetch } = useApi<SecretEntry[]>('/secrets');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [name, setName] = useState('');
  const [value, setValue] = useState('');
  const [scope, setScope] = useState('tenant');
  const [showValue, setShowValue] = useState(false);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const handleCreate = async () => {
    if (!name || !value) return;
    try {
      await api.post('/secrets', { name, value, scope });
      toast.success('Secret created');
      setName('');
      setValue('');
      setScope('tenant');
      setDialogOpen(false);
      refetch();
    } catch {
      toast.error('Failed to create secret');
    }
  };
  const handleDelete = async (id: string) => {
    if (!confirm('Delete this secret?')) return;
    try {
      await api.delete(`/secrets/${id}`);
      toast.success('Secret deleted');
      refetch();
    } catch {
      toast.error('Failed to delete secret');
    }
  };
  const handleCopyRef = (secretName: string, id: string) => {
    navigator.clipboard.writeText(`\${SECRET:${secretName}}`);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };
  const list = secrets || [];
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Secrets</h1>
          <p className="text-sm text-muted-foreground mt-1">Encrypted secret storage with AES-256-GCM</p>
        </div>
        <Button onClick={() => setDialogOpen(true)}>
          <Plus /> Add Secret
        </Button>
      </div>
      {/* Add Secret Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent onClose={() => setDialogOpen(false)}>
          <DialogHeader>
            <DialogTitle>Create Secret</DialogTitle>
            <DialogDescription>
              Secrets are encrypted with AES-256-GCM. Values are never returned by the API.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="secret-name">Name</Label>
              <Input
                id="secret-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="DB_PASSWORD"
                className="font-mono"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="secret-value">Value</Label>
              <div className="relative">
                <Input
                  id="secret-value"
                  type={showValue ? 'text' : 'password'}
                  value={value}
                  onChange={(e) => setValue(e.target.value)}
                  placeholder="secret value"
                  className="pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowValue(!showValue)}
                  className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  {showValue ? <EyeOff size={16} /> : <Eye size={16} />}
                </button>
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="secret-scope">Scope</Label>
              <Select
                id="secret-scope"
                value={scope}
                onChange={(e) => setScope(e.target.value)}
              >
                <option value="global">Global</option>
                <option value="tenant">Tenant</option>
                <option value="project">Project</option>
                <option value="app">App</option>
              </Select>
            </div>
            <Card className="bg-muted/50 py-3">
              <CardContent className="text-sm text-muted-foreground">
                Reference in env vars:{' '}
                <code className="rounded bg-background border px-1.5 py-0.5 font-mono text-xs text-foreground">
                  {'${'}SECRET:{name || 'name'}{'}'}
                </code>
              </CardContent>
            </Card>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleCreate} disabled={!name || !value}>Create Secret</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {/* Loading */}
      {loading && (
        <Card>
          <CardContent className="space-y-3 py-2">
            {[1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </CardContent>
        </Card>
      )}
      {/* Empty State */}
      {!loading && list.length === 0 && (
        <Card className="py-16">
          <CardContent className="flex flex-col items-center text-center">
            <Lock className="mb-4 text-muted-foreground" size={48} />
            <h2 className="text-lg font-medium mb-2">Secret Vault</h2>
            <p className="text-muted-foreground max-w-sm mb-2">
              Secrets are encrypted with AES-256-GCM and stored securely.
            </p>
            <p className="text-sm text-muted-foreground">
              Values are never returned by the API -- only names and metadata.
            </p>
          </CardContent>
        </Card>
      )}
      {/* Secrets Table */}
      {!loading && list.length > 0 && (
        <Card className="py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Scope</TableHead>
                <TableHead>Value</TableHead>
                <TableHead>Updated</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {list.map((s) => (
                <TableRow key={s.id}>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <ShieldCheck size={16} className="text-primary shrink-0" />
                      <span className="font-mono font-medium">{s.name}</span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <ScopeBadge scope={s.scope} />
                  </TableCell>
                  <TableCell>
                    <code className="text-muted-foreground text-xs font-mono">
                      ••••••••••••
                    </code>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {new Date(s.updated_at || s.created_at).toLocaleDateString()}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleCopyRef(s.name, s.id)}
                        title="Copy reference"
                      >
                        {copiedId === s.id ? (
                          <Check size={14} className="text-emerald-500" />
                        ) : (
                          <Copy size={14} />
                        )}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleDelete(s.id)}
                        className="text-muted-foreground hover:text-destructive"
                        title="Delete secret"
                      >
                        <Trash2 size={14} />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </div>
  );
}
