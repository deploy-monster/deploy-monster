import { Rocket, ShieldCheck, Cpu } from 'lucide-react';
import type { Template } from '@/api/marketplace';
import { cn } from '@/lib/utils';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { getCategoryColor } from './constants';

export function TemplateCardSkeleton() {
  return (
    <Card className="py-5">
      <CardHeader className="pb-0">
        <div className="flex items-start gap-3">
          <div className="size-10 rounded-xl shrink-0 bg-muted animate-pulse" />
          <div className="flex-1 space-y-2">
            <div className="h-4 w-28 bg-muted animate-pulse rounded" />
            <div className="h-3 w-16 bg-muted animate-pulse rounded" />
          </div>
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        <div className="space-y-2 mt-3">
          <div className="h-3 w-full bg-muted animate-pulse rounded" />
          <div className="h-3 w-3/4 bg-muted animate-pulse rounded" />
        </div>
        <div className="flex gap-1.5 mt-3">
          <div className="h-5 w-14 rounded-md bg-muted animate-pulse" />
          <div className="h-5 w-14 rounded-md bg-muted animate-pulse" />
          <div className="h-5 w-14 rounded-md bg-muted animate-pulse" />
        </div>
      </CardContent>
      <CardFooter className="border-t pt-4 pb-0 justify-between">
        <div className="h-3 w-10 bg-muted animate-pulse rounded" />
        <div className="h-8 w-20 rounded-md bg-muted animate-pulse" />
      </CardFooter>
    </Card>
  );
}

export function TemplateIcon({ template, size = 'size-10' }: { template: Template; size?: string }) {
  const catColor = getCategoryColor(template.category);
  if (template.icon && template.icon.length <= 4) {
    return (
      <div className={cn('flex items-center justify-center rounded-xl shrink-0 bg-card border', size)}>
        <span className="text-xl">{template.icon}</span>
      </div>
    );
  }
  return (
    <div className={cn('flex items-center justify-center rounded-xl shrink-0', catColor.iconBg, size)}>
      <span className="text-base font-bold text-white">
        {template.name.charAt(0).toUpperCase()}
      </span>
    </div>
  );
}

interface TemplateCardProps {
  template: Template;
  onDeploy: (t: Template) => void;
  onClick?: (t: Template) => void;
}

export function TemplateCard({ template, onDeploy, onClick }: TemplateCardProps) {
  const catColor = getCategoryColor(template.category);
  return (
    <Card
      className="group py-5 transition-all duration-200 hover:translate-y-[-2px] hover:shadow-lg cursor-pointer"
      onClick={() => onClick?.(template)}
    >
      <CardHeader className="pb-0">
        <div className="flex items-start gap-3">
          <TemplateIcon template={template} />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-1.5">
              <CardTitle className="text-sm truncate">{template.name}</CardTitle>
              {template.verified && <ShieldCheck className="size-3.5 text-blue-500 shrink-0" />}
            </div>
            <span className={cn('text-[10px] font-medium', catColor.text)}>
              {template.category}
            </span>
          </div>
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        <p className="text-xs text-muted-foreground line-clamp-2">{template.description}</p>
        {template.tags && template.tags.length > 0 && (
          <div className="flex gap-1.5 mt-3 flex-wrap">
            {template.tags.slice(0, 3).map((tag) => (
              <Badge key={tag} variant="secondary" className="text-[10px] font-normal px-1.5 py-0.5">
                {tag}
              </Badge>
            ))}
          </div>
        )}
        {template.min_resources?.memory_mb && (
          <div className="flex items-center gap-1.5 mt-2 text-[10px] text-muted-foreground">
            <Cpu className="size-3" />
            {template.min_resources.memory_mb} MB RAM
            {template.min_resources.disk_mb && ` \u00b7 ${template.min_resources.disk_mb} MB disk`}
          </div>
        )}
      </CardContent>
      <CardFooter className="border-t pt-4 pb-0 justify-between">
        <span className="text-[10px] text-muted-foreground">
          {template.version && `v${template.version}`}
        </span>
        <Button
          size="sm"
          className="h-7 text-xs gap-1.5 cursor-pointer"
          onClick={(e) => { e.stopPropagation(); onDeploy(template); }}
        >
          <Rocket className="size-3" />
          Deploy
        </Button>
      </CardFooter>
    </Card>
  );
}

interface FeaturedTemplateCardProps {
  template: Template;
  onDeploy: (t: Template) => void;
  onClick: (t: Template) => void;
}

export function FeaturedTemplateCard({ template, onDeploy, onClick }: FeaturedTemplateCardProps) {
  const catColor = getCategoryColor(template.category);
  return (
    <Card
      key={template.slug}
      className="group min-w-[280px] max-w-[280px] shrink-0 transition-all duration-200 hover:translate-y-[-2px] hover:shadow-lg hover:ring-2 hover:ring-primary/30 cursor-pointer"
      onClick={() => onClick(template)}
    >
      <CardHeader className="pb-2">
        <div className="flex items-start gap-3 min-w-0">
          <TemplateIcon template={template} />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-1.5">
              <CardTitle className="text-sm truncate">{template.name}</CardTitle>
              {template.verified && <ShieldCheck className="size-3.5 text-blue-500 shrink-0" />}
            </div>
            <span className={cn('text-[10px] font-medium', catColor.text)}>
              {template.category}
            </span>
          </div>
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        <p className="text-xs text-muted-foreground line-clamp-2">{template.description}</p>
        {template.min_resources?.memory_mb && (
          <div className="flex items-center gap-1.5 mt-2 text-[10px] text-muted-foreground">
            <Cpu className="size-3" />
            {template.min_resources.memory_mb} MB RAM
            {template.min_resources.disk_mb && ` \u00b7 ${template.min_resources.disk_mb} MB disk`}
          </div>
        )}
      </CardContent>
      <CardFooter className="border-t pt-3 pb-0">
        <Button
          size="sm"
          className="h-7 text-xs w-full gap-1.5 cursor-pointer"
          onClick={(e) => { e.stopPropagation(); onDeploy(template); }}
        >
          <Rocket className="size-3" />
          Deploy
        </Button>
      </CardFooter>
    </Card>
  );
}