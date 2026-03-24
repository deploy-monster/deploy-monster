import { useState } from 'react';
import { Lock, Plus, Eye, EyeOff } from 'lucide-react';
import { api } from '../api/client';

export function Secrets() {
  const [showCreate, setShowCreate] = useState(false);
  const [name, setName] = useState('');
  const [value, setValue] = useState('');
  const [scope, setScope] = useState('tenant');
  const [showValue, setShowValue] = useState(false);

  const handleCreate = async () => {
    if (!name || !value) return;
    await api.post('/secrets', { name, value, scope });
    setName('');
    setValue('');
    setShowCreate(false);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-text-primary">Secrets</h1>
          <p className="text-sm text-text-secondary mt-1">Encrypted secret storage with AES-256-GCM</p>
        </div>
        <button onClick={() => setShowCreate(!showCreate)}
          className="flex items-center gap-2 px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm font-medium rounded-lg transition-colors">
          <Plus size={16} /> Add Secret
        </button>
      </div>

      {showCreate && (
        <div className="bg-surface border border-border rounded-xl p-6 space-y-4">
          <h2 className="font-medium text-text-primary">Create Secret</h2>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">Name</label>
              <input type="text" value={name} onChange={(e) => setName(e.target.value)}
                className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50"
                placeholder="DB_PASSWORD" />
            </div>
            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">Value</label>
              <div className="relative">
                <input type={showValue ? 'text' : 'password'} value={value} onChange={(e) => setValue(e.target.value)}
                  className="w-full px-3 py-2 pr-10 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50"
                  placeholder="secret value" />
                <button onClick={() => setShowValue(!showValue)} className="absolute right-2 top-2 text-text-muted">
                  {showValue ? <EyeOff size={18} /> : <Eye size={18} />}
                </button>
              </div>
            </div>
            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">Scope</label>
              <select value={scope} onChange={(e) => setScope(e.target.value)}
                className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary">
                <option value="global">Global</option>
                <option value="tenant">Tenant</option>
                <option value="project">Project</option>
                <option value="app">App</option>
              </select>
            </div>
          </div>
          <div className="bg-surface-secondary rounded-lg p-3 text-sm text-text-secondary">
            Reference in env vars: <code className="bg-surface-tertiary px-1.5 py-0.5 rounded">${'{'}SECRET:{name || 'name'}{'}'}</code>
          </div>
          <button onClick={handleCreate} className="px-4 py-2 bg-monster-green text-white text-sm rounded-lg">Create Secret</button>
        </div>
      )}

      <div className="bg-surface border border-border rounded-xl px-5 py-16 text-center">
        <Lock className="mx-auto mb-4 text-text-muted" size={48} />
        <h2 className="text-lg font-medium text-text-primary mb-2">Secret Vault</h2>
        <p className="text-text-secondary">Secrets are encrypted with AES-256-GCM and stored securely.</p>
        <p className="text-sm text-text-muted mt-2">Values are never returned by the API — only names and metadata.</p>
      </div>
    </div>
  );
}
