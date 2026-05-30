import { useState, useCallback } from 'react';
import { appsAPI } from '@/api/apps';
import { toast } from '@/stores/toastStore';

interface UseAppSettingsStateOptions {
  app: { id: string; name: string; branch?: string; source_url?: string; server_id?: string } | null | undefined;
  onRefetch: () => void;
}

export function useAppSettingsState({ app, onRefetch }: UseAppSettingsStateOptions) {
  const [nameDraft, setNameDraft] = useState<string | null>(null);
  const [branchDraft, setBranchDraft] = useState<string | null>(null);
  const [sourceURLDraft, setSourceURLDraft] = useState<string | null>(null);
  const [serverIDDraft, setServerIDDraft] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const name = nameDraft ?? app?.name ?? '';
  const branch = branchDraft ?? app?.branch ?? '';
  const sourceURL = sourceURLDraft ?? app?.source_url ?? '';
  const serverID = serverIDDraft ?? app?.server_id ?? 'local';

  const dirty = !!app && (
    name !== app.name ||
    branch !== (app.branch ?? '') ||
    sourceURL !== (app.source_url ?? '') ||
    serverID !== (app.server_id || 'local')
  );

  const save = useCallback(async () => {
    if (!app?.id) return;
    setSaving(true);
    try {
      const patch: { name?: string; branch?: string; source_url?: string; server_id?: string } = {};
      if (name !== app.name) patch.name = name;
      if (branch !== (app.branch ?? '')) patch.branch = branch;
      if (sourceURL !== (app.source_url ?? '')) patch.source_url = sourceURL;
      if (serverID !== (app.server_id || 'local')) {
        patch.server_id = serverID === 'local' ? '' : serverID;
      }
      await appsAPI.update(app.id, patch);
      toast.success('Settings saved');
      await onRefetch();
      setNameDraft(null);
      setBranchDraft(null);
      setSourceURLDraft(null);
      setServerIDDraft(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save settings');
    } finally {
      setSaving(false);
    }
  }, [app, name, branch, sourceURL, serverID, onRefetch]);

  const reset = useCallback(() => {
    setNameDraft(null);
    setBranchDraft(null);
    setSourceURLDraft(null);
    setServerIDDraft(null);
  }, []);

  return {
    nameDraft,
    branchDraft,
    sourceURLDraft,
    serverIDDraft,
    saving,
    name,
    branch,
    sourceURL,
    serverID,
    dirty,
    setNameDraft,
    setBranchDraft,
    setSourceURLDraft,
    setServerIDDraft,
    save,
    reset,
  };
}