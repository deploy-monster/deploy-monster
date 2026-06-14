import { useState } from 'react';
import { Copy, AlertCircle, CheckCircle2 } from 'lucide-react';
import type { DeployTemplateResponse, Template } from '@/api/marketplace';
import { cn } from '@/lib/utils';
import { formatGeneratedSecrets, generatedSecretEntries } from '@/lib/generatedSecrets';
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
  title?: string;
  type: string;
  description?: string;
  default?: string;
  required?: boolean;
  secret?: boolean;
  options?: string[];
}

interface TemplateDeploySheetProps {
  template: Template | null;
  open: boolean;
  onClose: () => void;
  onDeploy: (t: Template, variables: Record<string, string>, appName: string) => Promise<DeployTemplateResponse | void>;
  onOpenApp?: (appId: string) => void;
}

export function TemplateDeploySheet({ template, open, onClose, onDeploy, onOpenApp }: TemplateDeploySheetProps) {
  const [appName, setAppName] = useState('');
  const [variables, setVariables] = useState<Record<string, string>>({});
  const [deploying, setDeploying] = useState(false);
  const [deployError, setDeployError] = useState('');
  const [generatedSecrets, setGeneratedSecrets] = useState<Record<string, string>>({});
  const [deployedAppId, setDeployedAppId] = useState('');
  const [credentialsCopied, setCredentialsCopied] = useState(false);

  const handleOpenChange = (o: boolean) => {
    if (!o) {
      onClose();
      setVariables({});
      setAppName('');
      setDeployError('');
      setGeneratedSecrets({});
      setDeployedAppId('');
      setCredentialsCopied(false);
    }
  };

  const handleDeploy = async () => {
    if (!template) return;
    const resolvedAppName = appName.trim() || template.slug;
    setDeploying(true);
    setDeployError('');
    try {
      const result = await onDeploy(template, variables, resolvedAppName);
      const secrets = result?.generated_secrets || {};
      if (Object.keys(secrets).length > 0) {
        setGeneratedSecrets(secrets);
        setDeployedAppId(result?.app_id || '');
        setCredentialsCopied(false);
      } else {
        handleOpenChange(false);
      }
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
      label: field.title || name,
      required: template?.config_schema?.required?.includes(name) ?? false,
    })
  );
  const hasGeneratedSecrets = Object.keys(generatedSecrets).length > 0;

  const handleCopyGeneratedSecrets = async () => {
    try {
      await navigator.clipboard.writeText(formatGeneratedSecrets(generatedSecrets));
      setCredentialsCopied(true);
      toast.success('Credentials copied');
    } catch {
      setCredentialsCopied(true);
      toast.error('Failed to copy credentials');
    }
  };

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent onClose={() => handleOpenChange(false)} className="flex flex-col">
        <SheetHeader>
          <div className="flex items-center gap-3">
            {template && <TemplateIcon template={template} size="size-8" />}
            <div>
              <SheetTitle>{template ? `Deploy ${template.name}` : 'Deploy Template'}</SheetTitle>
              <SheetDescription>
                Configure and deploy this template
              </SheetDescription>
            </div>
          </div>
        </SheetHeader>

        <SheetBody className="flex-1 overflow-y-auto">
          {hasGeneratedSecrets ? (
            <div className="space-y-4">
              <div className="space-y-3 rounded-lg border bg-muted/30 p-4">
                <Label className="text-sm font-medium">Generated Credentials</Label>
                <p className="text-xs text-muted-foreground">
                  These generated values are shown once. Copy or save them before opening the app.
                </p>
                <div className="space-y-2">
                  {generatedSecretEntries(generatedSecrets).map(([key, value]) => (
                    <div key={key} className="grid gap-1 rounded-md border bg-background px-3 py-2">
                      <span className="text-xs font-medium text-muted-foreground">{key}</span>
                      <code className="break-all text-sm">{value}</code>
                    </div>
                  ))}
                </div>
              </div>
              <div className="grid gap-2 sm:grid-cols-2">
                <Button variant="outline" onClick={handleCopyGeneratedSecrets}>
                  <Copy className="size-4" />
                  Copy Credentials
                </Button>
                <Button
                  onClick={() => deployedAppId && onOpenApp?.(deployedAppId)}
                  disabled={!credentialsCopied || !deployedAppId}
                >
                  Open App
                </Button>
              </div>
            </div>
          ) : (
            <>
          {template?.verified && (
            <div className="flex items-center gap-2 rounded-lg border border-blue-500/20 bg-blue-500/5 px-3 py-2.5 text-sm text-blue-700 dark:text-blue-400 mb-4">
              <CheckCircle2 className="size-4 shrink-0" />
              Verified template — reviewed for security and compatibility
            </div>
          )}

          {/* App name */}
          <div className="space-y-1.5 mb-6">
            <Label htmlFor="deploy-app-name">Stack Name *</Label>
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
            </>
          )}
        </SheetBody>

        {!hasGeneratedSecrets && (
          <div className="flex items-center gap-2 pt-4 border-t mt-4">
          <Button variant="outline" onClick={() => handleOpenChange(false)} disabled={deploying} className="flex-1 cursor-pointer">
            Cancel
          </Button>
          <Button
            onClick={handleDeploy}
            disabled={!template || deploying}
            className="flex-1 cursor-pointer"
          >
            {deploying ? 'Deploying...' : 'Deploy Now'}
          </Button>
          </div>
        )}
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
  const isSecret =
    field.secret ||
    field.type === 'password' ||
    field.name.toLowerCase().includes('secret') ||
    field.name.toLowerCase().includes('password');
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
