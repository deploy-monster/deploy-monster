import { Moon, Sun, Monitor } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { useThemeStore } from '@/stores/theme';

export function AppearanceSection() {
  const { theme, setTheme } = useThemeStore();

  const themes = [
    { id: 'light' as const, label: 'Light', icon: Sun },
    { id: 'dark' as const, label: 'Dark', icon: Moon },
    { id: 'system' as const, label: 'System', icon: Monitor },
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Appearance</CardTitle>
        <CardDescription>Customize the look and feel of your dashboard.</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex gap-2">
          {themes.map(({ id, label, icon: Icon }) => (
            <Button
              key={id}
              variant={theme === id ? 'default' : 'outline'}
              size="sm"
              onClick={() => setTheme(id)}
              className="cursor-pointer"
            >
              <Icon className="size-3.5" />
              {label}
            </Button>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}