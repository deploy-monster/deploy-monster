import { useAuthStore } from '../stores/auth';
import { useThemeStore } from '../stores/theme';
import { Moon, Sun, Monitor, Shield, Bell, Globe, Key } from 'lucide-react';

export function Settings() {
  const user = useAuthStore((s) => s.user);
  const { theme, setTheme } = useThemeStore();

  const themes = [
    { id: 'light' as const, label: 'Light', icon: Sun },
    { id: 'dark' as const, label: 'Dark', icon: Moon },
    { id: 'system' as const, label: 'System', icon: Monitor },
  ];

  return (
    <div className="space-y-6 max-w-3xl">
      <h1 className="text-2xl font-semibold text-text-primary">Settings</h1>

      {/* Profile */}
      <section className="bg-surface border border-border rounded-xl p-6">
        <h2 className="font-medium text-text-primary mb-4 flex items-center gap-2"><Shield size={18} /> Profile</h2>
        <div className="space-y-3">
          <div className="flex items-center justify-between py-2">
            <span className="text-sm text-text-secondary">Name</span>
            <span className="text-sm text-text-primary">{user?.name}</span>
          </div>
          <div className="flex items-center justify-between py-2">
            <span className="text-sm text-text-secondary">Email</span>
            <span className="text-sm text-text-primary">{user?.email}</span>
          </div>
          <div className="flex items-center justify-between py-2">
            <span className="text-sm text-text-secondary">Role</span>
            <span className="text-sm text-text-primary">{user?.role?.replace('role_', '')}</span>
          </div>
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
          <button className="text-sm text-monster-green hover:underline">Generate New Key</button>
        </div>
        <p className="text-sm text-text-muted">No API keys created yet.</p>
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
            <span className="text-text-primary">Let's Encrypt</span>
          </div>
        </div>
      </section>
    </div>
  );
}
