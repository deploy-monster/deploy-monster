import { useState } from 'react';
import { Database, Plus } from 'lucide-react';

const engines = [
  { id: 'postgres', name: 'PostgreSQL', versions: ['17', '16', '15'], icon: '🐘' },
  { id: 'mysql', name: 'MySQL', versions: ['8.4', '8.0'], icon: '🐬' },
  { id: 'mariadb', name: 'MariaDB', versions: ['11', '10.11'], icon: '🦭' },
  { id: 'redis', name: 'Redis', versions: ['7'], icon: '🔴' },
  { id: 'mongodb', name: 'MongoDB', versions: ['7'], icon: '🍃' },
];

export function Databases() {
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-text-primary">Databases</h1>
          <p className="text-sm text-text-secondary mt-1">Managed database instances</p>
        </div>
        <button onClick={() => setShowCreate(!showCreate)}
          className="flex items-center gap-2 px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm font-medium rounded-lg transition-colors">
          <Plus size={16} /> Create Database
        </button>
      </div>

      {showCreate && (
        <div className="bg-surface border border-border rounded-xl p-6">
          <h2 className="font-medium text-text-primary mb-4">Choose Database Engine</h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-3">
            {engines.map((e) => (
              <button key={e.id}
                className="flex flex-col items-center gap-2 p-4 rounded-xl border border-border hover:border-monster-green/50 hover:bg-surface-secondary transition-colors">
                <span className="text-2xl">{e.icon}</span>
                <span className="font-medium text-text-primary text-sm">{e.name}</span>
                <span className="text-xs text-text-muted">v{e.versions[0]}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      <div className="bg-surface border border-border rounded-xl px-5 py-16 text-center">
        <Database className="mx-auto mb-4 text-text-muted" size={48} />
        <h2 className="text-lg font-medium text-text-primary mb-2">No databases yet</h2>
        <p className="text-text-secondary">Create a managed database to get started.</p>
      </div>
    </div>
  );
}
