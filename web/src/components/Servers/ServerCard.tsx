import { Monitor, MapPin, Cpu, Clock } from 'lucide-react';
import type { ServerNode } from '@/api/servers';
import { Badge } from '@/components/ui/badge';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { cn, getProviderConfig } from './helpers';
import { timeAgo } from './helpers';

interface ServerCardProps {
  server: ServerNode;
}

export function ServerCard({ server }: ServerCardProps) {
  const providerCfg = getProviderConfig(server.provider);
  const isConnected = server.connected === true;

  return (
    <Card className="group transition-all duration-200 hover:translate-y-[-2px] hover:shadow-lg">
      <CardHeader className="gap-4">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className={cn(
              'flex items-center justify-center rounded-xl size-11 shrink-0',
              providerCfg.bgColor
            )}>
              <span className={cn('text-sm font-bold', providerCfg.textColor)}>
                {providerCfg.letter}
              </span>
            </div>
            <div>
              <CardTitle className="text-base">{server.hostname}</CardTitle>
              <CardDescription className="font-mono text-xs">{server.ip_address}</CardDescription>
            </div>
          </div>
          {isConnected ? (
            <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400 gap-1.5">
              <span className="relative flex size-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
                <span className="relative inline-flex rounded-full size-2 bg-emerald-500" />
              </span>
              Connected
            </Badge>
          ) : (
            <Badge variant="secondary" className="gap-1.5">
              <span className="size-2 rounded-full bg-muted-foreground" />
              {server.agent_status === 'disconnected' ? 'Agent offline' : server.status.charAt(0).toUpperCase() + server.status.slice(1)}
            </Badge>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-3 flex-wrap text-sm text-muted-foreground">
          <Badge variant="outline" className={cn('text-xs font-normal', providerCfg.badgeColor)}>
            {providerCfg.name}
          </Badge>
          {server.region && (
            <span className="flex items-center gap-1 text-xs">
              <MapPin className="size-3" /> {server.region}
            </span>
          )}
          {server.size && (
            <span className="flex items-center gap-1 text-xs">
              <Cpu className="size-3" /> {server.size}
            </span>
          )}
          {server.role && (
            <Badge variant="secondary" className="text-xs font-normal">
              {server.role.charAt(0).toUpperCase() + server.role.slice(1)}
            </Badge>
          )}
        </div>
      </CardContent>
      <CardFooter className="border-t pt-4 pb-0">
        <span className="flex items-center gap-1.5 text-xs text-muted-foreground tabular-nums">
          <Clock className="size-3" />
          Added {timeAgo(server.created_at)}
        </span>
      </CardFooter>
    </Card>
  );
}

export function LocalhostCard() {
  return (
    <Card className="group transition-all duration-200 hover:translate-y-[-1px] hover:shadow-md">
      <CardHeader className="gap-4">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="flex items-center justify-center rounded-xl size-11 shrink-0 bg-emerald-500/10">
              <Monitor className="size-5 text-emerald-500" />
            </div>
            <div>
              <CardTitle className="text-base">localhost</CardTitle>
              <CardDescription className="font-mono text-xs">127.0.0.1</CardDescription>
            </div>
          </div>
          <Badge className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:text-emerald-400 gap-1.5">
            <span className="relative flex size-2">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
              <span className="relative inline-flex rounded-full size-2 bg-emerald-500" />
            </span>
            Active
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <Badge variant="outline" className="gap-1 text-xs font-normal">
            <Cpu className="size-3" /> Local
          </Badge>
          <Badge variant="secondary" className="text-xs font-normal">Master Node</Badge>
        </div>
      </CardContent>
    </Card>
  );
}