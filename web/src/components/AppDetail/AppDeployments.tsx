import { History, Rocket, Clock, Upload } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { cn, timeAgo, getStatusConfig } from './helpers';
import type { Deployment } from '@/api/deployments';

interface AppDeploymentsProps {
  deployments: Deployment[];
  onDeploy: () => void;
}

export function AppDeployments({ deployments, onDeploy }: AppDeploymentsProps) {
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <div>
          <CardTitle className="text-base">Deployment History</CardTitle>
          <CardDescription>
            {deployments.length} deployment{deployments.length !== 1 ? 's' : ''}
          </CardDescription>
        </div>
        <Button size="sm" onClick={onDeploy} className="cursor-pointer">
          <Upload className="size-4" />
          New Deployment
        </Button>
      </CardHeader>
      <CardContent>
        {deployments.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <div className="rounded-full bg-muted p-4 mb-4">
              <History className="size-8 text-muted-foreground" />
            </div>
            <h3 className="font-medium mb-1">No deployments yet</h3>
            <p className="text-sm text-muted-foreground mb-4">
              Trigger your first deployment to see the history here.
            </p>
            <Button size="sm" onClick={onDeploy} className="cursor-pointer">
              <Rocket className="size-4" />
              Deploy Now
            </Button>
          </div>
        ) : (
          <div className="rounded-lg border overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/50">
                  <TableHead className="font-semibold">Version</TableHead>
                  <TableHead className="font-semibold">Status</TableHead>
                  <TableHead className="font-semibold hidden md:table-cell">
                    Image
                  </TableHead>
                  <TableHead className="font-semibold">Commit</TableHead>
                  <TableHead className="font-semibold hidden sm:table-cell">
                    Triggered By
                  </TableHead>
                  <TableHead className="font-semibold">Date</TableHead>
                  <TableHead className="font-semibold text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {deployments.map((d, index) => {
                  const dCfg = getStatusConfig(d.status);
                  return (
                    <TableRow
                      key={d.id}
                      className="hover:bg-muted/30 transition-colors"
                    >
                      <TableCell className="font-semibold">v{d.version}</TableCell>
                      <TableCell>
                        <Badge variant={dCfg.variant} className="text-xs gap-1.5">
                          <span className={cn('inline-flex rounded-full h-1.5 w-1.5', dCfg.dot)} />
                          {dCfg.label}
                        </Badge>
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        <span className="font-mono text-xs text-muted-foreground max-w-48 truncate block">
                          {d.image}
                        </span>
                      </TableCell>
                      <TableCell>
                        <code className="font-mono text-xs bg-muted px-2 py-0.5 rounded">
                          {d.commit_sha?.slice(0, 8) || '--------'}
                        </code>
                      </TableCell>
                      <TableCell className="hidden sm:table-cell text-muted-foreground text-sm">
                        {d.triggered_by}
                      </TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        <span className="inline-flex items-center gap-1">
                          <Clock className="size-3" />
                          {timeAgo(d.created_at)}
                        </span>
                      </TableCell>
                      <TableCell className="text-right">
                        {index > 0 && (
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-7 text-xs cursor-pointer"
                          >
                            <History className="size-3" />
                            Rollback
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
