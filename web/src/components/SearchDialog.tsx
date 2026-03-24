import { useState, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router';
import { Search, X, Rocket, Globe, FolderOpen } from 'lucide-react';
import { api } from '../api/client';

interface SearchResult {
  type: string;
  id: string;
  name: string;
  info: string;
}

const typeIcons: Record<string, React.ElementType> = {
  app: Rocket,
  domain: Globe,
  project: FolderOpen,
};

const typeRoutes: Record<string, (id: string) => string> = {
  app: (id) => `/apps/${id}`,
  domain: () => '/domains',
  project: () => `/projects`,
};

export function SearchDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate();
  const inputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (open) {
      setQuery('');
      setResults([]);
      setTimeout(() => inputRef.current?.focus(), 100);
    }
  }, [open]);

  useEffect(() => {
    if (query.length < 2) {
      setResults([]);
      return;
    }

    const timer = setTimeout(async () => {
      setLoading(true);
      try {
        const r = await api.get<{ results: SearchResult[] }>(`/search?q=${encodeURIComponent(query)}`);
        setResults(r.results || []);
      } catch {
        setResults([]);
      } finally {
        setLoading(false);
      }
    }, 300); // Debounce

    return () => clearTimeout(timer);
  }, [query]);

  const handleSelect = (result: SearchResult) => {
    const route = typeRoutes[result.type];
    if (route) {
      navigate(route(result.id));
    }
    onClose();
  };

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-20 bg-black/50" onClick={onClose}>
      <div className="bg-surface border border-border rounded-2xl w-full max-w-lg shadow-2xl overflow-hidden" onClick={(e) => e.stopPropagation()}>
        {/* Search input */}
        <div className="flex items-center gap-3 px-4 py-3 border-b border-border">
          <Search size={18} className="text-text-muted" />
          <input ref={inputRef} type="text" value={query} onChange={(e) => setQuery(e.target.value)}
            className="flex-1 bg-transparent text-text-primary outline-none text-sm"
            placeholder="Search apps, domains, projects..." />
          {query && <button onClick={() => setQuery('')} className="text-text-muted"><X size={16} /></button>}
          <kbd className="hidden sm:inline text-xs text-text-muted bg-surface-tertiary px-1.5 py-0.5 rounded">ESC</kbd>
        </div>

        {/* Results */}
        <div className="max-h-80 overflow-auto">
          {loading && (
            <div className="px-4 py-8 text-center text-text-muted text-sm">Searching...</div>
          )}

          {!loading && query.length >= 2 && results.length === 0 && (
            <div className="px-4 py-8 text-center text-text-muted text-sm">No results for "{query}"</div>
          )}

          {results.map((result) => {
            const Icon = typeIcons[result.type] || Rocket;
            return (
              <button key={`${result.type}-${result.id}`} onClick={() => handleSelect(result)}
                className="flex items-center gap-3 px-4 py-3 w-full text-left hover:bg-surface-secondary transition-colors">
                <Icon size={16} className="text-text-muted shrink-0" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-text-primary truncate">{result.name}</p>
                  <p className="text-xs text-text-muted">{result.type} {result.info && `• ${result.info}`}</p>
                </div>
              </button>
            );
          })}
        </div>

        {query.length < 2 && (
          <div className="px-4 py-6 text-center text-text-muted text-sm">
            Type at least 2 characters to search
          </div>
        )}
      </div>
    </div>
  );
}
