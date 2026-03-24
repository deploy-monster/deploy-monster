import { useState } from 'react';
import { useAuthStore } from '../stores/auth';
import { useThemeStore } from '../stores/theme';
import { Moon, Sun, Monitor, Shield, Bell, Globe, Key, Lock, Save, Copy, Check } from 'lucide-react';
import { api } from '../api/client';
import { toast } from '../components/Toast';

export function Settings() {
  const user = useAuthStore((s) => s.user);
  const { theme, setTheme } = useThemeStore();

  const [editName, setEditName] = useState(user?.name || '');
  const [saving, setSaving] = useState(false);

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [changingPassword, setChangingPassword] = useState(false);

  const [generatedKey, setGeneratedKey] = useState('');
  const [copied, setCopied] = useState(false);

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
    try {
      const result = await api.post<{ key: string }>('/admin/api-keys');
      setGeneratedKey(result.key);
      toast.success('API key generated — save it now!');
    } catch {
      toast.error('Failed to generate API key');
    }
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(generatedKey);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="space-y-6 max-w-3xl">
      <h1 className="text-2xl font-semibold text-text-primary">Settings</h1>

      {/* Profile */}
      <section className="bg-surface border border-border rounded-xl p-6">
        <h2 className="font-medium text-text-primary mb-4 flex items-center gap-2"><Shield size={18} /> Profile</h2>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-text-secondary mb-1">Name</label>
            <div className="flex gap-2">
              <input type="text" value={editName} onChange={(e) => setEditName(e.target.value)}
                className="flex-1 px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50" />
              <button onClick={handleSaveProfile} disabled={saving}
                className="flex items-center gap-1 px-3 py-2 bg-monster-green text-white text-sm rounded-lg hover:bg-monster-green-dark disabled:opacity-50">
                <Save size={14} /> {saving ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>
          <div className="flex items-center justify-between py-2">
            <span className="text-sm text-text-secondary">Email</span>
            <span className="text-sm text-text-primary">{user?.email}</span>
          </div>
          <div className="flex items-center justify-between py-2">
            <span className="text-sm text-text-secondary">Role</span>
            <span className="text-sm text-text-primary capitalize">{user?.role?.replace('role_', '')}</span>
          </div>
        </div>
      </section>

      {/* Change Password */}
      <section className="bg-surface border border-border rounded-xl p-6">
        <h2 className="font-medium text-text-primary mb-4 flex items-center gap-2"><Lock size={18} /> Change Password</h2>
        <div className="space-y-3 max-w-sm">
          <div>
            <label className="block text-sm font-medium text-text-secondary mb-1">Current Password</label>
            <input type="password" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)}
              className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50" />
          </div>
          <div>
            <label className="block text-sm font-medium text-text-secondary mb-1">New Password</label>
            <input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)}
              className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50" />
          </div>
          <button onClick={handleChangePassword} disabled={changingPassword || !currentPassword || !newPassword}
            className="px-4 py-2 bg-monster-green text-white text-sm rounded-lg hover:bg-monster-green-dark disabled:opacity-50">
            {changingPassword ? 'Changing...' : 'Change Password'}
          </button>
        </div>
      </section>

      {/* Theme */}
      <section className="bg-surface border border-border rounded-xl p-6">
        <h2 className="font-medium text-text-primary mb-4 flex items-center gap-2"><Sun size={18} /> Appearance</h2>
        <div className="flex gap-3">
          {themes.map(({ id, label, icon: Icon }) => (
            <button key={id} onClick={() => setTheme(id)}
              className={`flex items-center gap-2 px-4 py-2 rounded-lg border text-sm transition-colors ${
                theme === id
                  ? 'border-monster-green bg-monster-green/10 text-monster-green'
                  : 'border-border text-text-secondary hover:bg-surface-secondary'
              }`}>
              <Icon size={16} /> {label}
            </button>
          ))}
        </div>
      </section>

      {/* Notifications */}
      <section className="bg-surface border border-border rounded-xl p-6">
        <h2 className="font-medium text-text-primary mb-4 flex items-center gap-2"><Bell size={18} /> Notifications</h2>
        <div className="space-y-3">
          {['Email notifications', 'Slack alerts', 'Discord alerts', 'Deploy notifications'].map((item) => (
            <label key={item} className="flex items-center justify-between py-2 cursor-pointer">
              <span className="text-sm text-text-secondary">{item}</span>
              <input type="checkbox" defaultChecked className="w-4 h-4 accent-monster-green" />
            </label>
          ))}
        </div>
      </section>

      {/* API Keys */}
      <section className="bg-surface border border-border rounded-xl p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="font-medium text-text-primary flex items-center gap-2"><Key size={18} /> API Keys</h2>
          <button onClick={handleGenerateKey} className="text-sm text-monster-green hover:underline">Generate New Key</button>
        </div>
        {generatedKey ? (
          <div className="bg-surface-secondary rounded-lg p-3">
            <p className="text-xs text-text-muted mb-2">Save this key — it will not be shown again:</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 text-sm font-mono text-text-primary bg-surface-tertiary px-3 py-2 rounded overflow-x-auto">{generatedKey}</code>
              <button onClick={handleCopy} className="p-2 hover:bg-surface-tertiary rounded">
                {copied ? <Check size={16} className="text-monster-green" /> : <Copy size={16} className="text-text-muted" />}
              </button>
            </div>
          </div>
        ) : (
          <p className="text-sm text-text-muted">Generate an API key for programmatic access.</p>
        )}
      </section>

      {/* Domain Settings */}
      <section className="bg-surface border border-border rounded-xl p-6">
        <h2 className="font-medium text-text-primary mb-4 flex items-center gap-2"><Globe size={18} /> Domain Settings</h2>
        <div className="space-y-3 text-sm">
          <div className="flex items-center justify-between py-2">
            <span className="text-text-secondary">Auto-subdomain suffix</span>
            <span className="text-text-primary font-mono">.deploy.monster</span>
          </div>
          <div className="flex items-center justify-between py-2">
            <span className="text-text-secondary">SSL provider</span>
            <span className="text-text-primary">Let's Encrypt (auto)</span>
          </div>
          <div className="flex items-center justify-between py-2">
            <span className="text-text-secondary">Registration mode</span>
            <span className="text-text-primary">Open</span>
          </div>
        </div>
      </section>
    </div>
  );
}
