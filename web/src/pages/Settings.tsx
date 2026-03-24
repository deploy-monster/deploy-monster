import { useState } from 'react';
import {
  User, Lock, Moon, Sun, Monitor, Key, Copy, Check, Bell, Globe, Save,
} from 'lucide-react';
import { useAuthStore } from '@/stores/auth';
import { useThemeStore } from '@/stores/theme';
import { api } from '@/api/client';
import { toast } from '@/components/Toast';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { Separator } from '@/components/ui/separator';
export function Settings() {
  const user = useAuthStore((s) => s.user);
  const { theme, setTheme } = useThemeStore();
  // Profile
  const [editName, setEditName] = useState(user?.name || '');
  const [saving, setSaving] = useState(false);
  // Security
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [changingPassword, setChangingPassword] = useState(false);
  const [twoFA, setTwoFA] = useState(false);
  // API Key
  const [generatedKey, setGeneratedKey] = useState('');
  const [copied, setCopied] = useState(false);
  // Notifications
  const [notifications, setNotifications] = useState({
    email: true,
    slack: false,
    discord: false,
    deploy: true,
  });
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
    try {
      const result = await api.post<{ key: string }>('/admin/api-keys');
      setGeneratedKey(result.key);
      toast.success('API key generated -- save it now!');
    } catch {
      toast.error('Failed to generate API key');
    }
  };
  const handleCopy = () => {
    navigator.clipboard.writeText(generatedKey);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  const themes = [
    { id: 'light' as const, label: 'Light', icon: Sun },
    { id: 'dark' as const, label: 'Dark', icon: Moon },
    { id: 'system' as const, label: 'System', icon: Monitor },
  ];
  return (
    <div className="space-y-6 max-w-3xl">
      <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
      <Tabs defaultValue="profile">
        <TabsList>
          <TabsTrigger value="profile">
            <User size={14} /> Profile
          </TabsTrigger>
          <TabsTrigger value="security">
            <Lock size={14} /> Security
          </TabsTrigger>
        </TabsList>
        {/* Profile Tab */}
        <TabsContent value="profile" className="space-y-6">
          {/* Profile Info */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <User size={18} /> Profile
              </CardTitle>
              <CardDescription>Update your personal information.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="profile-name">Name</Label>
                <div className="flex gap-2">
                  <Input
                    id="profile-name"
                    value={editName}
                    onChange={(e) => setEditName(e.target.value)}
                    className="max-w-sm"
                  />
                  <Button onClick={handleSaveProfile} disabled={saving}>
                    <Save size={14} /> {saving ? 'Saving...' : 'Save'}
                  </Button>
                </div>
              </div>
              <div className="flex items-center justify-between py-2 max-w-sm">
                <span className="text-sm text-muted-foreground">Email</span>
                <span className="text-sm font-medium">{user?.email}</span>
              </div>
              <div className="flex items-center justify-between py-2 max-w-sm">
                <span className="text-sm text-muted-foreground">Role</span>
                <Badge variant="outline">{user?.role?.replace('role_', '')}</Badge>
              </div>
            </CardContent>
          </Card>
          {/* Appearance */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Sun size={18} /> Appearance
              </CardTitle>
              <CardDescription>Choose your preferred theme.</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex gap-3">
                {themes.map(({ id, label, icon: Icon }) => (
                  <Button
                    key={id}
                    variant={theme === id ? 'default' : 'outline'}
                    onClick={() => setTheme(id)}
                    className="gap-2"
                  >
                    <Icon size={16} /> {label}
                  </Button>
                ))}
              </div>
            </CardContent>
          </Card>
          {/* Notifications */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Bell size={18} /> Notifications
              </CardTitle>
              <CardDescription>Configure your notification preferences.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {([
                { key: 'email' as const, label: 'Email notifications' },
                { key: 'slack' as const, label: 'Slack alerts' },
                { key: 'discord' as const, label: 'Discord alerts' },
                { key: 'deploy' as const, label: 'Deploy notifications' },
              ]).map(({ key, label }) => (
                <div key={key} className="flex items-center justify-between max-w-sm">
                  <Label>{label}</Label>
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
              <CardTitle className="flex items-center gap-2">
                <Globe size={18} /> Domain Settings
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 text-sm max-w-sm">
              <div className="flex items-center justify-between py-1.5">
                <span className="text-muted-foreground">Auto-subdomain suffix</span>
                <code className="font-mono text-foreground">.deploy.monster</code>
              </div>
              <Separator />
              <div className="flex items-center justify-between py-1.5">
                <span className="text-muted-foreground">SSL provider</span>
                <span>Let's Encrypt (auto)</span>
              </div>
              <Separator />
              <div className="flex items-center justify-between py-1.5">
                <span className="text-muted-foreground">Registration mode</span>
                <span>Open</span>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        {/* Security Tab */}
        <TabsContent value="security" className="space-y-6">
          {/* Change Password */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Lock size={18} /> Change Password
              </CardTitle>
              <CardDescription>Update your account password.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4 max-w-sm">
              <div className="space-y-2">
                <Label htmlFor="current-pwd">Current Password</Label>
                <Input
                  id="current-pwd"
                  type="password"
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-pwd">New Password</Label>
                <Input
                  id="new-pwd"
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                />
              </div>
              <Button
                onClick={handleChangePassword}
                disabled={changingPassword || !currentPassword || !newPassword}
              >
                {changingPassword ? 'Changing...' : 'Change Password'}
              </Button>
            </CardContent>
          </Card>
          {/* Two-Factor Authentication */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Lock size={18} /> Two-Factor Authentication
              </CardTitle>
              <CardDescription>
                Add an extra layer of security to your account.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex items-center justify-between max-w-sm">
                <div>
                  <p className="text-sm font-medium">Enable 2FA</p>
                  <p className="text-sm text-muted-foreground">
                    Use an authenticator app for login verification
                  </p>
                </div>
                <Switch
                  checked={twoFA}
                  onCheckedChange={setTwoFA}
                />
              </div>
            </CardContent>
          </Card>
          {/* API Keys */}
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="flex items-center gap-2">
                    <Key size={18} /> API Keys
                  </CardTitle>
                  <CardDescription>Generate keys for programmatic access.</CardDescription>
                </div>
                <Button variant="outline" size="sm" onClick={handleGenerateKey}>
                  Generate New Key
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {generatedKey ? (
                <div className="space-y-2">
                  <p className="text-xs text-muted-foreground">
                    Save this key -- it will not be shown again:
                  </p>
                  <div className="flex items-center gap-2">
                    <code className="flex-1 truncate rounded-md border bg-muted/50 px-3 py-2 font-mono text-sm">
                      {generatedKey}
                    </code>
                    <Button variant="ghost" size="icon" onClick={handleCopy}>
                      {copied ? (
                        <Check size={16} className="text-emerald-500" />
                      ) : (
                        <Copy size={16} />
                      )}
                    </Button>
                  </div>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">
                  Generate an API key for programmatic access.
                </p>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
