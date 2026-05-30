import { useState } from 'react';
import { Copy, AlertCircle, CheckCircle2 } from 'lucide-react';
import type { Template } from '@/api/marketplace';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import {
  Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription, SheetBody,
} from '@/components/ui/sheet';
import { PasswordInput } from './PasswordInput';
import { TemplateIcon } from './TemplateCard';
import { toast } from '@/stores/toastStore';

interface ConfigVariable {
  name: string;
  label?: string;
  type: string;
  description?: string;
  default?: string;
  required?: boolean;
  options?: string[];
}

interface TemplateDeploySheetProps {
  template: Template | null;
  open: boolean;
  onClose: () => void;
  onDeploy: (t: Template, variables: Record<string, string>, appName: string) => Promise<void>;
}

export function TemplateDeploySheet({ template, open, onClose, onDeploy }: TemplateDeploySheetProps) {
  const [appName, setAppName] = useState('');
  const [variables, setVariables] = useState<Record<string, string>>({});
  const [deploying, setDeploying] = useState(false);
  const [deployError, setDeployError] = useState('');

  const handleOpenChange = (o: boolean) => {
    if (!o) {
      onClose();
      setVariables({});
      setAppName('');
      setDeployError('');
    }
  };

  const handleDeploy = async () => {
    if (!template || !appName.trim()) return;
    setDeploying(true);
    setDeployError('');
    try {
      await onDeploy(template, variables, appName);
      handleOpenChange(false);
    } catch (err) {
      setDeployError(err instanceof Error ? err.message : 'Deployment failed');
    } finally {
      setDeploying(false);
    }
  };

  const fields: ConfigVariable[] = Object.entries(template?.config_schema?.properties || {}).map(
    ([name, field]) => ({
      name,
      ...field,
      required: template?.config_schema?.required?.includes(name) ?? false,
    })
  );

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent onClose={() => handleOpenChange(false)} className="flex flex-col">
        <SheetHeader>
          <div className="flex items-center gap-3">
            {template && <TemplateIcon template={template} size="size-8" />}
            <div>
              <SheetTitle>{template?.name}</SheetTitle>
              <SheetDescription>
                Configure and deploy this template
              </SheetDescription>
            </div>
          </div>
        </SheetHeader>

        <SheetBody className="flex-1 overflow-y-auto">
          {template?.verified && (
            <div className="flex items-center gap-2 rounded-lg border border-blue-500/20 bg-blue-500/5 px-3 py-2.5 text-sm text-blue-700 dark:text-blue-400 mb-4">
              <CheckCircle2 className="size-4 shrink-0" />
              Verified template — reviewed for security and compatibility
            </div>
          )}

          {/* App name */}
          <div className="space-y-1.5 mb-6">
            <Label htmlFor="deploy-app-name">Application Name *</Label>
            <Input
              id="deploy-app-name"
              value={appName}
              onChange={(e) => setAppName(e.target.value)}
              placeholder="my-instance"
            />
          </div>

          {/* Variables */}
          {fields.length > 0 && (
            <div className="space-y-4">
              <div className="text-sm font-medium">Configuration</div>
              {fields.map((field) => (
                <VariableField
                  key={field.name}
                  field={field}
                  value={variables[field.name] ?? field.default ?? ''}
                  onChange={(val) => setVariables((prev) => ({ ...prev, [field.name]: val }))}
                />
              ))}
            </div>
          )}

          {deployError && (
            <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2.5 text-sm text-destructive mt-4">
              <AlertCircle className="size-4 shrink-0" />
              {deployError}
            </div>
          )}
        </SheetBody>

        <div className="flex items-center gap-2 pt-4 border-t mt-4">
          <Button variant="outline" onClick={() => handleOpenChange(false)} disabled={deploying} className="flex-1 cursor-pointer">
            Cancel
          </Button>
          <Button
            onClick={handleDeploy}
            disabled={!appName.trim() || deploying}
            className="flex-1 cursor-pointer"
          >
            {deploying ? 'Deploying...' : 'Deploy Now'}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}

/* ------------------------------------------------------------------ */
/*  Variable field                                                      */
/* ------------------------------------------------------------------ */

