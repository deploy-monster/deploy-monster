import { useState } from 'react';
import { Bell } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { Switch } from '@/components/ui/switch';
import { Separator } from '@/components/ui/separator';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { useMutation } from '@/hooks';
import { toast } from '@/stores/toastStore';
import { ProfileSection } from '@/components/Settings/ProfileSection';
import { SecuritySection, APIKeySection } from '@/components/Settings/SecuritySection';
import { AppearanceSection } from '@/components/Settings/AppearanceSection';

export function Settings() {
  const [notifications, setNotifications] = useState({
    email: true, slack: false, discord: false, deploy: true,
  });

  const { loading: savingNotif, mutate: saveNotifications } = useMutation<
    Record<string, boolean>,
    void
  >('put', '/settings/notifications');

  const handleSaveNotifications = () => {
    saveNotifications(notifications, {
      onSuccess: () => toast.success('Notification preferences saved'),
      onError: (err) => toast.error(`Failed to save: ${err}`),
    });
  };

  return (
    <div className="space-y-8 max-w-3xl">
      <div>
        <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
          Settings
        </h1>
        <p className="text-muted-foreground mt-1.5 text-sm">
          Manage your profile, security, and preferences.
        </p>
      </div>

      <Tabs defaultValue="profile">
        <TabsList>
          <TabsTrigger value="profile">Profile</TabsTrigger>
          <TabsTrigger value="security">Security</TabsTrigger>
          <TabsTrigger value="notifications">Notifications</TabsTrigger>
          <TabsTrigger value="appearance">Appearance</TabsTrigger>
        </TabsList>

        <TabsContent value="profile" className="space-y-6">
          <ProfileSection />
        </TabsContent>

        <TabsContent value="security" className="space-y-6">
          <SecuritySection />
          <APIKeySection />
        </TabsContent>

        <TabsContent value="notifications" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Bell className="size-4 text-primary" />
                Notification Preferences
              </CardTitle>
              <CardDescription>
                Choose how you want to receive notifications about deployments and system events.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {[
                { key: 'email', label: 'Email Notifications', desc: 'Receive notifications via email.' },
                { key: 'slack', label: 'Slack', desc: 'Send notifications to your Slack workspace.' },
                { key: 'discord', label: 'Discord', desc: 'Send notifications to your Discord server.' },
                { key: 'deploy', label: 'Deployment Updates', desc: 'Receive notifications when deployments succeed or fail.' },
              ].map(({ key, label, desc }) => (
                <div key={key} className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-medium">{label}</p>
                    <p className="text-xs text-muted-foreground">{desc}</p>
                  </div>
                  <Switch
                    checked={notifications[key as keyof typeof notifications]}
                    onCheckedChange={(checked) =>
                      setNotifications((prev) => ({ ...prev, [key]: checked }))
                    }
                  />
                </div>
              ))}
              <Separator />
              <div className="flex justify-end">
                <Button
                  size="sm"
                  onClick={handleSaveNotifications}
                  disabled={savingNotif}
                  className="cursor-pointer"
                >
                  {savingNotif ? 'Saving...' : 'Save Preferences'}
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="appearance" className="space-y-6">
          <AppearanceSection />
        </TabsContent>
      </Tabs>
    </div>
  );
}