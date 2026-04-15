import { useState } from 'react';
import {
  GitBranch,
  Plus,
  ExternalLink,
  Link2,
  Link2Off,
  Loader2,
  AlertCircle,
  ArrowLeft,
  Eye,
  EyeOff,
  GitPullRequest,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { gitSourcesAPI, type GitProvider } from '@/api/git-sources';
import { useApi } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import {
  Sheet, SheetContent, SheetHeader, SheetFooter, SheetTitle, SheetDescription, SheetBody,
} from '@/components/ui/sheet';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from '@/stores/toastStore';
import { AlertDialog } from '@/components/ui/alert-dialog';

// ---------------------------------------------------------------------------
// Provider configuration with colors
// ---------------------------------------------------------------------------

interface ProviderMeta {
  label: string;
  desc: string;
  bgColor: string;
  textColor: string;
  letter: string;
  badgeColor: string;
}

const providerMeta: Record<string, ProviderMeta> = {
  github: {
    label: 'GitHub',
    desc: 'Connect via OAuth or personal token',
    bgColor: 'bg-[#24292f] dark:bg-[#f0f6fc]',
    textColor: 'text-white dark:text-[#24292f]',
    letter: 'GH',
    badgeColor: 'bg-[#24292f]/10 text-[#24292f] dark:bg-[#f0f6fc]/10 dark:text-[#f0f6fc]',
  },
  gitlab: {
    label: 'GitLab',
    desc: 'GitLab.com or self-hosted',
    bgColor: 'bg-[#fc6d26]',
    textColor: 'text-white',
    letter: 'GL',
    badgeColor: 'bg-orange-500/10 text-orange-600 border-orange-500/20 dark:text-orange-400',
  },
  gitea: {
    label: 'Gitea',
    desc: 'Self-hosted Gitea instance',
    bgColor: 'bg-[#609926]',
    textColor: 'text-white',
    letter: 'GT',
    badgeColor: 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400',
  },
  bitbucket: {
    label: 'Bitbucket',
    desc: 'Bitbucket Cloud',
    bgColor: 'bg-[#2684ff]',
    textColor: 'text-white',
    letter: 'BB',
    badgeColor: 'bg-blue-500/10 text-blue-600 border-blue-500/20 dark:text-blue-400',
  },
};

function getMeta(type: string): ProviderMeta {
  return providerMeta[type] || providerMeta.github;
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function CardSkeleton() {
  return (
    <Card className="py-5">
      <CardHeader className="items-center text-center pb-0">
        <Skeleton className="size-14 rounded-xl" />
        <Skeleton className="h-5 w-20 mt-3" />
        <Skeleton className="h-3.5 w-28 mt-1" />
      </CardHeader>
      <CardContent className="flex flex-col items-center gap-2 pt-0 mt-4">
        <Skeleton className="h-5 w-24 rounded-md" />
      </CardContent>
      <CardFooter className="border-t pt-4 pb-0 justify-center">
        <Skeleton className="h-8 w-28 rounded-md" />
      </CardFooter>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// GitSources
// ---------------------------------------------------------------------------

export function GitSources() {
  const { data: providers, loading, refetch } = useApi<GitProvider[]>('/git/providers');
  const [connectDialog, setConnectDialog] = useState(false);
  const [selectedType, setSelectedType] = useState<string | null>(null);
  const [token, setToken] = useState('');
  const [tokenVisible, setTokenVisible] = useState(false);
  const [instanceUrl, setInstanceUrl] = useState('');
  const [connecting, setConnecting] = useState(false);
  const [connectError, setConnectError] = useState('');

  const handleConnect = async () => {
    if (!selectedType || !token) return;
    setConnecting(true);
    setConnectError('');
    try {
      await gitSourcesAPI.connect({
        type: selectedType,
        token,
        url: instanceUrl || undefined,
      });
      toast.success(`${getMeta(selectedType).label} connected successfully`);
      setToken('');
      setTokenVisible(false);
      setInstanceUrl('');
      setSelectedType(null);
      setConnectDialog(false);
      refetch();
    } catch (err) {
      setConnectError(err instanceof Error ? err.message : 'Failed to connect provider');
    } finally {
      setConnecting(false);
    }
  };

  const handleDisconnect = async (id: string) => {
    setDisconnectId(id);
  };

  const closeDialog = () => {
    setConnectDialog(false);
    setSelectedType(null);
    setToken('');
    setTokenVisible(false);
    setInstanceUrl('');
    setConnectError('');
    setConnecting(false);
  };

  const list = providers || [];
  const connectedCount = list.filter((p) => p.connected).length;
  const [disconnectId, setDisconnectId] = useState<string | null>(null);
  const pendingDisconnect = list.find((p) => p.id === disconnectId);

  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 mb-2">
              <GitBranch className="size-5 text-primary" />
              <Badge variant="secondary" className="text-xs font-normal">
                {list.length} provider{list.length !== 1 ? 's' : ''}
                {connectedCount > 0 && ` \u00b7 ${connectedCount} connected`}
              </Badge>
            </div>
            <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
              Git Sources
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm sm:text-base max-w-lg">
              Connect Git providers to enable automatic deployments on push.
            </p>
          </div>
          <Button onClick={() => setConnectDialog(true)} className="shrink-0">
            <Plus className="size-4" />
            Connect Provider
          </Button>
        </div>
        {/* Decorative */}
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
        <div className="pointer-events-none absolute -left-8 -bottom-8 size-48 rounded-full bg-primary/3 blur-2xl" />
      </div>

      {/* Connect Sheet */}
      <Sheet open={connectDialog} onOpenChange={(open) => !open && closeDialog()}>
        <SheetContent onClose={closeDialog}>
          <SheetHeader>
            <SheetTitle className="flex items-center gap-3">
              {selectedType ? (
                <div className={cn(
                  'flex items-center justify-center rounded-xl size-9',
                  getMeta(selectedType).bgColor
                )}>
                  <span className={cn('text-sm font-bold', getMeta(selectedType).textColor)}>
                    {getMeta(selectedType).letter}
                  </span>
                </div>
              ) : (
                <div className="flex items-center justify-center rounded-xl size-9 bg-primary">
                  <GitBranch className="size-4 text-primary-foreground" />
                </div>
              )}
              {selectedType
                ? `Connect ${getMeta(selectedType).label}`
                : 'Connect Git Provider'
              }
            </SheetTitle>
            <SheetDescription>
              {selectedType
                ? `Enter your ${getMeta(selectedType).label} access token to connect.`
                : 'Select a provider to connect for automatic deployments.'
              }
            </SheetDescription>
          </SheetHeader>

          <SheetBody>
            <div className="space-y-4">
              {/* Provider Selection Grid */}
              {!selectedType && (
                <div className="grid grid-cols-2 gap-3">
                  {Object.entries(providerMeta).map(([key, meta]) => (
                    <button
                      key={key}
                      type="button"
                      onClick={() => setSelectedType(key)}
                      className="flex flex-col items-center gap-2.5 rounded-lg border p-5 transition-all duration-200 hover:bg-accent hover:text-accent-foreground hover:translate-y-[-1px] hover:shadow-md cursor-pointer"
                    >
                      <div className={cn(
                        'flex items-center justify-center rounded-xl size-12',
                        meta.bgColor
                      )}>
                        <span className={cn('text-base font-bold', meta.textColor)}>
                          {meta.letter}
                        </span>
                      </div>
                      <span className="text-sm font-medium">{meta.label}</span>
                      <span className="text-xs text-muted-foreground text-center leading-relaxed">
                        {meta.desc}
                      </span>
                    </button>
                  ))}
                </div>
              )}

              {/* Token form for selected provider */}
              {selectedType && (
                <>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => { setSelectedType(null); setToken(''); setConnectError(''); }}
                    className="gap-1 -ml-2"
                  >
                    <ArrowLeft className="size-3.5" />
                    Back to providers
                  </Button>

                  {(selectedType === 'gitea' || selectedType === 'gitlab') && (
                    <div className="space-y-1.5">
                      <Label htmlFor="git-url">Instance URL</Label>
                      <div className="relative">
                        <GitBranch className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                        <Input
                          id="git-url"
                          value={instanceUrl}
                          onChange={(e) => setInstanceUrl(e.target.value)}
                          placeholder="https://git.example.com"
                          className="pl-9"
                        />
                      </div>
                    </div>
                  )}
                  <div className="space-y-1.5">
                    <Label htmlFor="git-token">Access Token</Label>
                    <div className="relative">
                      <Input
                        id="git-token"
                        type={tokenVisible ? 'text' : 'password'}
                        value={token}
                        onChange={(e) => setToken(e.target.value)}
                        placeholder="ghp_xxxxxxxxxxxx"
                        className="pr-10 font-mono"
                      />
                      <button
                        type="button"
                        onClick={() => setTokenVisible(!tokenVisible)}
                        className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                        tabIndex={-1}
                      >
                        {tokenVisible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                      </button>
                    </div>
                    <p className="text-[11px] text-muted-foreground">
                      Token needs read access to repositories and webhooks.
                    </p>
                  </div>

                  {connectError && (
                    <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2.5 text-sm text-destructive">
                      <AlertCircle className="size-4 shrink-0" />
                      {connectError}
                    </div>
                  )}
                </>
              )}
            </div>
          </SheetBody>

          {selectedType && (
            <SheetFooter>
              <Button variant="outline" onClick={closeDialog} disabled={connecting}>
                Cancel
              </Button>
              <Button onClick={handleConnect} disabled={!token || connecting}>
                {connecting ? (
                  <>
                    <Loader2 className="size-4 animate-spin" />
                    Connecting...
                  </>
                ) : (
                  <>
                    <Link2 className="size-4" />
                    Connect
                  </>
                )}
              </Button>
            </SheetFooter>
          )}
        </SheetContent>
      </Sheet>

      {/* Loading */}
      {loading && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <CardSkeleton key={i} />
          ))}
        </div>
      )}

      {/* Empty State */}
      {!loading && list.length === 0 && (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="rounded-full bg-muted p-6 mb-5">
            <GitBranch className="size-10 text-muted-foreground" />
          </div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">
            No Git providers connected
          </h2>
          <p className="text-muted-foreground max-w-sm text-sm mb-6">
            Connect a Git provider to enable automatic deployments from push events. Supports GitHub, GitLab, Gitea, and Bitbucket.
          </p>
          <Button onClick={() => setConnectDialog(true)}>
            <Plus className="size-4" />
            Connect your first provider
          </Button>
        </div>
      )}

      {/* Provider Cards */}
      {!loading && list.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {list.map((p) => {
            const meta = getMeta(p.type || p.id);

            return (
              <Card
                key={p.id}
                className="group transition-all duration-200 hover:translate-y-[-2px] hover:shadow-lg"
              >
                <CardHeader className="items-center text-center gap-3">
                  <div className={cn(
                    'flex items-center justify-center rounded-xl size-14',
                    meta.bgColor
                  )}>
                    <span className={cn('text-lg font-bold', meta.textColor)}>
                      {meta.letter}
                    </span>
                  </div>
                  <div>
                    <CardTitle className="text-base">{p.name || meta.label}</CardTitle>
                    <CardDescription className="mt-0.5">{meta.label}</CardDescription>
                  </div>
                </CardHeader>
                <CardContent className="flex flex-col items-center gap-3">
                  {p.connected ? (
                    <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400 gap-1.5">
                      <Link2 className="size-3" />
                      Connected
                    </Badge>
                  ) : (
                    <Badge variant="secondary" className="gap-1.5">
                      <Link2Off className="size-3" />
                      Disconnected
                    </Badge>
                  )}
                  {p.repo_count > 0 && (
                    <Badge variant="outline" className="gap-1 text-xs font-normal">
                      <GitPullRequest className="size-3" />
                      {p.repo_count} repo{p.repo_count !== 1 ? 's' : ''}
                    </Badge>
                  )}
                </CardContent>
                <CardFooter className="border-t pt-4 pb-0 justify-center gap-2">
                  {p.connected ? (
                    <>
                      <Button variant="outline" size="sm" className="gap-1.5">
                        <ExternalLink className="size-3.5" />
                        Repos
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive hover:bg-destructive/10"
                        onClick={() => handleDisconnect(p.id)}
                      >
                        Disconnect
                      </Button>
                    </>
                  ) : (
                    <Button
                      size="sm"
                      onClick={() => { setSelectedType(p.type || p.id); setConnectDialog(true); }}
                      className="gap-1.5"
                    >
                      <Link2 className="size-3.5" />
                      Connect
                    </Button>
                  )}
                </CardFooter>
              </Card>
            );
          })}
        </div>
      )}

      {/* Disconnect Confirmation Dialog */}
      <AlertDialog
        open={disconnectId !== null}
        onOpenChange={(open) => !open && setDisconnectId(null)}
        title="Disconnect Provider"
        description={`Disconnect "${pendingDisconnect?.name || pendingDisconnect?.type}"? Connected repositories will no longer trigger automatic deployments.`}
        confirmLabel="Disconnect"
        cancelLabel="Cancel"
        variant="destructive"
        onConfirm={async () => {
          if (!disconnectId) return;
          try {
            await gitSourcesAPI.disconnect(disconnectId);
            toast.success('Provider disconnected');
            refetch();
          } catch {
            toast.error('Failed to disconnect provider');
          } finally {
            setDisconnectId(null);
          }
        }}
      />
    </div>
  );
}
