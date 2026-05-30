import { GitBranch, Server, Calendar, Rocket, Upload, User, CheckCircle2 } from 'lucide-react';
import { type App } from '@/api/apps';
import { type Deployment } from '@/api/deployments';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { AppStatsCards, getStatusConfig, timeAgo, type AppStatsResponse } from './AppStatsCards';

interface AppOverviewSectionProps {
  app: App;
  stats: AppStatsResponse | null;
  statsError: string | null;
  deployments: Deployment[];
  onDeploy: () => void;
}

export function AppOverviewSection({ app, stats, statsError, deployments, onDeploy }: AppOverviewSectionProps) {
  return (
    <div className="space-y-6">
      {statsError && (
        <div className="rounded-md border border-amber-500/30 bg-amber-500/5 px-4 py-2 text-sm text-amber-700 dark:text-amber-400">
          Live metrics unavailable: {statsError}
        </div>
      )}
      <AppStatsCards stats={stats} app={app} statsError={statsError} />

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Application Info */}
        <div className="rounded-lg border bg-card p-6">
          <h2 className="text-base font-semibold mb-4">Application Info</h2>
          <div className="space-y-4">
            {[
              { icon: GitBranch, label: 'Source', value: app.source_type },
              { icon: GitBranch, label: 'Branch', value: app.branch || 'main' },
              { icon: Server, label: 'Replicas', value: String(app.replicas) },
              {
                icon: Calendar,
                label: 'Created',
                value: new Date(app.created_at).toLocaleDateString('en-US', {
                  year: 'numeric',
                  month: 'short',
                  day: 'numeric',
                }),
              },
            ].map(({ icon: Icon, label, value }) => (
              <div key={label} className="flex items-center justify-between text-sm">
                <span className="flex items-center gap-2 text-muted-foreground">
                  <Icon className="size-4" />
                  {label}
                </span>
                <span className="font-medium">{value}</span>
              </div>
            ))}
            {app.source_url && (
              <div className="text-sm pt-2 border-t">
                <p className="text-muted-foreground mb-1.5">Repository URL</p>
                <p className="font-mono text-xs bg-muted rounded-md px-3 py-2 truncate">
                  {app.source_url}
                </p>
              </div>
            )}
          </div>
        </div>

        {/* Latest Deployment */}
        <div className="rounded-lg border bg-card p-6">
          <h2 className="text-base font-semibold mb-4">Latest Deployment</h2>
          {deployments.length > 0 ? (
            <div className="space-y-4">
              {[
                { icon: Rocket, label: 'Version', value: `v${deployments[0].version}` },
                {
                  icon: CheckCircle2,
                  label: 'Status',
                  value: deployments[0].status,
                  isBadge: true,
                },
                { icon: User, label: 'Triggered by', value: deployments[0].triggered_by },
                { icon: Server, label: 'Date', value: timeAgo(deployments[0].created_at) },
              ].map(({ icon: Icon, label, value, isBadge }) => (
                <div key={label} className="flex items-center justify-between text-sm">
                  <span className="flex items-center gap-2 text-muted-foreground">
                    <Icon className="size-4" />
                    {label}
                  </span>
                  {isBadge ? (
                    <Badge variant={getStatusConfig(value).variant}>
                      {getStatusConfig(value).label}
                    </Badge>
                  ) : (
                    <span className="font-medium">{value}</span>
                  )}
                </div>
              ))}
              {deployments[0].commit_sha && (
                <div className="text-sm pt-2 border-t">
                  <p className="text-muted-foreground mb-1.5">Commit SHA</p>
                  <code className="font-mono text-xs bg-muted rounded-md px-3 py-2 block">
                    {deployments[0].commit_sha.slice(0, 8)}
                  </code>
                </div>
              )}
            </div>
          ) : (
            <div className="flex flex-col items-center justify-center py-8 text-center">
              <div className="rounded-full bg-muted p-3 mb-3">
                <Rocket className="size-5 text-muted-foreground" />
              </div>
              <p className="text-sm text-muted-foreground mb-3">No deployments yet</p>
              <Button size="sm" onClick={onDeploy} className="cursor-pointer">
                <Upload className="size-4" />
                Deploy Now
              </Button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}