import { TrendingUp, Zap } from 'lucide-react';
import { useApi } from '../hooks';

interface Plan {
  id: string;
  name: string;
  description: string;
  price_cents: number;
  currency: string;
  max_apps: number;
  max_containers: number;
  max_ram_mb: number;
  features: string[];
}

export function Billing() {
  const { data: plans } = useApi<Plan[]>('/billing/plans');
  const { data: usage } = useApi<any>('/billing/usage');

  const formatPrice = (cents: number) => {
    if (cents === 0) return 'Free';
    return `$${(cents / 100).toFixed(0)}/mo`;
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-text-primary">Billing & Plans</h1>
        <p className="text-sm text-text-secondary mt-1">Manage your subscription and view usage</p>
      </div>

      {/* Current Usage */}
      {usage && (
        <div className="bg-surface border border-border rounded-xl p-6">
          <h2 className="font-medium text-text-primary mb-4 flex items-center gap-2"><TrendingUp size={18} /> Current Usage</h2>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div className="bg-surface-secondary rounded-lg p-4">
              <p className="text-sm text-text-secondary">Applications</p>
              <p className="text-2xl font-semibold text-text-primary">{usage.apps_used || 0} <span className="text-sm text-text-muted">/ {usage.apps_limit || '∞'}</span></p>
            </div>
            <div className="bg-surface-secondary rounded-lg p-4">
              <p className="text-sm text-text-secondary">Plan</p>
              <p className="text-2xl font-semibold text-monster-green">{usage.plan?.name || 'Free'}</p>
            </div>
            <div className="bg-surface-secondary rounded-lg p-4">
              <p className="text-sm text-text-secondary">Status</p>
              <p className="text-2xl font-semibold text-status-running">{usage.quota?.apps_ok ? 'OK' : 'At Limit'}</p>
            </div>
          </div>
        </div>
      )}

      {/* Plans */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {(plans || []).map((plan) => (
          <div key={plan.id} className={`bg-surface border rounded-xl p-6 ${
            plan.id === (usage?.plan?.id || 'free')
              ? 'border-monster-green ring-2 ring-monster-green/20'
              : 'border-border'
          }`}>
            <div className="flex items-center justify-between mb-3">
              <h3 className="font-semibold text-text-primary">{plan.name}</h3>
              {plan.id === (usage?.plan?.id || 'free') && (
                <span className="text-xs bg-monster-green/10 text-monster-green px-2 py-0.5 rounded-full">Current</span>
              )}
            </div>
            <p className="text-3xl font-bold text-text-primary mb-1">{formatPrice(plan.price_cents)}</p>
            <p className="text-sm text-text-secondary mb-4">{plan.description}</p>
            <ul className="space-y-2 text-sm text-text-secondary">
              <li className="flex items-center gap-2"><Zap size={14} className="text-monster-green" /> {plan.max_apps < 0 ? 'Unlimited' : plan.max_apps} apps</li>
              <li className="flex items-center gap-2"><Zap size={14} className="text-monster-green" /> {plan.max_containers < 0 ? 'Unlimited' : plan.max_containers} containers</li>
              <li className="flex items-center gap-2"><Zap size={14} className="text-monster-green" /> {plan.max_ram_mb < 0 ? 'Unlimited' : (plan.max_ram_mb / 1024).toFixed(0) + 'GB'} RAM</li>
            </ul>
            {plan.id !== (usage?.plan?.id || 'free') && (
              <button className="w-full mt-4 py-2 border border-monster-green text-monster-green text-sm font-medium rounded-lg hover:bg-monster-green/5 transition-colors">
                {plan.price_cents === 0 ? 'Downgrade' : 'Upgrade'}
              </button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
