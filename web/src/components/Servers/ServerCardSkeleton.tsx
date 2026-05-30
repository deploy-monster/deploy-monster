import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';

export function ServerCardSkeleton() {
  return (
    <Card className="py-5">
      <CardHeader className="pb-0">
        <div className="flex items-start gap-3">
          <Skeleton className="size-11 rounded-xl shrink-0" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-5 w-24" />
            <Skeleton className="h-3.5 w-32" />
          </div>
          <Skeleton className="h-5 w-16 rounded-md" />
        </div>
      </CardHeader>
      <CardContent className="pt-0 mt-3">
        <Skeleton className="h-4 w-48" />
      </CardContent>
    </Card>
  );
}