import {
  Zap,
  CheckCircle2,
  XCircle,
  TrendingUp,
  Crown,
  CreditCard,
  Sparkles,
  ArrowRight,
} from 'lucide-react';
import type { Plan, UsageData } from '@/api/billing';
import { cn } from '@/lib/utils';
import { useApi } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const PLAN_COLORS: Record<string, { gradient: string; border: string; badge: string; icon: string }> = {
  free:       { gradient: 'from-slate-500/10 to-slate-500/5',     border: 'border-slate-500/20',     badge: 'bg-slate-500/10 text-slate-600 dark:text-slate-400',     icon: 'text-slate-500' },
  pro:        { gradient: 'from-blue-500/10 to-blue-500/5',       border: 'border-blue-500/20',       badge: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',       icon: 'text-blue-500' },
  business:   { gradient: 'from-purple-500/10 to-purple-500/5',   border: 'border-purple-500/20',     badge: 'bg-purple-500/10 text-purple-600 dark:text-purple-400', icon: 'text-purple-500' },
  enterprise: { gradient: 'from-amber-500/10 to-amber-500/5',     border: 'border-amber-500/20',     badge: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',   icon: 'text-amber-500' },
};

const DEFAULT_PLAN_COLOR = PLAN_COLORS.free;

function getPlanColor(planId: string) {
  return PLAN_COLORS[planId.toLowerCase()] || DEFAULT_PLAN_COLOR;
}

function formatPrice(cents: number) {
  if (cents === 0) return 'Free';
  return `$${(cents / 100).toFixed(0)}`;
}

function formatLimit(val: number) {
  return val < 0 ? 'Unlimited' : String(val);
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function UsageBar({ used, limit, label }: { used: number; limit: number; label: string }) {
  const isUnlimited = limit < 0;
  const pct = isUnlimited ? 0 : Math.min((used / limit) * 100, 100);
  const barColor = isUnlimited
    ? 'bg-emerald-500'
    : pct >= 80
      ? 'bg-red-500'
      : pct >= 50
        ? 'bg-amber-500'
        : 'bg-emerald-500';

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between text-sm">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium tabular-nums">
          {used} / {isUnlimited ? '\u221E' : limit}
        </span>
      </div>
      <div className="h-2 rounded-full bg-muted overflow-hidden">
        <div
          className={cn('h-full rounded-full transition-all duration-500', barColor)}
          style={{ width: isUnlimited ? '0%' : `${pct}%` }}
        />
      </div>
      {!isUnlimited && pct >= 80 && (
        <p className="text-[11px] text-red-600 dark:text-red-400 font-medium">
          {pct >= 95 ? 'Limit almost reached' : 'Approaching limit'} &mdash; consider upgrading
        </p>
      )}
    </div>
  );
}

function PlanCardSkeleton() {
  return (
    <Card className="py-5">
      <CardHeader className="text-center pb-0">
        <Skeleton className="h-5 w-20 mx-auto" />
        <Skeleton className="h-3 w-32 mx-auto mt-2" />
      </CardHeader>
      <CardContent className="text-center space-y-4 pt-4">
        <Skeleton className="h-10 w-16 mx-auto" />
        <Separator />
        <div className="space-y-2.5">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-3.5 w-full" />
          ))}
        </div>
      </CardContent>
      <CardFooter className="justify-center pt-0">
        <Skeleton className="h-9 w-full rounded-md" />
      </CardFooter>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Billing
// ---------------------------------------------------------------------------

export function Billing() {
  const { data: plans, loading: plansLoading } = useApi<Plan[]>('/billing/plans');
  const { data: usage, loading: usageLoading } = useApi<UsageData>('/billing/usage');

  const currentPlanId = usage?.plan?.id || 'free';

  return (
    <div className="space-y-8">
      {/* Hero Section */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10">
          <div className="flex items-center gap-2 mb-2">
            <CreditCard className="size-5 text-primary" />
            <Badge variant="secondary" className="text-xs font-normal">
              {usage?.plan?.name || 'Free'} Plan
            </Badge>
          </div>
          <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
            Billing &amp; Plans
          </h1>
          <p className="text-muted-foreground mt-1.5 text-sm sm:text-base max-w-lg">
            Manage your subscription, monitor resource usage, and explore available plans.
          </p>
        </div>
        {/* Decorative */}
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
        <div className="pointer-events-none absolute -left-8 -bottom-8 size-48 rounded-full bg-primary/3 blur-2xl" />
      </div>

      {/* Current Usage Card */}
      {usageLoading && (
        <Card>
          <CardHeader>
            <Skeleton className="h-6 w-40" />
          </CardHeader>
          <CardContent className="space-y-5">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="space-y-2">
                <div className="flex justify-between">
                  <Skeleton className="h-3.5 w-24" />
                  <Skeleton className="h-3.5 w-16" />
                </div>
                <Skeleton className="h-2 w-full rounded-full" />
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      {usage && (
        <Card className="overflow-hidden">
          <div className={cn(
            'bg-gradient-to-r p-6',
            getPlanColor(currentPlanId).gradient
          )}>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="flex items-center justify-center rounded-xl size-11 bg-background/80 backdrop-blur-sm">
                  <Crown className={cn('size-5', getPlanColor(currentPlanId).icon)} />
                </div>
                <div>
                  <h2 className="font-semibold text-foreground">
                    {usage.plan?.name || 'Free'} Plan
                  </h2>
                  <p className="text-sm text-muted-foreground">Current subscription</p>
                </div>
              </div>
              <Badge className={cn('text-xs font-medium', getPlanColor(currentPlanId).badge)}>
                <TrendingUp className="size-3" />
                Active
              </Badge>
            </div>
          </div>
          <CardContent className="space-y-5 pt-6">
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

      {/* Plan Comparison */}
      <div>
        <div className="flex items-center gap-2 mb-5">
          <Sparkles className="size-5 text-primary" />
          <h2 className="text-lg font-semibold text-foreground">Available Plans</h2>
        </div>

        {plansLoading && (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <PlanCardSkeleton key={i} />
            ))}
          </div>
        )}

        {(plans || []).length > 0 && (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {(plans || []).map((plan) => {
              const isCurrent = plan.id === currentPlanId;
              const planColor = getPlanColor(plan.id);
              const isUpgrade = plan.price_cents > 0 && !isCurrent;
              const isDowngrade = plan.price_cents === 0 && !isCurrent;

              return (
                <Card
                  key={plan.id}
                  className={cn(
                    'relative group transition-all duration-200 hover:translate-y-[-1px] hover:shadow-lg hover:ring-2 hover:ring-primary/20',
                    isCurrent && cn('ring-2', planColor.border, 'ring-primary/30')
                  )}
                >
                  {/* Current plan badge */}
                  {isCurrent && (
                    <div className="absolute -top-3 left-1/2 -translate-x-1/2 z-10">
                      <Badge className="bg-primary text-primary-foreground shadow-md">
                        Current Plan
                      </Badge>
                    </div>
                  )}

                  {/* Gradient header */}
                  <div className={cn('rounded-t-xl bg-gradient-to-r p-4 text-center', planColor.gradient)}>
                    <CardTitle className="text-lg">{plan.name}</CardTitle>
                    <CardDescription className="mt-1 text-xs">{plan.description}</CardDescription>
                  </div>

                  <CardContent className="text-center space-y-4 pt-5">
                    <div>
                      <span className="text-4xl font-bold tracking-tight">
                        {formatPrice(plan.price_cents)}
                      </span>
                      {plan.price_cents > 0 && (
                        <span className="text-muted-foreground text-sm">/mo</span>
                      )}
                    </div>

                    <Separator />

                    <ul className="space-y-2.5 text-sm text-left">
                      <li className="flex items-center gap-2">
                        <Zap className={cn('size-3.5 shrink-0', planColor.icon)} />
                        <span>{formatLimit(plan.max_apps)} apps</span>
                      </li>
                      <li className="flex items-center gap-2">
                        <Zap className={cn('size-3.5 shrink-0', planColor.icon)} />
                        <span>{formatLimit(plan.max_containers)} containers</span>
                      </li>
                      <li className="flex items-center gap-2">
                        <Zap className={cn('size-3.5 shrink-0', planColor.icon)} />
                        <span>
                          {plan.max_ram_mb < 0
                            ? 'Unlimited'
                            : `${(plan.max_ram_mb / 1024).toFixed(0)} GB`}{' '}
                          RAM
                        </span>
                      </li>
                      {(plan.features || []).map((feat, i) => (
                        <li key={i} className="flex items-center gap-2">
                          <CheckCircle2 className="size-3.5 text-emerald-500 shrink-0" />
                          <span>{feat}</span>
                        </li>
                      ))}
                    </ul>
                  </CardContent>

                  <CardFooter className="justify-center pb-5">
                    {isCurrent ? (
                      <Button variant="outline" disabled className="w-full">
                        <CheckCircle2 className="size-4" />
                        Current Plan
                      </Button>
                    ) : isUpgrade ? (
                      <Button className="w-full">
                        <ArrowRight className="size-4" />
                        Upgrade
                      </Button>
                    ) : isDowngrade ? (
                      <Button variant="outline" className="w-full">
                        <XCircle className="size-4" />
                        Downgrade
                      </Button>
                    ) : (
                      <Button variant="outline" className="w-full">
                        Select
                      </Button>
                    )}
                  </CardFooter>
                </Card>
              );
            })}
          </div>
        )}

        {!plansLoading && (plans || []).length === 0 && (
          <div className="flex flex-col items-center justify-center py-24 text-center">
            <div className="rounded-full bg-muted p-6 mb-5">
              <CreditCard className="size-10 text-muted-foreground" />
            </div>
            <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">
              No plans available
            </h2>
            <p className="text-muted-foreground max-w-sm text-sm">
              Plans will appear here once configured by your administrator.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
