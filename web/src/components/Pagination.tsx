import { ChevronLeft, ChevronRight } from 'lucide-react';

interface PaginationProps {
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
}

export function Pagination({ page, totalPages, onPageChange }: PaginationProps) {
  if (totalPages <= 1) return null;

  const pages: (number | '...')[] = [];
  for (let i = 1; i <= totalPages; i++) {
    if (i === 1 || i === totalPages || (i >= page - 1 && i <= page + 1)) {
      pages.push(i);
    } else if (pages[pages.length - 1] !== '...') {
      pages.push('...');
    }
  }

  return (
    <div className="flex items-center gap-1 justify-center mt-6">
      <button onClick={() => onPageChange(page - 1)} disabled={page <= 1}
        className="p-2 rounded-lg border border-border text-text-secondary hover:bg-surface-secondary disabled:opacity-30 disabled:cursor-not-allowed">
        <ChevronLeft size={16} />
      </button>

      {pages.map((p, i) =>
        p === '...' ? (
          <span key={`dots-${i}`} className="px-2 text-text-muted">...</span>
        ) : (
          <button key={p} onClick={() => onPageChange(p)}
            className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
              p === page
                ? 'bg-monster-green text-white'
                : 'border border-border text-text-secondary hover:bg-surface-secondary'
            }`}>
            {p}
          </button>
        )
      )}

      <button onClick={() => onPageChange(page + 1)} disabled={page >= totalPages}
        className="p-2 rounded-lg border border-border text-text-secondary hover:bg-surface-secondary disabled:opacity-30 disabled:cursor-not-allowed">
        <ChevronRight size={16} />
      </button>
    </div>
  );
}
