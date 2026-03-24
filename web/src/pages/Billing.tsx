import { Zap, CheckCircle, TrendingUp, Crown } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useApi } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';

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

interface UsageData {
  apps_used: number;
  apps_limit: number;
  containers_used: number;
  containers_limit: number;
  ram_used_mb: number;
  ram_limit_mb: number;
  plan: { id: string; name: string };
  quota: { apps_ok: boolean; containers_ok: boolean; ram_ok: boolean };
}

function formatPrice(cents: number) {
  if (cents === 0) return 'Free';
  return `$${(cents / 100).toFixed(0)}`;
}

function formatLimit(val: number) {
  return val < 0 ? 'Unlimited' : String(val);
}

function UsageBar({ used, limit, label }: { used: number; limit: number; label: string }) {
  const isUnlimited = limit < 0;
  const pct = isUnlimited ? 0 : Math.min((used / limit) * 100, 100);
  const isWarning = !isUnlimited && pct >= 80;
  const isDanger = !isUnlimited && pct >= 95;

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between text-sm">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium">
          {used} / {isUnlimited ? '\u221E' : limit}
        </span>
      </div>
      <div className="h-2 rounded-full bg-muted overflow-hidden">
        <div
          className={cn(
            'h-full rounded-full transition-all',
            isDanger ? 'bg-destructive' : isWarning ? 'bg-amber-500' : 'bg-primary',
          )}
          style={{ width: isUnlimited ? '0%' : `${pct}%` }}
        />
      </div>
    </div>
  );
}

export function Billing() {
  const { data: plans, loading: plansLoading } = useApi<Plan[]>('/billing/plans');
  const { data: usage, loading: usageLoading } = useApi<UsageData>('/billing/usage');

  const currentPlanId = usage?.plan?.id || 'free';

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Billing & Plans</h1>
        <p className="text-sm text-muted-foreground mt-1">Manage your subscription and view usage</p>
      </div>

      {/* Current Usage */}
      {usageLoading && (
        <Card>
          <CardContent className="space-y-4">
            <Skeleton className="h-6 w-40" />
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-full" />
          </CardContent>
        </Card>
      )}

      {usage && (
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <TrendingUp size={18} className="text-primary" />
                <CardTitle>Current Usage</CardTitle>
              </div>
              <Badge className="bg-primary/10 text-primary border-primary/20">
                <Crown size={12} /> {usage.plan?.name || 'Free'}
              </Badge>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <UsageBar
              used={usage.apps_used || 0}
              limit={usage.apps_limit || -1}
              label="Applications"
            />
            <UsageBar
              used={usage.containers_used || 0}
              limit={usage.containers_limit || -1}
              label="Containers"
            />
            <UsageBar
              used={usage.ram_used_mb || 0}
              limit={usage.ram_limit_mb || -1}
              label="RAM (MB)"
            />
          </CardContent>
        </Card>
      )}

      {/* Plans Comparison */}
      <div>
        <h2 className="text-lg font-semibold mb-4">Available Plans</h2>
        {plansLoading && (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {[1, 2, 3, 4].map((i) => (
              <Card key={i}>
                <CardContent className="space-y-3">
                  <Skeleton className="h-6 w-20" />
                  <Skeleton className="h-10 w-24" />
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-full" />
                </CardContent>
              </Card>
            ))}
          </div>
        )}

        {(plans || []).length > 0 && (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {(plans || []).map((plan) => {
              const isCurrent = plan.id === currentPlanId;

              return (
                <Card
                  key={plan.id}
                  className={cn(
                    'relative',
                    isCurrent && 'border-primary ring-2 ring-primary/20',
                  )}
                >
                  {isCurrent && (
                    <div className="absolute -top-3 left-1/2 -translate-x-1/2">
                      <Badge>Current Plan</Badge>
                    </div>
                  )}
                  <CardHeader className="text-center">
                    <CardTitle>{plan.name}</CardTitle>
                    <CardDescription>{plan.description}</CardDescription>
                  </CardHeader>
                  <CardContent className="text-center space-y-4">
                    <div>
                      <span className="text-4xl font-bold">{formatPrice(plan.price_cents)}</span>
                      {plan.price_cents > 0 && (
                        <span className="text-muted-foreground text-sm">/mo</span>
                      )}
                    </div>
                    <Separator />
                    <ul className="space-y-2.5 text-sm text-left">
                      <li className="flex items-center gap-2">
                        <Zap size={14} className="text-primary shrink-0" />
                        <span>{formatLimit(plan.max_apps)} apps</span>
                      </li>
                      <li className="flex items-center gap-2">
                        <Zap size={14} className="text-primary shrink-0" />
                        <span>{formatLimit(plan.max_containers)} containers</span>
                      </li>
                      <li className="flex items-center gap-2">
                        <Zap size={14} className="text-primary shrink-0" />
                        <span>
                          {plan.max_ram_mb < 0
                            ? 'Unlimited'
                            : `${(plan.max_ram_mb / 1024).toFixed(0)} GB`}{' '}
                          RAM
                        </span>
                      </li>
                      {(plan.features || []).map((feat, i) => (
                        <li key={i} className="flex items-center gap-2">
                          <CheckCircle size={14} className="text-emerald-500 shrink-0" />
                          <span>{feat}</span>
                        </li>
                      ))}
                    </ul>
                  </CardContent>
                  <CardFooter className="justify-center">
                    {isCurrent ? (
                      <Button variant="outline" disabled className="w-full">
                        Current Plan
                      </Button>
                    ) : (
                      <Button
                        variant={plan.price_cents === 0 ? 'outline' : 'default'}
                        className="w-full"
                      >
                        {plan.price_cents === 0 ? 'Downgrade' : 'Upgrade'}
                      </Button>
                    )}
                  </CardFooter>
                </Card>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
