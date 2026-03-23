import { useState } from 'react';
import { Server, Plus, Wifi } from 'lucide-react';

const providers = [
  { id: 'hetzner', name: 'Hetzner Cloud', icon: '🇩🇪' },
  { id: 'digitalocean', name: 'DigitalOcean', icon: '🌊' },
  { id: 'vultr', name: 'Vultr', icon: '⚡' },
  { id: 'custom', name: 'Custom SSH', icon: '🔑' },
];

export function Servers() {
  const [showAdd, setShowAdd] = useState(false);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-text-primary">Servers</h1>
          <p className="text-sm text-text-secondary mt-1">Manage your infrastructure</p>
        </div>
        <button onClick={() => setShowAdd(!showAdd)}
          className="flex items-center gap-2 px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm font-medium rounded-lg transition-colors">
          <Plus size={16} /> Add Server
        </button>
      </div>

      {showAdd && (
        <div className="bg-surface border border-border rounded-xl p-6">
          <h2 className="font-medium text-text-primary mb-4">Choose Provider</h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
            {providers.map((p) => (
              <button key={p.id}
                className="flex items-center gap-3 p-4 rounded-xl border border-border hover:border-monster-green/50 hover:bg-surface-secondary transition-colors text-left">
                <span className="text-2xl">{p.icon}</span>
                <div>
                  <span className="font-medium text-text-primary text-sm block">{p.name}</span>
                  <span className="text-xs text-text-muted">{p.id === 'custom' ? 'Connect existing server' : 'Provision new server'}</span>
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Local server (always present) */}
      <div className="bg-surface border border-border rounded-xl overflow-hidden">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="font-medium text-text-primary">Connected Servers</h2>
        </div>
        <div className="divide-y divide-border">
          <div className="flex items-center justify-between px-5 py-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-monster-green/10 flex items-center justify-center">
                <Server size={20} className="text-monster-green" />
              </div>
              <div>
                <p className="font-medium text-text-primary">localhost</p>
                <p className="text-sm text-text-secondary">127.0.0.1 (this server)</p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <Wifi size={16} className="text-status-running" />
              <span className="text-sm text-status-running">Active</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
