import { useState } from 'react';
import {
  GitBranch, Plus, ExternalLink, Link2, Link2Off,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { api } from '@/api/client';
import { useApi } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';

interface GitProvider {
  id: string;
  name: string;
  type: string;
  connected: boolean;
  repo_count: number;
  url?: string;
}

const providerMeta: Record<string, { label: string; desc: string; color: string }> = {
  github: { label: 'GitHub', desc: 'Connect via OAuth or personal token', color: 'bg-[#24292f] dark:bg-[#f0f6fc]' },
  gitlab: { label: 'GitLab', desc: 'GitLab.com or self-hosted', color: 'bg-[#fc6d26]' },
  gitea: { label: 'Gitea', desc: 'Self-hosted Gitea instance', color: 'bg-[#609926]' },
  bitbucket: { label: 'Bitbucket', desc: 'Bitbucket Cloud', color: 'bg-[#2684ff]' },
};

const providerAbbrev: Record<string, string> = {
  github: 'GH',
  gitlab: 'GL',
  gitea: 'GT',
  bitbucket: 'BB',
};

export function GitSources() {
  const { data: providers, loading, refetch } = useApi<GitProvider[]>('/git/providers');
  const [connectDialog, setConnectDialog] = useState(false);
  const [selectedType, setSelectedType] = useState<string | null>(null);
  const [token, setToken] = useState('');
  const [instanceUrl, setInstanceUrl] = useState('');

  const handleConnect = async () => {
    if (!selectedType || !token) return;
    await api.post('/git/providers', {
      type: selectedType,
      token,
      url: instanceUrl || undefined,
    });
    setToken('');
    setInstanceUrl('');
    setSelectedType(null);
    setConnectDialog(false);
    refetch();
  };

  const handleDisconnect = async (id: string) => {
    if (!confirm('Disconnect this Git provider?')) return;
    await api.delete(`/git/providers/${id}`);
    refetch();
  };

  const list = providers || [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Git Sources</h1>
          <p className="text-sm text-muted-foreground mt-1">Connect Git providers for auto-deploy</p>
        </div>
        <Button onClick={() => setConnectDialog(true)}>
          <Plus /> Connect Provider
        </Button>
      </div>

      {/* Connect Dialog */}
      <Dialog open={connectDialog} onOpenChange={setConnectDialog}>
        <DialogContent onClose={() => setConnectDialog(false)}>
          <DialogHeader>
            <DialogTitle>Connect Git Provider</DialogTitle>
            <DialogDescription>
              Select a provider and enter your access token to connect.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            {/* Provider Selection Grid */}
            {!selectedType && (
              <div className="grid grid-cols-2 gap-3">
                {Object.entries(providerMeta).map(([key, meta]) => (
                  <button
                    key={key}
                    onClick={() => setSelectedType(key)}
                    className={cn(
                      'flex flex-col items-center gap-2 rounded-lg border p-4 transition-colors hover:bg-accent hover:text-accent-foreground',
                    )}
                  >
                    <div className="flex size-12 items-center justify-center rounded-xl bg-muted font-bold text-foreground">
                      {providerAbbrev[key]}
                    </div>
                    <span className="text-sm font-medium">{meta.label}</span>
                    <span className="text-xs text-muted-foreground text-center">{meta.desc}</span>
                  </button>
                ))}
              </div>
            )}

            {/* Token form for selected provider */}
            {selectedType && (
              <>
                <div className="flex items-center gap-2">
                  <Button variant="ghost" size="sm" onClick={() => setSelectedType(null)}>
                    &larr; Back
                  </Button>
                  <span className="font-medium">
                    {providerMeta[selectedType]?.label}
                  </span>
                </div>
                {(selectedType === 'gitea' || selectedType === 'gitlab') && (
                  <div className="space-y-2">
                    <Label htmlFor="git-url">Instance URL</Label>
                    <Input
                      id="git-url"
                      value={instanceUrl}
                      onChange={(e) => setInstanceUrl(e.target.value)}
                      placeholder="https://git.example.com"
                    />
                  </div>
                )}
                <div className="space-y-2">
                  <Label htmlFor="git-token">Access Token</Label>
                  <Input
                    id="git-token"
                    type="password"
                    value={token}
                    onChange={(e) => setToken(e.target.value)}
                    placeholder="ghp_xxxxxxxxxxxx"
                  />
                </div>
              </>
            )}
          </div>
          {selectedType && (
            <DialogFooter>
              <Button variant="outline" onClick={() => { setSelectedType(null); setToken(''); }}>
                Back
              </Button>
              <Button onClick={handleConnect} disabled={!token}>
                Connect
              </Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>

      {/* Loading */}
      {loading && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {[1, 2, 3, 4].map((i) => (
            <Card key={i}>
              <CardContent className="space-y-3">
                <Skeleton className="size-12 rounded-xl" />
                <Skeleton className="h-5 w-24" />
                <Skeleton className="h-4 w-32" />
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* Empty State */}
      {!loading && list.length === 0 && (
        <Card className="py-16">
          <CardContent className="flex flex-col items-center text-center">
            <GitBranch className="mb-4 text-muted-foreground" size={48} />
            <h2 className="text-lg font-medium mb-2">No Git providers connected</h2>
            <p className="text-muted-foreground max-w-sm">
              Connect a provider to enable auto-deploy from push.
            </p>
          </CardContent>
        </Card>
      )}

      {/* Provider Cards */}
      {!loading && list.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {list.map((p) => {
            const meta = providerMeta[p.type] || providerMeta[p.id];
            const abbrev = providerAbbrev[p.type] || providerAbbrev[p.id] || p.name[0];

            return (
              <Card key={p.id}>
                <CardHeader className="items-center text-center">
                  <div className="flex size-14 items-center justify-center rounded-xl bg-muted font-bold text-lg">
                    {abbrev}
                  </div>
                  <CardTitle className="text-base">{p.name || meta?.label}</CardTitle>
                  <CardDescription>{p.type}</CardDescription>
                </CardHeader>
                <CardContent className="flex flex-col items-center gap-3">
                  {p.connected ? (
                    <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400">
                      <Link2 size={12} /> Connected
                    </Badge>
                  ) : (
                    <Badge variant="secondary">
                      <Link2Off size={12} /> Disconnected
                    </Badge>
                  )}
                  {p.repo_count > 0 && (
                    <span className="text-sm text-muted-foreground">
                      {p.repo_count} repo{p.repo_count !== 1 ? 's' : ''}
                    </span>
                  )}
                </CardContent>
                <CardFooter className="justify-center gap-2">
                  {p.connected ? (
                    <>
                      <Button variant="outline" size="sm">
                        <ExternalLink size={14} /> Browse Repos
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => handleDisconnect(p.id)}
                      >
                        Disconnect
                      </Button>
                    </>
                  ) : (
                    <Button size="sm" onClick={() => { setSelectedType(p.type || p.id); setConnectDialog(true); }}>
                      Connect
                    </Button>
                  )}
                </CardFooter>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
