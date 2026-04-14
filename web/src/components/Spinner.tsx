export function Spinner({ size = 'md' }: { size?: 'sm' | 'md' | 'lg' }) {
  const sizes = { sm: 'w-4 h-4 border', md: 'w-8 h-8 border-2', lg: 'w-12 h-12 border-3' };
  return (
    <div
      role="status"
      aria-label="Loading"
      className={`${sizes[size]} border-monster-green border-t-transparent rounded-full animate-spin`}
    />
  );
}

export function FullPageLoader() {
  return (
    <div data-testid="full-page-loader" className="min-h-screen flex items-center justify-center bg-surface">
      <Spinner size="lg" />
    </div>
  );
}
