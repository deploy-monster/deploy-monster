import { Bell } from 'lucide-react';

interface Announcement {
  id: string;
  title: string;
  body: string;
  type: string;
}

interface AnnouncementsBannerProps {
  announcements: Announcement[];
}

export function AnnouncementsBanner({ announcements }: AnnouncementsBannerProps) {
  if (announcements.length === 0) return null;

  const latest = announcements[0];
  return (
    <div className="rounded-lg border border-primary/20 bg-primary/5 p-4 flex items-start gap-3">
      <div className="rounded-full bg-primary/10 p-1.5 mt-0.5 shrink-0">
        <Bell className="size-4 text-primary" />
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-foreground">{latest.title}</p>
        {latest.body && (
          <p className="text-sm text-muted-foreground mt-0.5">{latest.body}</p>
        )}
      </div>
    </div>
  );
}