import { useEffect, useState } from 'react';
import { Loader2, CheckCircle, XCircle, Rocket, FileCode, Server, Globe } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Progress } from '@/components/ui/progress';
import { cn } from '@/lib/utils';

interface DeployModalProps {
  open: boolean;
  onClose: () => void;
  result: DeployResult | null;
  isDeploying: boolean;
  status: DeployStatus;
}

interface DeployResult {
  success: boolean;
  message: string;
  createdResources?: {
    apps: string[];
    databases: string[];
    domains: string[];
    volumes: string[];
  };
  containers?: string[];
  networks?: string[];
  volumes?: string[];
  duration?: string;
  errors?: string[];
}

export type DeployStatus = 'idle' | 'validating' | 'compiling' | 'building' | 'deploying' | 'success' | 'error';

const STAGES: { key: DeployStatus; label: string; icon: typeof Rocket }[] = [
  { key: 'validating', label: 'Validating topology', icon: FileCode },
  { key: 'compiling', label: 'Generating configuration', icon: FileCode },
  { key: 'building', label: 'Building images', icon: Server },
  { key: 'deploying', label: 'Starting services', icon: Rocket },
];

export function DeployModal({ open, onClose, result, isDeploying, status }: DeployModalProps) {
  const [currentStageIndex, setCurrentStageIndex] = useState(0);

  useEffect(() => {
    if (!open) {
      setCurrentStageIndex(0);
      return;
    }

    // Update stage index based on status
    const stageIndex = STAGES.findIndex(s => s.key === status);
    if (stageIndex >= 0) {
      setCurrentStageIndex(stageIndex);
    }
  }, [open, status]);

  const getProgress = () => {
    if (status === 'success' || status === 'error') return 100;
    const stageIndex = STAGES.findIndex(s => s.key === status);
    if (stageIndex < 0) return 0;
    return ((stageIndex + 1) / STAGES.length) * 100;
  };

  const renderStageIndicator = () => (
    <div className="space-y-3">
      {STAGES.map((stage, index) => {
        const isActive = status === stage.key;
        const isComplete = currentStageIndex > index || status === 'success';
        const isPending = currentStageIndex < index;

        return (
          <div
            key={stage.key}
            className={cn(
              'flex items-center gap-3 p-3 rounded-lg transition-colors',
              isActive && 'bg-primary/10 border border-primary/30',
              isComplete && 'bg-green-500/10',
              isPending && 'opacity-50'
            )}
          >
            <div
              className={cn(
                'flex h-8 w-8 items-center justify-center rounded-full',
                isActive && 'bg-primary text-primary-foreground',
                isComplete && 'bg-green-500 text-white',
                isPending && 'bg-muted'
              )}
            >
              {isComplete ? (
                <CheckCircle className="h-4 w-4" />
              ) : isActive ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <stage.icon className="h-4 w-4" />
              )}
            </div>
            <span className={cn('text-sm font-medium', isActive && 'text-primary')}>
              {stage.label}
            </span>
          </div>
        );
      })}
    </div>
  );

  const renderResult = () => {
    if (!result) return null;

    if (result.success) {
      return (
        <div className="space-y-4">
          <div className="flex items-center gap-3 p-4 rounded-lg bg-green-500/10 border border-green-500/30">
            <CheckCircle className="h-6 w-6 text-green-500" />
            <div>
              <p className="font-medium text-green-600">Deployment Successful</p>
              <p className="text-sm text-green-500">{result.message}</p>
            </div>
          </div>

          {result.duration && (
            <p className="text-sm text-muted-foreground">
              Completed in {result.duration}
            </p>
          )}

          {result.createdResources && (
            <div className="space-y-2">
              <h4 className="text-sm font-medium">Created Resources</h4>

              {result.createdResources.apps.length > 0 && (
                <div className="flex items-center gap-2">
                  <Server className="h-4 w-4 text-blue-500" />
                  <span className="text-sm">Apps: {result.createdResources.apps.join(', ')}</span>
                </div>
              )}

              {result.createdResources.databases.length > 0 && (
                <div className="flex items-center gap-2">
                  <Server className="h-4 w-4 text-green-500" />
                  <span className="text-sm">Databases: {result.createdResources.databases.join(', ')}</span>
                </div>
              )}

              {result.createdResources.domains.length > 0 && (
                <div className="flex items-center gap-2">
                  <Globe className="h-4 w-4 text-purple-500" />
                  <span className="text-sm">Domains: {result.createdResources.domains.join(', ')}</span>
                </div>
              )}
            </div>
          )}

          <div className="flex gap-2 pt-4">
            <Button variant="outline" onClick={onClose}>
              Close
            </Button>
            <Button onClick={() => window.location.href = '/apps'}>
              View Applications
            </Button>
          </div>
        </div>
      );
    }

    return (
      <div className="space-y-4">
        <div className="flex items-center gap-3 p-4 rounded-lg bg-red-500/10 border border-red-500/30">
          <XCircle className="h-6 w-6 text-red-500" />
          <div>
            <p className="font-medium text-red-600">Deployment Failed</p>
            <p className="text-sm text-red-500">{result.message}</p>
          </div>
        </div>

        {result.errors && result.errors.length > 0 && (
          <div className="space-y-2">
            <h4 className="text-sm font-medium">Errors</h4>
            <ul className="space-y-1">
              {result.errors.map((error, i) => (
                <li key={i} className="text-sm text-red-500 flex items-start gap-2">
                  <span className="text-red-400">•</span>
                  {error}
                </li>
              ))}
            </ul>
          </div>
        )}

        <div className="flex gap-2 pt-4">
          <Button variant="outline" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    );
  };

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Rocket className="h-5 w-5" />
            Deploy Topology
          </DialogTitle>
        </DialogHeader>

        <div className="py-4">
          {isDeploying ? (
            <div className="space-y-4">
              <Progress value={getProgress()} className="h-2" />
              {renderStageIndicator()}
              <p className="text-center text-sm text-muted-foreground">
                Please wait while your topology is being deployed...
              </p>
            </div>
          ) : result ? (
            renderResult()
          ) : (
            <div className="flex items-center justify-center py-8 text-muted-foreground">
              No deployment result
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
