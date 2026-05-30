import { useState } from 'react';
import { User, Save } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { useAuthStore } from '@/stores/auth';
import { api } from '@/api/client';
import { toast } from '@/stores/toastStore';

function getInitials(name: string) {
  return name.split(' ').map((n) => n[0]).join('').toUpperCase().slice(0, 2);
}

export function getInitialsValue(name: string) {
  return getInitials(name);
}

interface ProfileSectionProps {
  onSave?: () => void;
}

export function ProfileSection({ onSave }: ProfileSectionProps) {
  const user = useAuthStore((s) => s.user);
  const updateUser = useAuthStore((s) => s.updateUser);
  const [editName, setEditName] = useState(user?.name || '');
  const [saving, setSaving] = useState(false);

  const handleSaveProfile = async () => {
    setSaving(true);
    try {
      const updatedUser = await api.patch<{ name?: string }>('/auth/me', { name: editName });
      updateUser({ name: updatedUser?.name || editName });
      toast.success('Profile updated');
      onSave?.();
    } catch {
      toast.error('Failed to update profile');
    } finally {
      setSaving(false);
    }
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
              {getInitials(user?.name || 'U')}
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
            disabled={saving || !editName.trim() || editName === user?.name}
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
                Save Changes
              </>
            )}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}