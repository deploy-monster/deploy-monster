import { useEffect, useState } from 'react';
import { Archive, Plus, Download, Clock } from 'lucide-react';
import { api } from '../api/client';

interface BackupEntry {
  key: string;
  size: number;
  created_at: number;
}

export function Backups() {
  const [backups, setBackups] = useState<BackupEntry[]>([]);

  useEffect(() => {
    api.get<{ data: BackupEntry[] }>('/backups').then((r) => setBackups(r.data || [])).catch(() => {});
  }, []);

  const handleCreate = async () => {
    await api.post('/backups', { source_type: 'full', source_id: 'all' });
    api.get<{ data: BackupEntry[] }>('/backups').then((r) => setBackups(r.data || []));
  };

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / 1024 / 1024).toFixed(1) + ' MB';
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-text-primary">Backups</h1>
          <p className="text-sm text-text-secondary mt-1">Volume snapshots and database dumps</p>
        </div>
        <button onClick={handleCreate}
          className="flex items-center gap-2 px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm font-medium rounded-lg transition-colors">
          <Plus size={16} /> Create Backup
        </button>
      </div>

      {backups.length === 0 ? (
        <div className="bg-surface border border-border rounded-xl px-5 py-16 text-center">
          <Archive className="mx-auto mb-4 text-text-muted" size={48} />
          <h2 className="text-lg font-medium text-text-primary mb-2">No backups yet</h2>
          <p className="text-text-secondary mb-4">Backups run automatically at your configured schedule.</p>
          <p className="text-sm text-text-muted">Default: Daily at 02:00 AM</p>
        </div>
      ) : (
        <div className="bg-surface border border-border rounded-xl overflow-hidden">
          <table className="w-full">
            <thead><tr className="border-b border-border bg-surface-secondary text-left">
              <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Name</th>
              <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Size</th>
              <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Created</th>
              <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase w-10"></th>
            </tr></thead>
            <tbody className="divide-y divide-border">
              {backups.map((b) => (
                <tr key={b.key} className="hover:bg-surface-secondary">
                  <td className="px-5 py-3 font-medium text-text-primary font-mono text-sm">{b.key}</td>
                  <td className="px-5 py-3 text-sm text-text-secondary">{formatSize(b.size)}</td>
                  <td className="px-5 py-3 text-sm text-text-muted flex items-center gap-1">
                    <Clock size={14} />{new Date(b.created_at * 1000).toLocaleString()}
                  </td>
                  <td className="px-5 py-3">
                    <button className="p-1.5 rounded hover:bg-surface-tertiary text-text-muted hover:text-monster-green">
                      <Download size={16} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
