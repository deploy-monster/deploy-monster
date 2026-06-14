// P1-12: Migrate hand-rolled mutation state to useMutation hook
import { useEffect, useRef, useState } from 'react';
import { User, Save } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { useAuthStore } from '@/stores/auth';
import { useMutation } from '@/hooks';
import { toast } from '@/stores/toastStore';
import { getInitialsValue } from './helpers';

interface ProfileSectionProps {
  onSave?: () => void;
}

export function ProfileSection({ onSave }: ProfileSectionProps) {
  const user = useAuthStore((s) => s.user);
  const updateUser = useAuthStore((s) => s.updateUser);
  const userID = user?.id ?? '';
  const userName = user?.name ?? '';
  const [editName, setEditName] = useState(userName);
  const initializedUserID = useRef(userID);
  const lastSyncedName = useRef(userName);

  useEffect(() => {
    if (userID !== initializedUserID.current || editName === lastSyncedName.current) {
      initializedUserID.current = userID;
      lastSyncedName.current = userName;
      setEditName(userName);
    }
  }, [editName, userID, userName]);

  const { mutate: saveProfile, loading: saving } = useMutation<{ name: string }, { name?: string }>('patch', '/auth/me');

  const handleSaveProfile = () => {
    void saveProfile(
      { name: editName },
      {
        onSuccess: (updatedUser) => {
          updateUser({ name: updatedUser?.name || editName });
          toast.success('Profile updated');
          onSave?.();
        },
        onError: (err) => { toast.error(err); },
      },
    ).catch(() => undefined);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <User className="size-4 text-primary" />
          Profile Information
        </CardTitle>
        <CardDescription>Update your personal information.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Avatar */}
        <div className="flex items-center gap-4">
          <Avatar className="size-16">
            <AvatarFallback className="text-lg bg-primary/10 text-primary">
              {getInitialsValue(user?.name || 'U')}
            </AvatarFallback>
          </Avatar>
          <div>
            <p className="text-sm font-medium">{user?.name}</p>
            <p className="text-xs text-muted-foreground">{user?.email}</p>
          </div>
        </div>

        {/* Form fields */}
        <div className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <Label htmlFor="profile-name">Display Name</Label>
              <Input
                id="profile-name"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                placeholder="Your name"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="profile-email">Email Address</Label>
              <Input
                id="profile-email"
                value={user?.email || ''}
                readOnly
                className="bg-muted"
              />
              <p className="text-[11px] text-muted-foreground">Email cannot be changed.</p>
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="profile-role">Role</Label>
            <Input
              id="profile-role"
              value={user?.role?.replace('role_', '').replace('_', ' ') || 'user'}
              readOnly
              className="bg-muted capitalize"
            />
          </div>
        </div>

        <div className="flex justify-end">
          <Button
            size="sm"
            onClick={handleSaveProfile}
            disabled={saving || !editName.trim()}
            className="cursor-pointer"
          >
            {saving ? (
              <>
                <Save className="size-3.5 animate-spin" />
                Saving...
              </>
            ) : (
              <>
                <Save className="size-3.5" />
                Save Profile
              </>
            )}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
