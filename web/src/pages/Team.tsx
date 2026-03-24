import { useEffect, useState } from 'react';
import { Users, UserPlus, Shield, Clock } from 'lucide-react';
import { api } from '../api/client';

interface Role {
  id: string;
  name: string;
  description: string;
  is_builtin: boolean;
}

interface AuditEntry {
  id: number;
  action: string;
  resource_type: string;
  resource_id: string;
  ip_address: string;
  created_at: string;
}

export function Team() {
  const [tab, setTab] = useState<'members' | 'roles' | 'audit'>('members');
  const [roles, setRoles] = useState<Role[]>([]);
  const [auditLog, setAuditLog] = useState<AuditEntry[]>([]);
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteRole, setInviteRole] = useState('role_developer');
  const [showInvite, setShowInvite] = useState(false);

  useEffect(() => {
    api.get<{ data: Role[] }>('/team/roles').then((r) => setRoles(r.data || [])).catch(() => {});
    api.get<{ data: AuditEntry[] }>('/team/audit-log').then((r) => setAuditLog(r.data || [])).catch(() => {});
  }, []);

  const handleInvite = async () => {
    if (!inviteEmail) return;
    await api.post('/team/invites', { email: inviteEmail, role_id: inviteRole });
    setInviteEmail('');
    setShowInvite(false);
  };

  const tabs = [
    { key: 'members' as const, label: 'Members', icon: Users },
    { key: 'roles' as const, label: 'Roles', icon: Shield },
    { key: 'audit' as const, label: 'Audit Log', icon: Clock },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-text-primary">Team Management</h1>
        <button onClick={() => setShowInvite(!showInvite)}
          className="flex items-center gap-2 px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm font-medium rounded-lg transition-colors">
          <UserPlus size={16} /> Invite Member
        </button>
      </div>

      {showInvite && (
        <div className="bg-surface border border-border rounded-xl p-6 space-y-4">
          <h2 className="font-medium text-text-primary">Invite Team Member</h2>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div className="sm:col-span-2">
              <label className="block text-sm font-medium text-text-secondary mb-1">Email</label>
              <input type="email" value={inviteEmail} onChange={(e) => setInviteEmail(e.target.value)}
                className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50"
                placeholder="colleague@company.com" />
            </div>
            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">Role</label>
              <select value={inviteRole} onChange={(e) => setInviteRole(e.target.value)}
                className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary">
                <option value="role_admin">Admin</option>
                <option value="role_developer">Developer</option>
                <option value="role_operator">Operator</option>
                <option value="role_viewer">Viewer</option>
              </select>
            </div>
          </div>
          <button onClick={handleInvite} className="px-4 py-2 bg-monster-green text-white text-sm rounded-lg">Send Invite</button>
        </div>
      )}

      {/* Tabs */}
      <div className="border-b border-border">
        <nav className="flex gap-6">
          {tabs.map(({ key, label, icon: Icon }) => (
            <button key={key} onClick={() => setTab(key)}
              className={`flex items-center gap-2 pb-3 text-sm font-medium border-b-2 transition-colors ${
                tab === key ? 'border-monster-green text-monster-green' : 'border-transparent text-text-secondary hover:text-text-primary'
              }`}>
              <Icon size={16} /> {label}
            </button>
          ))}
        </nav>
      </div>

      {tab === 'members' && (
        <div className="bg-surface border border-border rounded-xl px-5 py-12 text-center">
          <Users className="mx-auto mb-4 text-text-muted" size={48} />
          <p className="text-text-secondary">Team members will appear here. Use the invite button to add members.</p>
        </div>
      )}

      {tab === 'roles' && (
        <div className="bg-surface border border-border rounded-xl overflow-hidden">
          <table className="w-full">
            <thead><tr className="border-b border-border bg-surface-secondary text-left">
              <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Role</th>
              <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Description</th>
              <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Type</th>
            </tr></thead>
            <tbody className="divide-y divide-border">
              {roles.map((role) => (
                <tr key={role.id} className="hover:bg-surface-secondary">
                  <td className="px-5 py-3 font-medium text-text-primary">{role.name}</td>
                  <td className="px-5 py-3 text-sm text-text-secondary">{role.description}</td>
                  <td className="px-5 py-3">
                    <span className={`text-xs px-2 py-0.5 rounded ${role.is_builtin ? 'bg-blue-100 text-blue-600 dark:bg-blue-900/20 dark:text-blue-400' : 'bg-gray-100 text-gray-600'}`}>
                      {role.is_builtin ? 'Built-in' : 'Custom'}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === 'audit' && (
        <div className="bg-surface border border-border rounded-xl overflow-hidden">
          {auditLog.length === 0 ? (
            <div className="px-5 py-12 text-center text-text-muted">No audit log entries yet</div>
          ) : (
            <table className="w-full">
              <thead><tr className="border-b border-border bg-surface-secondary text-left">
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Action</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Resource</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">IP</th>
                <th className="px-5 py-3 text-xs font-medium text-text-secondary uppercase">Time</th>
              </tr></thead>
              <tbody className="divide-y divide-border">
                {auditLog.map((entry) => (
                  <tr key={entry.id}>
                    <td className="px-5 py-3 text-sm font-medium text-text-primary">{entry.action}</td>
                    <td className="px-5 py-3 text-sm text-text-secondary">{entry.resource_type}/{entry.resource_id}</td>
                    <td className="px-5 py-3 text-sm text-text-muted font-mono">{entry.ip_address}</td>
                    <td className="px-5 py-3 text-sm text-text-muted">{new Date(entry.created_at).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}
