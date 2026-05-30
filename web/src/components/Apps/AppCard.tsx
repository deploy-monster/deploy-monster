import { Link } from 'react-router';
import { Clock, GitBranch, Container, Play, Square, RotateCcw, Trash2 } from 'lucide-react';
import type { App } from '@/api/apps';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Tooltip } from '@/components/ui/tooltip';
import { cn, getStatusConfig, getSourceConfig, timeAgo } from './helpers';

interface AppCardProps {
  app: App;
  isLoading: boolean;
  onAction: (e: React.MouseEvent, appId: string, action: 'start' | 'stop' | 'restart' | 'delete') => void;
}

export function AppCard({ app, isLoading, onAction }: AppCardProps) {
  const statusCfg = getStatusConfig(app.status);
  const sourceCfg = getSourceConfig(app.source_type);
  const SourceIcon = sourceCfg.icon;

  return (
    <Link to={`/apps/${app.id}`} className="block group">
      <Card
        className={cn(
          'relative transition-all duration-200 h-full',
          'hover:ring-1 hover:ring-primary/20 hover:shadow-md hover:-translate-y-px',
          isLoading && 'opacity-60 pointer-events-none'
        )}
      >
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between gap-3">
            <div className="flex items-center gap-3 min-w-0 flex-1">
              {/* Status dot with animation */}
              <span className="relative flex h-2.5 w-2.5 shrink-0">
                {app.status === 'running' && (
                  <span
                    className={cn(
                      'absolute inline-flex h-full w-full rounded-full opacity-75 animate-ping',
                      statusCfg.dot
                    )}
                  />
                )}
                <span
                  className={cn(
                    'relative inline-flex rounded-full h-2.5 w-2.5',
                    statusCfg.dot
                  )}
                />
              </span>
              <CardTitle className="text-base truncate">{app.name}</CardTitle>
            </div>
            {/* Source badge */}
            <span
              className={cn(
                'inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium shrink-0',
                sourceCfg.color
              )}
            >
              <SourceIcon className="size-3" />
              {sourceCfg.label}
            </span>
          </div>
        </CardHeader>

        <CardContent className="pb-3 space-y-3">
          {/* Status badge */}
          <Badge variant={statusCfg.variant} className="text-xs">
            {statusCfg.label}
          </Badge>

          {/* Meta info */}
          <div className="space-y-1.5 text-xs text-muted-foreground">
            <div className="flex items-center gap-1.5">
              <Clock className="size-3 shrink-0" />
              <span>Last deploy: {timeAgo(app.updated_at)}</span>
            </div>
            {app.branch && (
              <div className="flex items-center gap-1.5">
                <GitBranch className="size-3 shrink-0" />
                <span className="truncate">{app.branch}</span>
              </div>
            )}
            <div className="flex items-center gap-1.5">
              <Container className="size-3 shrink-0" />
              <span className="font-mono text-[11px] truncate">
                {app.id.slice(0, 12)}
              </span>
            </div>
          </div>
        </CardContent>

        {/* Action row */}
        <div className="border-t px-4 py-2.5 flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
          {app.status === 'running' ? (
            <Tooltip content="Stop">
              <Button
                variant="ghost"
                size="icon"
                className="size-7 cursor-pointer"
                onClick={(e) => onAction(e, app.id, 'stop')}
              >
                <Square className="size-3.5" />
              </Button>
            </Tooltip>
          ) : (
            <Tooltip content="Start">
              <Button
                variant="ghost"
                size="icon"
                className="size-7 cursor-pointer"
                onClick={(e) => onAction(e, app.id, 'start')}
              >
                <Play className="size-3.5" />
              </Button>
            </Tooltip>
          )}
          <Tooltip content="Restart">
            <Button
              variant="ghost"
              size="icon"
              className="size-7 cursor-pointer"
              onClick={(e) => onAction(e, app.id, 'restart')}
            >
              <RotateCcw className="size-3.5" />
            </Button>
          </Tooltip>
          <Tooltip content="Delete">
            <Button
              variant="ghost"
              size="icon"
              className="size-7 text-destructive hover:text-destructive cursor-pointer"
              onClick={(e) => onAction(e, app.id, 'delete')}
            >
              <Trash2 className="size-3.5" />
            </Button>
          </Tooltip>
        </div>
      </Card>
    </Link>
  );
}