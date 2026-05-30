import { Link } from 'react-router';
import { ArrowLeft, Play, Square, RotateCcw, Trash2, GitBranch, Upload } from 'lucide-react';
import { type App } from '@/api/apps';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { cn, getStatusConfig } from './AppStatsCards';

interface AppHeaderProps {
  app: App;
  actionLoading: string | null;
  onAction: (action: 'start' | 'stop' | 'restart') => void;
  onDeploy: () => void;
  onDeleteRequested: () => void;
}

export function AppHeader({ app, actionLoading, onAction, onDeploy, onDeleteRequested }: AppHeaderProps) {
  const statusCfg = getStatusConfig(app.status);
  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div className="flex items-start gap-4">
        <Link to="/apps">
          <Button variant="ghost" size="icon" aria-label="Back to applications" className="mt-1 cursor-pointer">
            <ArrowLeft className="size-4" />
          </Button>
        </Link>
        <div>
          <div className="flex items-center gap-3 flex-wrap">
            <h1 className="text-3xl font-bold tracking-tight">{app.name}</h1>
            <Badge variant={statusCfg.variant} className="text-xs gap-1.5">
              <span className="relative flex h-2 w-2">
                {app.status === 'running' && (
                  <span
                    className={cn(
                      'absolute inline-flex h-full w-full rounded-full opacity-75 animate-ping',
                      statusCfg.dot
                    )}
                  />
                )}
                <span className={cn('relative inline-flex rounded-full h-2 w-2', statusCfg.dot)} />
              </span>
              {statusCfg.label}
            </Badge>
          </div>
          <p className="text-muted-foreground mt-1 text-sm">
            {app.source_type} &middot; {app.type}
            {app.branch && (
              <span className="inline-flex items-center gap-1 ml-2">
                <GitBranch className="size-3" />
                {app.branch}
              </span>
            )}
          </p>
        </div>
      </div>

      {/* Action buttons */}
      <div className="flex items-center gap-2 ml-12 sm:ml-0">
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer"
          onClick={() => onAction('restart')}
          disabled={actionLoading !== null}
        >
          <RotateCcw className={cn('size-4', actionLoading === 'restart' && 'animate-spin')} />
          Restart
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer"
          onClick={() => onAction(app.status === 'running' ? 'stop' : 'start')}
          disabled={actionLoading !== null}
        >
          {app.status === 'running' ? (
            <>
              <Square className="size-4" />
              Stop
            </>
          ) : (
            <>
              <Play className="size-4" />
              Start
            </>
          )}
        </Button>
        <Button
          size="sm"
          className="cursor-pointer"
          onClick={onDeploy}
          disabled={actionLoading !== null}
        >
          <Upload className={cn('size-4', actionLoading === 'deploy' && 'animate-bounce')} />
          Deploy
        </Button>
        <Button
          variant="destructive"
          size="sm"
          className="cursor-pointer"
          onClick={onDeleteRequested}
          disabled={actionLoading !== null}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
    </div>
  );
}