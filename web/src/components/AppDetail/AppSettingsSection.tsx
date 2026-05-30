import { Trash2, Settings } from 'lucide-react';
import { type App } from '@/api/apps';
import type { ServerNode } from '@/api/servers';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';

interface AppSettingsProps {
  app: App;
  remoteServers: ServerNode[];
  onSave: () => void;
  onDeleteRequested: () => void;
  settingsDirty: boolean;
  settingsSaving: boolean;
  settingsName: string;
  settingsBranch: string;
  settingsSourceURL: string;
  settingsServerID: string;
  onNameChange: (v: string) => void;
  onBranchChange: (v: string) => void;
  onSourceURLChange: (v: string) => void;
  onServerIDChange: (v: string) => void;
  onReset: () => void;
}

export function AppSettings({
  app,
  remoteServers,
  onSave,
  onDeleteRequested,
  settingsDirty,
  settingsSaving,
  settingsName,
  settingsBranch,
  settingsSourceURL,
  settingsServerID,
  onNameChange,
  onBranchChange,
  onSourceURLChange,
  onServerIDChange,
  onReset,
}: AppSettingsProps) {
  return (
    <div className="space-y-6">
      <div className="rounded-lg border bg-card p-6">
        <h2 className="text-base font-semibold mb-1">General</h2>
        <p className="text-sm text-muted-foreground mb-4">
          Update application name, source URL, and branch
        </p>
        <div className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <Label htmlFor="settings-name" className="mb-1.5">Application Name</Label>
              <Input
                id="settings-name"
                value={settingsName}
                onChange={(e) => onNameChange(e.target.value)}
              />
            </div>
            <div>
              <Label className="mb-1.5">Application ID</Label>
              <div className="flex gap-2">
                <Input value={app.id} readOnly className="bg-muted font-mono text-xs" />
                <Button
                  variant="outline"
                  size="icon"
                  aria-label="Copy application ID"
                  className="cursor-pointer"
                  onClick={() => navigator.clipboard.writeText(app.id)}
                >
                  <Settings className="size-4" />
                </Button>
              </div>
            </div>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <Label htmlFor="settings-source" className="mb-1.5">Source URL</Label>
              <Input
                id="settings-source"
                value={settingsSourceURL}
                onChange={(e) => onSourceURLChange(e.target.value)}
                placeholder="https://github.com/example/repo.git"
              />
            </div>
            <div>
              <Label htmlFor="settings-server" className="mb-1.5">Target Server</Label>
              <Select
                id="settings-server"
                value={settingsServerID}
                onChange={(e) => onServerIDChange(e.target.value)}
              >
                <option value="local">Local server</option>
                {remoteServers.map((server) => (
                  <option key={server.id} value={server.id} disabled={server.connected === false}>
                    {server.hostname || server.id}
                    {server.connected === false ? ' (agent disconnected)' : ''}
                  </option>
                ))}
              </Select>
            </div>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <Label htmlFor="settings-branch" className="mb-1.5">Branch</Label>
              <Input
                id="settings-branch"
                value={settingsBranch}
                onChange={(e) => onBranchChange(e.target.value)}
                placeholder="main"
              />
            </div>
            <div>
              <Label className="mb-1.5">Created</Label>
              <Input
                value={new Date(app.created_at).toLocaleString()}
                readOnly
                className="bg-muted"
              />
            </div>
            <div>
              <Label className="mb-1.5">Last Updated</Label>
              <Input
                value={new Date(app.updated_at).toLocaleString()}
                readOnly
                className="bg-muted"
              />
            </div>
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={onReset}
              disabled={!settingsDirty || settingsSaving}
              className="cursor-pointer"
            >
              Reset
            </Button>
            <Button
              size="sm"
              onClick={onSave}
              disabled={!settingsDirty || settingsSaving || !settingsName.trim()}
              className="cursor-pointer"
            >
              {settingsSaving ? 'Saving…' : 'Save Changes'}
            </Button>
          </div>
        </div>
      </div>

      <div className="rounded-lg border border-destructive/50 bg-card p-6">
        <h2 className="text-base font-semibold text-destructive mb-1">Danger Zone</h2>
        <p className="text-sm text-muted-foreground mb-4">
          These actions are irreversible. Please proceed with caution.
        </p>
        <div className="flex items-center justify-between rounded-lg border border-destructive/30 p-4">
          <div>
            <p className="font-medium text-sm">Delete this application</p>
            <p className="text-sm text-muted-foreground">
              Permanently remove this application, all deployments, domains, and associated data.
            </p>
          </div>
          <Button
            variant="destructive"
            size="sm"
            className="cursor-pointer shrink-0 ml-4"
            onClick={onDeleteRequested}
          >
            <Trash2 className="size-4" />
            Delete Application
          </Button>
        </div>
      </div>
    </div>
  );
}