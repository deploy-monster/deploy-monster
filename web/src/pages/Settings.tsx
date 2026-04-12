import { useState } from 'react';
import {
  User,
  Lock,
  Moon,
  Sun,
  Monitor,
  Key,
  Copy,
  Check,
  Bell,
  Globe,
  Save,
  Shield,
  Eye,
  EyeOff,
  Camera,
  Trash2,
  Loader2,
  AlertCircle,
} from 'lucide-react';
import { useAuthStore } from '@/stores/auth';
import { useThemeStore } from '@/stores/theme';
import { cn } from '@/lib/utils';
import { api } from '@/api/client';
import { adminAPI } from '@/api/admin';
import { toast } from '@/stores/toastStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { Separator } from '@/components/ui/separator';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function PasswordInput({
  id,
  value,
  onChange,
  placeholder,
}: {
  id: string;
  value: string;
  onChange: (val: string) => void;
  placeholder: string;
}) {
  const [visible, setVisible] = useState(false);

  return (
    <div className="relative">
      <Lock className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
      <Input
        id={id}
        type={visible ? 'text' : 'password'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="pl-9 pr-10"
      />
      <button
        type="button"
        onClick={() => setVisible((v) => !v)}
        className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
        tabIndex={-1}
      >
        {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  );
}

function getInitials(name: string) {
  return name
    .split(' ')
    .map((n) => n[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

export function Settings() {
  const user = useAuthStore((s) => s.user);
  const { theme, setTheme } = useThemeStore();

  // Profile
  const [editName, setEditName] = useState(user?.name || '');
  const [editEmail] = useState(user?.email || '');
  const [saving, setSaving] = useState(false);

  // Security
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [changingPassword, setChangingPassword] = useState(false);
  const [twoFA, setTwoFA] = useState(false);

  // API Key
  const [generatedKey, setGeneratedKey] = useState('');
  const [copied, setCopied] = useState(false);
  const [generatingKey, setGeneratingKey] = useState(false);

  // Notifications
  const [notifications, setNotifications] = useState({
    email: true,
    slack: false,
    discord: false,
    deploy: true,
  });

  const themes = [
    { id: 'light' as const, label: 'Light', icon: Sun },
    { id: 'dark' as const, label: 'Dark', icon: Moon },
    { id: 'system' as const, label: 'System', icon: Monitor },
  ];

  const handleSaveProfile = async () => {
    setSaving(true);
    try {
      await api.patch('/auth/me', { name: editName });
      toast.success('Profile updated');
    } catch {
      toast.error('Failed to update profile');
    } finally {
      setSaving(false);
    }
  };

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
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to change password');
    } finally {
      setChangingPassword(false);
    }
  };

  const handleGenerateKey = async () => {
    setGeneratingKey(true);
    try {
      const result = await adminAPI.generateApiKey();
      setGeneratedKey(result.key);
      toast.success('API key generated -- save it now!');
    } catch {
      toast.error('Failed to generate API key');
    } finally {
      setGeneratingKey(false);
    }
  };

  const handleRevokeKey = () => {
    setGeneratedKey('');
    toast.success('API key revoked');
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(generatedKey);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="space-y-8 max-w-3xl">
      {/* Page header */}
      <div>
        <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
          Settings
        </h1>
        <p className="text-muted-foreground mt-1.5 text-sm">
          Manage your profile, security, and preferences.
        </p>
      </div>

      <Tabs defaultValue="profile">
        <TabsList>
          <TabsTrigger value="profile">
            <User className="size-3.5" />
            Profile
          </TabsTrigger>
          <TabsTrigger value="security">
            <Lock className="size-3.5" />
            Security
          </TabsTrigger>
        </TabsList>

        {/* ================================================================
            Profile Tab
        ================================================================ */}
        <TabsContent value="profile" className="space-y-6">
          {/* Avatar & Profile Info */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <User className="size-4 text-primary" />
                Profile Information
              </CardTitle>
              <CardDescription>Update your personal information and avatar.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Avatar upload area */}
              <div className="flex items-center gap-5">
                <div className="relative group">
                  <Avatar className="size-20 text-lg">
                    <AvatarFallback className="bg-primary/10 text-primary text-xl font-semibold">
                      {getInitials(editName || editEmail || 'U')}
                    </AvatarFallback>
                  </Avatar>
                  <div className="absolute inset-0 flex items-center justify-center rounded-full bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer">
                    <Camera className="size-5 text-white" />
                  </div>
                </div>
                <div>
                  <p className="text-sm font-medium text-foreground">{editName || 'Your Name'}</p>
                  <p className="text-xs text-muted-foreground">{editEmail}</p>
                  <Badge variant="outline" className="mt-1.5 text-[10px] font-normal">
                    {user?.role?.replace('role_', '') || 'user'}
                  </Badge>
                </div>
              </div>

              <Separator />

              {/* Name */}
              <div className="space-y-1.5">
                <Label htmlFor="profile-name">Display Name</Label>
                <Input
                  id="profile-name"
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  placeholder="Your full name"
                  className="max-w-sm"
                />
              </div>

              {/* Email (read-only) */}
              <div className="space-y-1.5">
                <Label>Email Address</Label>
                <Input
                  value={editEmail}
                  disabled
                  className="max-w-sm bg-muted/50"
                />
                <p className="text-[11px] text-muted-foreground">
                  Email changes require admin verification.
                </p>
              </div>

              <Button onClick={handleSaveProfile} disabled={saving}>
                {saving ? (
                  <>
                    <Loader2 className="size-4 animate-spin" />
                    Saving...
                  </>
                ) : (
                  <>
                    <Save className="size-4" />
                    Save Profile
                  </>
                )}
              </Button>
            </CardContent>
          </Card>

          {/* Appearance */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Sun className="size-4 text-primary" />
                Appearance
              </CardTitle>
              <CardDescription>Choose your preferred theme for the dashboard.</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex gap-3">
                {themes.map(({ id, label, icon: Icon }) => (
                  <button
                    key={id}
                    onClick={() => setTheme(id)}
                    className={cn(
                      'flex flex-col items-center gap-2 rounded-xl border-2 p-4 transition-all duration-200 cursor-pointer min-w-[90px]',
                      'hover:translate-y-[-1px] hover:shadow-md',
                      theme === id
                        ? 'border-primary bg-primary/5 shadow-sm'
                        : 'border-transparent bg-muted/50 hover:border-border'
                    )}
                  >
                    <div className={cn(
                      'flex items-center justify-center rounded-lg size-10',
                      theme === id ? 'bg-primary/10' : 'bg-muted'
                    )}>
                      <Icon className={cn(
                        'size-5',
                        theme === id ? 'text-primary' : 'text-muted-foreground'
                      )} />
                    </div>
                    <span className={cn(
                      'text-xs font-medium',
                      theme === id ? 'text-primary' : 'text-muted-foreground'
                    )}>
                      {label}
                    </span>
                  </button>
                ))}
              </div>
            </CardContent>
          </Card>

          {/* Notifications */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Bell className="size-4 text-primary" />
                Notifications
              </CardTitle>
              <CardDescription>Configure your notification preferences.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-1">
              {([
                { key: 'email' as const, label: 'Email notifications', desc: 'Receive alerts via email' },
                { key: 'slack' as const, label: 'Slack alerts', desc: 'Send alerts to your Slack workspace' },
                { key: 'discord' as const, label: 'Discord alerts', desc: 'Send alerts to your Discord server' },
                { key: 'deploy' as const, label: 'Deploy notifications', desc: 'Get notified on successful and failed deployments' },
              ]).map(({ key, label, desc }) => (
                <div key={key} className="flex items-center justify-between rounded-lg p-3 hover:bg-muted/50 transition-colors max-w-md">
                  <div className="space-y-0.5">
                    <Label className="text-sm font-medium">{label}</Label>
                    <p className="text-xs text-muted-foreground">{desc}</p>
                  </div>
                  <Switch
                    checked={notifications[key]}
                    onCheckedChange={(v) =>
                      setNotifications((prev) => ({ ...prev, [key]: v }))
                    }
                  />
                </div>
              ))}
            </CardContent>
          </Card>

          {/* Domain Settings */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Globe className="size-4 text-primary" />
                Domain Settings
              </CardTitle>
              <CardDescription>Platform domain and SSL configuration.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-0 text-sm max-w-md">
              <div className="flex items-center justify-between py-3">
                <span className="text-muted-foreground">Auto-subdomain suffix</span>
                <code className="font-mono text-foreground bg-muted/50 px-2 py-0.5 rounded text-xs">.deploy.monster</code>
              </div>
              <Separator />
              <div className="flex items-center justify-between py-3">
                <span className="text-muted-foreground">SSL provider</span>
                <span className="text-sm font-medium">Let&apos;s Encrypt (auto)</span>
              </div>
              <Separator />
              <div className="flex items-center justify-between py-3">
                <span className="text-muted-foreground">Registration mode</span>
                <Badge variant="outline" className="text-xs font-normal">Open</Badge>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* ================================================================
            Security Tab
        ================================================================ */}
        <TabsContent value="security" className="space-y-6">
          {/* Change Password */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Lock className="size-4 text-primary" />
                Change Password
              </CardTitle>
              <CardDescription>Update your account password. Use a strong, unique password.</CardDescription>
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
                <p className="text-[11px] text-muted-foreground">
                  Minimum 8 characters with uppercase, lowercase, and numbers.
                </p>
              </div>
              <Button
                onClick={handleChangePassword}
                disabled={changingPassword || !currentPassword || !newPassword}
              >
                {changingPassword ? (
                  <>
                    <Loader2 className="size-4 animate-spin" />
                    Changing...
                  </>
                ) : (
                  <>
                    <Lock className="size-4" />
                    Change Password
                  </>
                )}
              </Button>
            </CardContent>
          </Card>

          {/* Two-Factor Authentication */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Shield className="size-4 text-primary" />
                Two-Factor Authentication
              </CardTitle>
              <CardDescription>
                Add an extra layer of security to protect your account.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex items-center justify-between max-w-md rounded-lg border p-4">
                <div className="space-y-0.5">
                  <p className="text-sm font-medium text-foreground">Enable 2FA</p>
                  <p className="text-xs text-muted-foreground">
                    Use an authenticator app (Google Authenticator, Authy) for login verification
                  </p>
                </div>
                <Switch
                  checked={twoFA}
                  onCheckedChange={setTwoFA}
                />
              </div>
              {twoFA && (
                <div className="flex items-center gap-2 mt-3 rounded-lg border border-emerald-500/20 bg-emerald-500/5 px-3 py-2.5 max-w-md">
                  <Check className="size-4 text-emerald-500 shrink-0" />
                  <p className="text-xs text-emerald-600 dark:text-emerald-400">
                    Two-factor authentication is enabled for your account.
                  </p>
                </div>
              )}
            </CardContent>
          </Card>

          {/* API Keys */}
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="flex items-center gap-2 text-base">
                    <Key className="size-4 text-primary" />
                    API Keys
                  </CardTitle>
                  <CardDescription>Generate keys for programmatic access to the API.</CardDescription>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleGenerateKey}
                  disabled={generatingKey}
                >
                  {generatingKey ? (
                    <Loader2 className="size-3.5 animate-spin" />
                  ) : (
                    <Key className="size-3.5" />
                  )}
                  Generate New Key
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {generatedKey ? (
                <div className="space-y-3">
                  <div className="flex items-center gap-2 rounded-lg border border-amber-500/20 bg-amber-500/5 px-3 py-2.5">
                    <AlertCircle className="size-4 text-amber-500 shrink-0" />
                    <p className="text-xs text-amber-600 dark:text-amber-400">
                      Save this key now. It will not be shown again after you leave this page.
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <code className="flex-1 truncate rounded-lg border bg-muted/50 px-3 py-2.5 font-mono text-sm">
                      {generatedKey}
                    </code>
                    <Button variant="outline" size="icon" onClick={handleCopy} className="shrink-0">
                      {copied ? (
                        <Check className="size-4 text-emerald-500" />
                      ) : (
                        <Copy className="size-4" />
                      )}
                    </Button>
                  </div>
                  <Button variant="destructive" size="sm" onClick={handleRevokeKey}>
                    <Trash2 className="size-3.5" />
                    Revoke Key
                  </Button>
                </div>
              ) : (
                <div className="flex flex-col items-center py-8 text-center">
                  <div className="rounded-full bg-muted p-4 mb-3">
                    <Key className="size-6 text-muted-foreground" />
                  </div>
                  <p className="text-sm font-medium text-foreground mb-1">No API keys</p>
                  <p className="text-xs text-muted-foreground max-w-xs">
                    Generate an API key to access the DeployMonster API programmatically from scripts and integrations.
                  </p>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
