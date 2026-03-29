import { useState, useEffect } from 'react';
import { Copy, Check, FileCode, Globe, FileText, Download } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

interface CompileModalProps {
  open: boolean;
  onClose: () => void;
  result: CompileResult | null;
  isCompiling: boolean;
}

interface CompileResult {
  success: boolean;
  message: string;
  composeYaml?: string;
  caddyfile?: string;
  envFile?: string;
  errors?: string[];
}

export function CompileModal({ open, onClose, result, isCompiling }: CompileModalProps) {
  const [copied, setCopied] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState('compose');

  useEffect(() => {
    if (!open) {
      const id = requestAnimationFrame(() => {
        setCopied(null);
        setActiveTab('compose');
      });
      return () => cancelAnimationFrame(id);
    }
  }, [open]);

  const copyToClipboard = async (text: string, key: string) => {
    await navigator.clipboard.writeText(text);
    setCopied(key);
    setTimeout(() => setCopied(null), 2000);
  };

  const downloadFile = (content: string, filename: string) => {
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  };

  const hasContent = (content?: string) => content && content.length > 0;

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="max-w-4xl max-h-[80vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FileCode className="h-5 w-5" />
            Generated Configuration
          </DialogTitle>
        </DialogHeader>

        {isCompiling ? (
          <div className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-3 text-muted-foreground">Compiling topology...</span>
          </div>
        ) : result ? (
          <>
            {result.success ? (
              <div className="flex-1 overflow-hidden flex flex-col">
                <div className="mb-4 p-3 rounded-lg bg-green-500/10 border border-green-500/20 text-green-600 text-sm">
                  {result.message}
                </div>

                <Tabs value={activeTab} onValueChange={setActiveTab} className="flex-1 flex flex-col overflow-hidden">
                  <TabsList className="grid w-full grid-cols-3">
                    <TabsTrigger value="compose" className="flex items-center gap-2">
                      <FileCode className="h-4 w-4" />
                      docker-compose.yaml
                    </TabsTrigger>
                    <TabsTrigger value="caddy" className="flex items-center gap-2" disabled={!hasContent(result.caddyfile)}>
                      <Globe className="h-4 w-4" />
                      Caddyfile
                    </TabsTrigger>
                    <TabsTrigger value="env" className="flex items-center gap-2" disabled={!hasContent(result.envFile)}>
                      <FileText className="h-4 w-4" />
                      .env
                    </TabsTrigger>
                  </TabsList>

                  <div className="flex-1 overflow-hidden mt-4">
                    <TabsContent value="compose" className="h-full mt-0">
                      <FileViewer
                        content={result.composeYaml || ''}
                        filename="docker-compose.yaml"
                        onCopy={() => copyToClipboard(result.composeYaml || '', 'compose')}
                        onDownload={() => downloadFile(result.composeYaml || '', 'docker-compose.yaml')}
                        copied={copied === 'compose'}
                      />
                    </TabsContent>

                    <TabsContent value="caddy" className="h-full mt-0">
                      <FileViewer
                        content={result.caddyfile || ''}
                        filename="Caddyfile"
                        onCopy={() => copyToClipboard(result.caddyfile || '', 'caddy')}
                        onDownload={() => downloadFile(result.caddyfile || '', 'Caddyfile')}
                        copied={copied === 'caddy'}
                      />
                    </TabsContent>

                    <TabsContent value="env" className="h-full mt-0">
                      <FileViewer
                        content={result.envFile || ''}
                        filename=".env"
                        onCopy={() => copyToClipboard(result.envFile || '', 'env')}
                        onDownload={() => downloadFile(result.envFile || '', '.env')}
                        copied={copied === 'env'}
                      />
                    </TabsContent>
                  </div>
                </Tabs>
              </div>
            ) : (
              <div className="p-4 rounded-lg bg-red-500/10 border border-red-500/20">
                <p className="text-red-600 font-medium">Compilation failed</p>
                <p className="text-red-500 text-sm mt-1">{result.message}</p>
                {result.errors && result.errors.length > 0 && (
                  <ul className="mt-3 space-y-1">
                    {result.errors.map((error, i) => (
                      <li key={i} className="text-sm text-red-500 flex items-start gap-2">
                        <span className="text-red-400">•</span>
                        {error}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </>
        ) : (
          <div className="flex items-center justify-center py-12 text-muted-foreground">
            No compilation result
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

interface FileViewerProps {
  content: string;
  filename: string;
  onCopy: () => void;
  onDownload: () => void;
  copied: boolean;
}

function FileViewer({ content, filename, onCopy, onDownload, copied }: FileViewerProps) {
  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between mb-2">
        <span className="text-sm font-mono text-muted-foreground">{filename}</span>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={onCopy}>
            {copied ? (
              <Check className="h-4 w-4 text-green-500" />
            ) : (
              <Copy className="h-4 w-4" />
            )}
            <span className="ml-1">{copied ? 'Copied!' : 'Copy'}</span>
          </Button>
          <Button variant="ghost" size="sm" onClick={onDownload}>
            <Download className="h-4 w-4" />
            <span className="ml-1">Download</span>
          </Button>
        </div>
      </div>
      <div className="flex-1 overflow-auto rounded-lg bg-muted/50 border">
        <pre className="p-4 text-xs font-mono whitespace-pre-wrap break-all">
          {content || <span className="text-muted-foreground italic">No content generated</span>}
        </pre>
      </div>
    </div>
  );
}
