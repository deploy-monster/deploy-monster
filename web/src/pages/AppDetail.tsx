import { useState, useCallback } from 'react';
import { useParams } from 'react-router';
import { Settings, Terminal, Layers, Rocket } from 'lucide-react';
import { appsAPI, type App } from '../api/apps';
import type { Deployment, EnvVar } from '@/api/deployments';
import { useApi } from '../hooks';
import type { PaginatedResponse } from '@/api/client';
import { toast } from '@/stores/toastStore';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { AlertDialog } from '@/components/ui/alert-dialog';
import { useAppSettingsState } from '@/hooks/useAppSettingsState';
import {
  AppHeader,
  AppOverviewSection,
  AppSettings,
  AppEnvVars,
  AppDeployments,
  AppLogs,
  type AppStatsResponse,
} from '@/components/AppDetail';
import type { ServerNode } from '@/api/servers';

/* ------------------------------------------------------------------ */
/*  Main Component                                                     */
/* ------------------------------------------------------------------ */

export function AppDetail() {
  const { id } = useParams();
  const { data: app, loading: appLoading, refetch: refetchApp } = useApi<App>(`/apps/${id}`);
  const { data: serversResp } = useApi<{ data: ServerNode[]; total: number } | ServerNode[]>('/servers');
  const servers = Array.isArray(serversResp) ? serversResp : serversResp?.data ?? [];
  const remoteServers = servers.filter((server) => server.id !== 'local' && server.provider !== 'local');
  const { data: deploymentsData } = useApi<PaginatedResponse<Deployment>>(`/apps/${id}/deployments`);
  const deployments = deploymentsData?.data ?? [];

  const { data: envData, refetch: refetchEnv } = useApi<{ data: { key: string; value: string }[] }>(
    `/apps/${id}/env`,
  );
  const envVars: EnvVar[] = (envData?.data ?? []).map((e) => ({
    key: e.key,
    value: e.value,
    isSecret: e.value.startsWith('${SECRET:'),
  }));

  const { data: stats, error: statsError } = useApi<AppStatsResponse>(
    `/apps/${id}/stats`,
    { refreshInterval: 10000 },
  );

  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  const settings = useAppSettingsState({ app, onRefetch: refetchApp });

  /* -- Actions ---------------------------------------------------- */

  const handleAction = useCallback(
    async (action: 'start' | 'stop' | 'restart') => {
      if (!id) return;
      setActionLoading(action);
      try {
        await appsAPI[action](id);
        refetchApp();
      } catch {
        toast.error(`Failed to ${action} application`);
      } finally {
        setActionLoading(null);
      }
    },
    [id, refetchApp],
  );

  const handleDelete = useCallback(async () => {
    if (!id) return;
    setActionLoading('delete');
    try {
      await appsAPI.delete(id);
      window.location.href = '/apps';
    } catch {
      toast.error('Failed to delete application');
      setActionLoading(null);
    }
  }, [id]);

  const handleDeleteRequested = useCallback(() => {
    setShowDeleteConfirm(true);
  }, []);

  const handleDeploy = useCallback(async () => {
    if (!id) return;
    setActionLoading('deploy');
    try {
      await appsAPI.deploy(id);
      toast.success('Deployment started');
      refetchApp();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to deploy application');
    } finally {
      setActionLoading(null);
    }
  }, [id, refetchApp]);

  /* -- Loading state ---------------------------------------------- */

  if (appLoading || !app) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="flex flex-col items-center gap-3">
          <div className="size-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
          <p className="text-sm text-muted-foreground">Loading application...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <AppHeader
        app={app}
        actionLoading={actionLoading}
        onAction={handleAction}
        onDeploy={handleDeploy}
        onDeleteRequested={handleDeleteRequested}
      />

      {/* Tabs */}
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview" className="cursor-pointer">
            <Layers className="size-4" />
            Overview
          </TabsTrigger>
          <TabsTrigger value="deployments" className="cursor-pointer">
            <Rocket className="size-4" />
            Deployments
          </TabsTrigger>
          <TabsTrigger value="env" className="cursor-pointer">
            <Settings className="size-4" />
            Environment
          </TabsTrigger>
          <TabsTrigger value="logs" className="cursor-pointer">
            <Terminal className="size-4" />
            Logs
          </TabsTrigger>
          <TabsTrigger value="settings" className="cursor-pointer">
            <Settings className="size-4" />
            Settings
          </TabsTrigger>
        </TabsList>

        {/* Overview Tab */}
        <TabsContent value="overview" className="space-y-6">
          <AppOverviewSection
            app={app}
            stats={stats ?? null}
            statsError={statsError}
            deployments={deployments}
            onDeploy={handleDeploy}
          />
        </TabsContent>

        {/* Deployments Tab */}
        <TabsContent value="deployments" className="space-y-4">
          <AppDeployments deployments={deployments} onDeploy={handleDeploy} />
        </TabsContent>

        {/* Environment Tab */}
        <TabsContent value="env" className="space-y-4">
          <AppEnvVars appId={id || ''} envVars={envVars} onRefetch={refetchEnv} />
        </TabsContent>

        {/* Logs Tab */}
        <TabsContent value="logs" className="space-y-4">
          <AppLogs appId={id || ''} />
        </TabsContent>

        {/* Settings Tab */}
        <TabsContent value="settings" className="space-y-6">
          <AppSettings
            app={app}
            remoteServers={remoteServers}
            onSave={settings.save}
            onDeleteRequested={handleDeleteRequested}
            settingsDirty={settings.dirty}
            settingsSaving={settings.saving}
            settingsName={settings.name}
            settingsBranch={settings.branch}
            settingsSourceURL={settings.sourceURL}
            settingsServerID={settings.serverID}
            onNameChange={settings.setNameDraft}
            onBranchChange={settings.setBranchDraft}
            onSourceURLChange={settings.setSourceURLDraft}
            onServerIDChange={settings.setServerIDDraft}
            onReset={settings.reset}
          />
        </TabsContent>
      </Tabs>

      {/* Delete Confirmation Dialog */}
      <AlertDialog
        open={showDeleteConfirm}
        onOpenChange={(open) => !open && setShowDeleteConfirm(false)}
        title="Delete Application"
        description="All deployments, domains, and data will be permanently removed."
        confirmLabel="Delete"
        cancelLabel="Cancel"
        variant="destructive"
        onConfirm={handleDelete}
      />
    </div>
  );
}