interface VariableFieldProps {
  field: ConfigVariable;
  value: string;
  onChange: (val: string) => void;
}

function VariableField({ field, value, onChange }: VariableFieldProps) {
  const isSecret = field.type === 'password' || field.name.toLowerCase().includes('secret') || field.name.toLowerCase().includes('password');
  const isRequired = field.required ?? false;

  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Label htmlFor={`var-${field.name}`} className="text-xs">
          {field.label || field.name}
        </Label>
        {isRequired && <span className="text-destructive text-xs">*</span>}
        {isSecret && <Badge variant="secondary" className="text-[9px] font-normal px-1">secret</Badge>}
      </div>

      {field.type === 'password' || isSecret ? (
        <PasswordInput
          id={`var-${field.name}`}
          value={value}
          onChange={onChange}
          placeholder={field.default || field.description || ''}
        />
      ) : field.type === 'select' && field.options ? (
        <select
          id={`var-${field.name}`}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm"
        >
          <option value="">Select...</option>
          {field.options.map((opt) => (
            <option key={opt} value={opt}>{opt}</option>
          ))}
        </select>
      ) : field.type === 'boolean' ? (
        <div className="flex items-center gap-2 h-9">
          <button
            type="button"
            onClick={() => onChange(value === 'true' ? 'false' : 'true')}
            className={cn(
              'relative inline-flex h-5 w-9 items-center rounded-full transition-colors cursor-pointer',
              value === 'true' ? 'bg-primary' : 'bg-input'
            )}
          >
            <span
              className={cn(
                'inline-block size-4 rounded-full bg-white shadow-sm transition-transform',
                value === 'true' ? 'translate-x-[18px]' : 'translate-x-0.5'
              )}
            />
          </button>
          <span className="text-sm text-muted-foreground">{value === 'true' ? 'Enabled' : 'Disabled'}</span>
        </div>
      ) : (
        <div className="relative">
          {field.name.toLowerCase().includes('url') ? (
            <Input
              id={`var-${field.name}`}
              type="url"
              value={value}
              onChange={(e) => onChange(e.target.value)}
              placeholder={field.default || field.description || ''}
            />
          ) : field.name.toLowerCase().includes('email') ? (
            <Input
              id={`var-${field.name}`}
              type="email"
              value={value}
              onChange={(e) => onChange(e.target.value)}
              placeholder={field.default || field.description || ''}
            />
          ) : (
            <Input
              id={`var-${field.name}`}
              value={value}
              onChange={(e) => onChange(e.target.value)}
              placeholder={field.default || field.description || ''}
            />
          )}
        </div>
      )}

      {field.description && (
        <p className="text-[11px] text-muted-foreground mt-0.5">{field.description}</p>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Generated secrets copy button                                      */
/* ------------------------------------------------------------------ */

interface GeneratedSecretsProps {
  entries: { key: string; value: string }[];
}

export function GeneratedSecrets({ entries }: GeneratedSecretsProps) {
  if (entries.length === 0) return null;

  return (
    <div className="rounded-lg border bg-muted/30 p-3 space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">Generated secrets — copy to use in your app</span>
        <Button
          variant="ghost"
          size="sm"
          className="h-6 text-xs cursor-pointer"
          onClick={() => {
            const text = entries.map((e) => `${e.key}=${e.value}`).join('\n');
            navigator.clipboard.writeText(text).then(() => toast.success('Copied!'));
          }}
        >
          <Copy className="size-3" />
          Copy all
        </Button>
      </div>
      {entries.map((e) => (
        <div key={e.key} className="flex items-center gap-2 text-xs">
          <code className="font-mono text-[10px] text-muted-foreground bg-muted px-1.5 py-0.5 rounded">{e.key}</code>
          <code className="font-mono text-[10px] text-foreground bg-muted px-1.5 py-0.5 rounded flex-1 truncate">{e.value}</code>
          <button
            type="button"
            onClick={() => navigator.clipboard.writeText(e.value).then(() => toast.success('Copied'))}
            className="text-muted-foreground hover:text-foreground cursor-pointer"
          >
            <Copy className="size-3" />
          </button>
        </div>
      ))}
    </div>
  );
}