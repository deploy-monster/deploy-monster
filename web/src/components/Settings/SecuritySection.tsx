// P1-12: Migrate hand-rolled mutation state to useMutation hook
import { useState } from 'react';
import { Lock, Shield, Loader2, Plus, Trash2, Copy, Check, AlertCircle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { PasswordInput } from '@/components/Marketplace/PasswordInput';
import { useApi, useMutation } from '@/hooks';
import { adminAPI, type APIKey } from '@/api/admin';
import { api } from '@/api/client';
import { toast } from '@/stores/toastStore';
import { useAuthStore } from '@/stores/auth';

interface SecuritySectionProps {
  onPasswordChanged?: () => void;
}

export function SecuritySection({ onPasswordChanged }: SecuritySectionProps) {
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');

  const { mutate: changePassword, loading: changingPassword } = useMutation<
    { current_password: string; new_password: string },
    void
  >('post', '/auth/change-password');

  const handleChangePassword = () => {
    if (!currentPassword || !newPassword) return;
    void changePassword(
      { current_password: currentPassword, new_password: newPassword },
      {
        onSuccess: () => {
          toast.success('Password changed');
          setCurrentPassword('');
          setNewPassword('');
          onPasswordChanged?.();
        },
        onError: (err) => { toast.error(err); },
      },
    ).catch(() => undefined);
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

export function TwoFactorSection() {
  const { data: status } = useApi<{ enabled: boolean }>('/auth/totp/status');
  const [enabledOverride, setEnabledOverride] = useState<boolean | null>(null);
  const [enrollmentCode, setEnrollmentCode] = useState('');
  const [disableCode, setDisableCode] = useState('');
  const [provisioningURI, setProvisioningURI] = useState('');
  const [loading, setLoading] = useState(false);
  const enabled = enabledOverride ?? status?.enabled ?? false;

  const startEnrollment = async () => {
    setLoading(true);
    try {
      const result = await api.post<{ provisioning_uri?: string }>('/auth/totp/enroll', {});
      setProvisioningURI(result?.provisioning_uri || '');
      setEnrollmentCode('');
      toast.success('Two-factor authentication setup started');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Two-factor authentication update failed');
    } finally {
      setLoading(false);
    }
  };

  const handleEnrollmentSwitch = (checked: boolean) => {
    if (checked && !enabled && !provisioningURI) {
      void startEnrollment();
      return;
    }
    if (!checked && provisioningURI) {
      setProvisioningURI('');
      setEnrollmentCode('');
    }
  };

  const handleVerify = async () => {
    if (!enrollmentCode.trim()) return;
    setLoading(true);
    try {
      await api.post('/auth/totp/enroll', { code: enrollmentCode });
      setEnabledOverride(true);
      setProvisioningURI('');
      setEnrollmentCode('');
      toast.success('Two-factor authentication enabled');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Two-factor authentication verification failed');
    } finally {
      setLoading(false);
    }
  };

  const handleDisable = async () => {
    if (!disableCode.trim()) return;
    setLoading(true);
    try {
      await api.post('/auth/totp/disable', { code: disableCode });
      setEnabledOverride(false);
      setDisableCode('');
      toast.success('Two-factor authentication disabled');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Two-factor authentication update failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Shield className="size-4 text-primary" />
          Two-Factor Authentication
        </CardTitle>
        <CardDescription>Require an authentication code when signing in.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4 max-w-sm">
        <div className="flex items-center justify-between gap-4">
          <div className="text-sm">
            {enabled ? 'Two-factor authentication is enabled.' : 'Enable 2FA'}
          </div>
          <Switch
            checked={enabled || !!provisioningURI}
            onCheckedChange={handleEnrollmentSwitch}
            disabled={loading || enabled}
          />
        </div>
        {provisioningURI && (
          <div className="space-y-3">
            <Input readOnly value={provisioningURI} />
            <div className="space-y-1.5">
              <Label htmlFor="totp-code">Authentication Code</Label>
              <Input id="totp-code" value={enrollmentCode} onChange={(e) => setEnrollmentCode(e.target.value)} />
            </div>
            <Button onClick={handleVerify} disabled={loading || !enrollmentCode.trim()}>
              Verify
            </Button>
          </div>
        )}
        {enabled && (
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="totp-disable-code">Authentication Code</Label>
              <Input id="totp-disable-code" value={disableCode} onChange={(e) => setDisableCode(e.target.value)} />
            </div>
            <Button variant="outline" onClick={handleDisable} disabled={loading || !disableCode.trim()}>
              Disable
            </Button>
          </div>
        )}
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

  if (!canManage) return null;

  const handleGenerateKey = async () => {
    setGeneratingKey(true);
    try {
      const result = await adminAPI.generateApiKey();
      setGeneratedKey(result.key);
      setGeneratedKeyPrefix(result.prefix);
      refetchAPIKeys();
      toast.success('API key generated -- save it now!');
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

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(generatedKey);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
      toast.success('API key copied to clipboard');
    } catch {
      toast.error('Failed to copy API key');
    }
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
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleRevokeKey(generatedKeyPrefix)}
                disabled={!generatedKeyPrefix || revokingKey === generatedKeyPrefix}
                className="shrink-0 cursor-pointer"
              >
                <Trash2 className="size-4" />
                Revoke Key
              </Button>
            </div>
          </div>
        )}

        {apiKeys.length > 0 ? (
          <div className="space-y-2">
            {apiKeys.map((key) => (
              <div key={key.id ?? key.prefix} className="flex items-center justify-between rounded-lg border p-3">
                <div className="flex items-center gap-3 min-w-0">
                  <code className="font-mono text-sm text-muted-foreground truncate">
                    {key.prefix}
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
        ) : (
          <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
            No API keys
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
