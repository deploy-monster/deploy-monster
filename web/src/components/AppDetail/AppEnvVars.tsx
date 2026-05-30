import { useState } from 'react';
import { Eye, EyeOff, Plus, Pencil, Trash2, Copy } from 'lucide-react';
import { api } from '@/api/client';
import { toast } from '@/stores/toastStore';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Separator } from '@/components/ui/separator';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Tooltip } from '@/components/ui/tooltip';
import type { EnvVar } from '@/api/deployments';

interface AppEnvVarsProps {
  appId: string;
  envVars: EnvVar[];
  onRefetch: () => void;
}

export function AppEnvVars({ appId, envVars, onRefetch }: AppEnvVarsProps) {
  const [newEnvKey, setNewEnvKey] = useState('');
  const [newEnvValue, setNewEnvValue] = useState('');
  const [showSecrets, setShowSecrets] = useState(false);
  const [, setEditingEnv] = useState<string | null>(null);
  const [envSaving, setEnvSaving] = useState(false);

  const persistEnvVars = async (next: { key: string; value: string }[]) => {
    setEnvSaving(true);
    try {
      await api.put(`/apps/${appId}/env`, { vars: next });
      onRefetch();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save environment variables');
    } finally {
      setEnvSaving(false);
    }
  };

  const addEnvVar = () => {
    const key = newEnvKey.trim();
    if (!key) return;
    if (envVars.some((v) => v.key === key)) {
      toast.error(`Variable ${key} already exists`);
      return;
    }
    const next = [...envVars.map(({ key: k, value }) => ({ key: k, value })), { key, value: newEnvValue }];
    setNewEnvKey('');
    setNewEnvValue('');
    persistEnvVars(next);
  };

  const removeEnvVar = (key: string) => {
    const next = envVars.filter((v) => v.key !== key).map(({ key: k, value }) => ({ key: k, value }));
    persistEnvVars(next);
  };

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <div>
          <CardTitle className="text-base">Environment Variables</CardTitle>
          <CardDescription>
            Manage your application's environment configuration. Use{' '}
            <code className="text-xs bg-muted px-1.5 py-0.5 rounded font-mono">
              {'${SECRET:name}'}
            </code>{' '}
            for encrypted references.
          </CardDescription>
        </div>
        <div className="flex items-center gap-2">
          {envSaving && (
            <span className="text-xs text-muted-foreground">Saving…</span>
          )}
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer"
            onClick={() => setShowSecrets(!showSecrets)}
          >
            {showSecrets ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
            {showSecrets ? 'Hide Values' : 'Reveal Values'}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Add new variable */}
        <div className="flex items-end gap-3 p-4 rounded-lg border border-dashed bg-muted/30">
          <div className="flex-1">
            <Label htmlFor="env-key" className="mb-1.5 text-xs">
              Key
            </Label>
            <Input
              id="env-key"
              placeholder="VARIABLE_NAME"
              value={newEnvKey}
              onChange={(e) => setNewEnvKey(e.target.value.toUpperCase())}
              className="font-mono text-sm"
            />
          </div>
          <div className="flex-1">
            <Label htmlFor="env-value" className="mb-1.5 text-xs">
              Value
            </Label>
            <Input
              id="env-value"
              placeholder="value or ${SECRET:name}"
              value={newEnvValue}
              onChange={(e) => setNewEnvValue(e.target.value)}
              className="font-mono text-sm"
            />
          </div>
          <Button
            size="sm"
            onClick={addEnvVar}
            disabled={!newEnvKey.trim()}
            className="cursor-pointer"
          >
            <Plus className="size-4" />
            Add Variable
          </Button>
        </div>

        <Separator />

        {/* Variable list */}
        {envVars.length === 0 ? (
          <div className="flex flex-col items-center py-8 text-center">
            <div className="rounded-full bg-muted p-3 mb-3">
              <Pencil className="size-5 text-muted-foreground" />
            </div>
            <p className="text-sm text-muted-foreground">
              No environment variables configured.
            </p>
          </div>
        ) : (
          <div className="rounded-lg border overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/50">
                  <TableHead className="font-semibold">Key</TableHead>
                  <TableHead className="font-semibold">Value</TableHead>
                  <TableHead className="w-28 text-right font-semibold">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {envVars.map((v) => (
                  <TableRow key={v.key} className="hover:bg-muted/30 transition-colors">
                    <TableCell className="font-mono text-sm font-bold">
                      {v.key}
                    </TableCell>
                    <TableCell className="font-mono text-sm text-muted-foreground">
                      {v.isSecret && !showSecrets ? (
                        <span className="select-none tracking-wider">
                          {'\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022'}
                        </span>
                      ) : (
                        v.value
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Tooltip content="Copy value">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-7 cursor-pointer"
                            onClick={() => navigator.clipboard.writeText(v.value)}
                          >
                            <Copy className="size-3" />
                          </Button>
                        </Tooltip>
                        <Tooltip content="Edit">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-7 cursor-pointer"
                            onClick={() => setEditingEnv(v.key)}
                          >
                            <Pencil className="size-3" />
                          </Button>
                        </Tooltip>
                        <Tooltip content="Delete">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-7 text-destructive hover:text-destructive cursor-pointer"
                            onClick={() => removeEnvVar(v.key)}
                          >
                            <Trash2 className="size-3" />
                          </Button>
                        </Tooltip>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}