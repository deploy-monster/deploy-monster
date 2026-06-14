const CATEGORY_COLORS: Record<string, { bg: string; text: string; iconBg: string }> = {
  ai:            { bg: 'bg-violet-500/10',  text: 'text-violet-600 dark:text-violet-400',  iconBg: 'bg-violet-500' },
  cms:           { bg: 'bg-blue-500/10',    text: 'text-blue-600 dark:text-blue-400',      iconBg: 'bg-blue-500' },
  monitoring:    { bg: 'bg-emerald-500/10',  text: 'text-emerald-600 dark:text-emerald-400', iconBg: 'bg-emerald-500' },
  devtools:      { bg: 'bg-orange-500/10',  text: 'text-orange-600 dark:text-orange-400',  iconBg: 'bg-orange-500' },
  storage:       { bg: 'bg-cyan-500/10',    text: 'text-cyan-600 dark:text-cyan-400',      iconBg: 'bg-cyan-500' },
  analytics:     { bg: 'bg-pink-500/10',    text: 'text-pink-600 dark:text-pink-400',      iconBg: 'bg-pink-500' },
  security:      { bg: 'bg-red-500/10',     text: 'text-red-600 dark:text-red-400',        iconBg: 'bg-red-500' },
  automation:    { bg: 'bg-amber-500/10',   text: 'text-amber-600 dark:text-amber-400',    iconBg: 'bg-amber-500' },
  database:      { bg: 'bg-indigo-500/10',  text: 'text-indigo-600 dark:text-indigo-400',  iconBg: 'bg-indigo-500' },
  finance:       { bg: 'bg-green-500/10',   text: 'text-green-600 dark:text-green-400',    iconBg: 'bg-green-500' },
  collaboration: { bg: 'bg-sky-500/10',     text: 'text-sky-600 dark:text-sky-400',        iconBg: 'bg-sky-500' },
  productivity:  { bg: 'bg-teal-500/10',    text: 'text-teal-600 dark:text-teal-400',      iconBg: 'bg-teal-500' },
  search:        { bg: 'bg-purple-500/10',  text: 'text-purple-600 dark:text-purple-400',  iconBg: 'bg-purple-500' },
  communication: { bg: 'bg-sky-500/10',     text: 'text-sky-600 dark:text-sky-400',        iconBg: 'bg-sky-500' },
  media:         { bg: 'bg-rose-500/10',    text: 'text-rose-600 dark:text-rose-400',      iconBg: 'bg-rose-500' },
  ecommerce:     { bg: 'bg-emerald-500/10', text: 'text-emerald-600 dark:text-emerald-400', iconBg: 'bg-emerald-500' },
  iot:           { bg: 'bg-lime-500/10',    text: 'text-lime-600 dark:text-lime-400',      iconBg: 'bg-lime-500' },
  design:        { bg: 'bg-fuchsia-500/10', text: 'text-fuchsia-600 dark:text-fuchsia-400', iconBg: 'bg-fuchsia-500' },
  networking:    { bg: 'bg-slate-500/10',   text: 'text-slate-600 dark:text-slate-400',    iconBg: 'bg-slate-500' },
};

const DEFAULT_CATEGORY_COLOR = { bg: 'bg-muted', text: 'text-muted-foreground', iconBg: 'bg-muted-foreground' };

export function getCategoryColor(category: string) {
  return CATEGORY_COLORS[category.toLowerCase()] || DEFAULT_CATEGORY_COLOR;
}
