import { Link } from 'react-router';
import { ArrowRight } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { QUICK_ACTIONS } from './constants';

export function QuickActions() {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
      {QUICK_ACTIONS.map((action) => (
        <Link key={action.href} to={action.href} className="group">
          <Card className="py-5 h-full transition-all duration-200 hover:translate-y-[-2px] hover:shadow-lg hover:border-primary/20">
            <CardContent className="flex items-start gap-4">
              <div className={cn('flex items-center justify-center rounded-xl size-11 shrink-0', action.bgColor)}>
                <action.icon className={cn('size-5', action.color)} />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <h2 className="font-semibold text-sm text-foreground group-hover:text-primary transition-colors">
                    {action.title}
                  </h2>
                  <ArrowRight className="size-3.5 text-muted-foreground opacity-0 -translate-x-1 group-hover:opacity-100 group-hover:translate-x-0 transition-all duration-200" />
                </div>
                <p className="text-xs text-muted-foreground mt-1 leading-relaxed">
                  {action.description}
                </p>
              </div>
            </CardContent>
          </Card>
        </Link>
      ))}
    </div>
  );
}
