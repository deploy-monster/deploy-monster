import { Container, Database, Globe, HardDrive, Cog } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { TopologyNodeType } from '@/types/topology';

interface PaletteItem {
  type: TopologyNodeType;
  label: string;
  icon: React.ReactNode;
  color: string;
  description: string;
}

const paletteItems: PaletteItem[] = [
  {
    type: 'app',
    label: 'App',
    icon: <Container className="h-5 w-5" />,
    color: 'blue',
    description: 'Container from Git',
  },
  {
    type: 'database',
    label: 'Database',
    icon: <Database className="h-5 w-5" />,
    color: 'green',
    description: 'Managed DB',
  },
  {
    type: 'domain',
    label: 'Domain',
    icon: <Globe className="h-5 w-5" />,
    color: 'purple',
    description: 'Custom domain',
  },
  {
    type: 'volume',
    label: 'Volume',
    icon: <HardDrive className="h-5 w-5" />,
    color: 'orange',
    description: 'Persistent storage',
  },
  {
    type: 'worker',
    label: 'Worker',
    icon: <Cog className="h-5 w-5" />,
    color: 'yellow',
    description: 'Background worker',
  },
];

const colorClasses: Record<string, string> = {
  blue: 'border-blue-500/50 hover:border-blue-500 hover:bg-blue-500/10',
  green: 'border-green-500/50 hover:border-green-500 hover:bg-green-500/10',
  purple: 'border-purple-500/50 hover:border-purple-500 hover:bg-purple-500/10',
  orange: 'border-orange-500/50 hover:border-orange-500 hover:bg-orange-500/10',
  yellow: 'border-yellow-500/50 hover:border-yellow-500 hover:bg-yellow-500/10',
};

const iconColorClasses: Record<string, string> = {
  blue: 'text-blue-500',
  green: 'text-green-500',
  purple: 'text-purple-500',
  orange: 'text-orange-500',
  yellow: 'text-yellow-500',
};

interface ComponentPaletteProps {
  onDragStart: (type: TopologyNodeType) => void;
}

export function ComponentPalette({ onDragStart }: ComponentPaletteProps) {
  return (
    <div className="flex h-full w-48 flex-col border-r bg-card">
      <div className="border-b p-3">
        <h3 className="text-sm font-semibold text-foreground">Components</h3>
        <p className="text-xs text-muted-foreground">Drag to canvas</p>
      </div>
      <div className="flex-1 space-y-2 overflow-y-auto p-3">
        {paletteItems.map((item) => (
          <div
            key={item.type}
            draggable
            onDragStart={(e) => {
              e.dataTransfer.setData('application/topology-node', item.type);
              e.dataTransfer.effectAllowed = 'move';
              onDragStart(item.type);
            }}
            className={cn(
              'flex cursor-grab items-center gap-3 rounded-lg border-2 p-3 transition-all active:cursor-grabbing',
              colorClasses[item.color]
            )}
          >
            <div className={cn('flex-shrink-0', iconColorClasses[item.color])}>
              {item.icon}
            </div>
            <div className="flex-1 min-w-0">
              <div className="text-sm font-medium text-foreground">{item.label}</div>
              <div className="text-xs text-muted-foreground truncate">{item.description}</div>
            </div>
          </div>
        ))}
      </div>
      <div className="border-t p-3">
        <p className="text-xs text-muted-foreground">
          Drag components to the canvas to build your infrastructure
        </p>
      </div>
    </div>
  );
}
