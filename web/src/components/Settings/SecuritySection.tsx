import { useState } from 'react';
import { Lock, Shield, Loader2, Plus, Trash2, Copy, Check, AlertCircle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { PasswordInput } from '@/components/Marketplace/PasswordInput';
import { api } from '@/api/client';
import { useApi } from '@/hooks';
import { adminAPI, type APIKey } from '@/api/admin';
import { toast } from '@/stores/toastStore';
import { useAuthStore } from '@/stores/auth';

interface SecuritySectionProps {
  onPasswordChanged?: () => void;
}

export function SecuritySection({ onPasswordChanged }: SecuritySectionProps) {
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [changingPassword, setChangingPassword] = useState(false);

  const handleChangePassword = async () => {
    if (!currentPassword || !newPassword) return;
    setChangingPassword(true);
    try {
      await api.post('/auth/change-password', {
        current_password: currentPassword,
        new_password: newPassword,
      });
      toast.success('Password changed');
      setCurrentPassword('');
      setNewPassword('');
      onPasswordChanged?.();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to change password');
    } finally {
      setChangingPassword(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Lock className="size-4 text-primary" />
          Change Password
        </CardTitle>
        <CardDescription>Update your account password.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4 max-w-sm">
        <div className="space-y-1.5">
          <Label htmlFor="current-pwd">Current Password</Label>
          <PasswordInput
            id="current-pwd"
            value={currentPassword}
            onChange={setCurrentPassword}
            placeholder="Enter current password"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="new-pwd">New Password</Label>
          <PasswordInput
            id="new-pwd"
            value={newPassword}
            onChange={setNewPassword}
            placeholder="Enter new password"
          />
        </div>
        <Button
          onClick={handleChangePassword}
          disabled={changingPassword || !currentPassword || !newPassword}
          className="cursor-pointer"
        >
          {changingPassword ? (
            <><Loader2 className="size-4 animate-spin" /> Changing...</>
          ) : (
            <><Lock className="size-4" /> Change Password</>
          )}
        </Button>
      </CardContent>
    </Card>
  );
}

interface APIKeySectionProps {
  onKeyGenerated?: () => void;
}

export function APIKeySection({ onKeyGenerated }: APIKeySectionProps) {
  const user = useAuthStore((s) => s.user);
  const canManage = user?.role === 'role_super_admin';

  const { data: apiKeysResp, refetch: refetchAPIKeys } = useApi<
    { data: APIKey[]; total: number } | APIKey[]
  >('/admin/api-keys', { immediate: canManage });
  const apiKeys = Array.isArray(apiKeysResp) ? apiKeysResp : apiKeysResp?.data ?? [];

  const [generatedKey, setGeneratedKey] = useState('');
  const [generatedKeyPrefix, setGeneratedKeyPrefix] = useState('');
  const [copied, setCopied] = useState(false);
  const [generatingKey, setGeneratingKey] = useState(false);
  const [revokingKey, setRevokingKey] = useState('');

  const handleGenerateKey = async () => {
    setGeneratingKey(true);
    try {
      const result = await adminAPI.generateApiKey();
      setGeneratedKey(result.key);
      setGeneratedKeyPrefix(result.prefix);
      refetchAPIKeys();
      toast.success('API key generated — save it now!');
      onKeyGenerated?.();
    } catch {
      toast.error('Failed to generate API key');
    } finally {
      setGeneratingKey(false);
    }
  };

  const handleRevokeKey = async (prefix: string) => {
    setRevokingKey(prefix);
    try {
      await adminAPI.revokeApiKey(prefix);
      if (generatedKeyPrefix === prefix) {
        setGeneratedKey('');
        setGeneratedKeyPrefix('');
      }
      refetchAPIKeys();
      toast.success('API key revoked');
    } catch {
      toast.error('Failed to revoke API key');
    } finally {
      setRevokingKey('');
    }
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(generatedKey);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Shield className="size-4 text-primary" />
          API Keys
        </CardTitle>
        <CardDescription>Manage API keys for programmatic access.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {generatedKey && (
          <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-4 space-y-2">
            <div className="flex items-center gap-2">
              <AlertCircle className="size-4 text-amber-600 dark:text-amber-400 shrink-0" />
              <p className="text-sm font-medium text-amber-700 dark:text-amber-400">
                This key is shown only once — copy and store it securely.
              </p>
            </div>
            <div className="flex items-center gap-2">
              <code className="flex-1 font-mono text-sm bg-muted rounded-md px-3 py-2 break-all">
                {generatedKey}
              </code>
              <Button variant="outline" size="sm" onClick={handleCopy} className="shrink-0 cursor-pointer">
                {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
                {copied ? 'Copied' : 'Copy'}
              </Button>
            </div>
          </div>
        )}

        {apiKeys.length > 0 && (
          <div className="space-y-2">
            {apiKeys.map((key) => (
              <div key={key.id} className="flex items-center justify-between rounded-lg border p-3">
                <div className="flex items-center gap-3 min-w-0">
                  <code className="font-mono text-sm text-muted-foreground truncate">
                    {key.prefix}...
                  </code>
                  <span className="text-xs text-muted-foreground">
                    {new Date(key.created_at).toLocaleDateString()}
                  </span>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-destructive hover:text-destructive shrink-0 cursor-pointer"
                  onClick={() => handleRevokeKey(key.prefix)}
                  disabled={revokingKey === key.prefix}
                >
                  <Trash2 className="size-4" />
                  Revoke
                </Button>
              </div>
            ))}
          </div>
        )}

        <Button
          variant="outline"
          onClick={handleGenerateKey}
          disabled={generatingKey}
          className="cursor-pointer"
        >
          {generatingKey ? (
            <><Loader2 className="size-4 animate-spin" /> Generating...</>
          ) : (
            <><Plus className="size-4" /> Generate New Key</>
          )}
        </Button>
      </CardContent>
    </Card>
  );
}