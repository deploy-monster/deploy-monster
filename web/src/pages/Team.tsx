import { useState } from 'react';
import {
  Users,
  UserPlus,
  Clock,
  Mail,
  Trash2,
  Shield,
  CircleDot,
  Settings,
  LogIn,
  LogOut,
  UserCog,
  Key,
} from 'lucide-react';
import { teamAPI, type TeamMember, type AuditEntry } from '@/api/team';
import { useApi } from '@/hooks';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Select } from '@/components/ui/select';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import {
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
} from '@/components/ui/table';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from '@/stores/toastStore';
import { AlertDialog } from '@/components/ui/alert-dialog';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const ROLE_CONFIG: Record<string, { variant: 'default' | 'secondary' | 'outline'; dotColor: string; label: string }> = {
  admin:     { variant: 'default',   dotColor: 'bg-red-500',           label: 'Admin' },
  developer: { variant: 'secondary', dotColor: 'bg-blue-500',          label: 'Developer' },
  operator:  { variant: 'outline',   dotColor: 'bg-amber-500',         label: 'Operator' },
  viewer:    { variant: 'outline',   dotColor: 'bg-muted-foreground',  label: 'Viewer' },
};

const AUDIT_COLORS: Record<string, string> = {
  login:     'bg-emerald-500',
  logout:    'bg-muted-foreground',
  create:    'bg-blue-500',
  update:    'bg-cyan-500',
  delete:    'bg-destructive',
  invite:    'bg-purple-500',
  deploy:    'bg-emerald-500',
  settings:  'bg-amber-500',
  auth:      'bg-indigo-500',
};

const AUDIT_ICONS: Record<string, typeof CircleDot> = {
  login:     LogIn,
  logout:    LogOut,
  create:    UserPlus,
  update:    UserCog,
  delete:    Trash2,
  invite:    Mail,
  deploy:    CircleDot,
  settings:  Settings,
  auth:      Key,
};

function getInitials(name: string) {
  return name
    .split(' ')
    .map((n) => n[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);
}

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  return `${months}mo ago`;
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function RoleBadge({ role }: { role: string }) {
  const label = role.replace('role_', '');
  const config = ROLE_CONFIG[label] || ROLE_CONFIG.viewer;
  return (
    <Badge variant={config.variant} className="gap-1.5">
      <span className={cn('size-1.5 rounded-full', config.dotColor)} />
      {config.label}
    </Badge>
  );
}

function MemberRowSkeleton() {
  return (
    <TableRow>
      <TableCell>
        <div className="flex items-center gap-3">
          <Skeleton className="size-9 rounded-full" />
          <div className="space-y-1.5">
            <Skeleton className="h-4 w-28" />
            <Skeleton className="h-3 w-36" />
          </div>
        </div>
      </TableCell>
      <TableCell><Skeleton className="h-5 w-20 rounded-md" /></TableCell>
      <TableCell><Skeleton className="h-3 w-20" /></TableCell>
      <TableCell><Skeleton className="h-7 w-7 rounded-md" /></TableCell>
    </TableRow>
  );
}

function AuditSkeleton() {
  return (
    <div className="space-y-0">
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="flex gap-3 py-3.5">
          <Skeleton className="size-[18px] rounded-full shrink-0" />
          <div className="flex-1 space-y-1.5">
            <Skeleton className="h-3.5 w-48" />
            <Skeleton className="h-2.5 w-24" />
          </div>
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Team
// ---------------------------------------------------------------------------

export function Team() {
  const { data: members, loading: membersLoading, refetch: refetchMembers } = useApi<TeamMember[]>('/team/members');
  const { data: auditLog, loading: auditLoading } = useApi<AuditEntry[]>('/team/audit-log');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteRole, setInviteRole] = useState('role_developer');

  const handleInvite = async () => {
    if (!inviteEmail) return;
    try {
      await teamAPI.invite({ email: inviteEmail, role_id: inviteRole });
      toast.success('Invite sent');
      setInviteEmail('');
      setDialogOpen(false);
      refetchMembers();
    } catch {
      toast.error('Failed to send invite');
    }
  };

  const handleRemove = async (id: string) => {
    setRemoveMemberId(id);
  };

  const memberList = members || [];
  const auditList = auditLog || [];
  const [removeMemberId, setRemoveMemberId] = useState<string | null>(null);
  const pendingRemoveMember = memberList.find((m) => m.id === removeMemberId);

  return (
    <div className="space-y-8">
      {/* Hero Section */}
      <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-primary/3 to-transparent p-6 sm:p-8">
        <div className="relative z-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Users className="size-5 text-primary" />
              {memberList.length > 0 && (
                <Badge variant="secondary" className="text-xs font-normal">
                  {memberList.length} member{memberList.length !== 1 ? 's' : ''}
                </Badge>
              )}
            </div>
            <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
              Team Management
            </h1>
            <p className="text-muted-foreground mt-1.5 text-sm sm:text-base">
              Manage team members, roles, and review audit history.
            </p>
          </div>
          <Button onClick={() => setDialogOpen(true)} className="shrink-0">
            <UserPlus className="size-4" />
            Invite Member
          </Button>
        </div>
        {/* Decorative gradient circles */}
        <div className="pointer-events-none absolute -right-16 -top-16 size-64 rounded-full bg-primary/5 blur-3xl" />
        <div className="pointer-events-none absolute -left-8 -bottom-8 size-48 rounded-full bg-primary/3 blur-2xl" />
      </div>

      {/* Invite Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent onClose={() => setDialogOpen(false)} className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-3">
              <div className="flex items-center justify-center rounded-xl size-9 bg-primary">
                <Mail className="size-4 text-primary-foreground" />
              </div>
              Invite Team Member
            </DialogTitle>
            <DialogDescription>
              Send an invitation email to add a new member to your team.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="invite-email">Email Address</Label>
              <Input
                id="invite-email"
                type="email"
                value={inviteEmail}
                onChange={(e) => setInviteEmail(e.target.value)}
                placeholder="colleague@company.com"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="invite-role">Role</Label>
              <Select
                id="invite-role"
                value={inviteRole}
                onChange={(e) => setInviteRole(e.target.value)}
              >
                <option value="role_admin">Admin</option>
                <option value="role_developer">Developer</option>
                <option value="role_operator">Operator</option>
                <option value="role_viewer">Viewer</option>
              </Select>
              <p className="text-[11px] text-muted-foreground">
                Admins have full access. Developers can deploy and manage apps. Viewers are read-only.
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleInvite} disabled={!inviteEmail}>
              <Mail className="size-4" />
              Send Invite
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Tabs */}
      <Tabs defaultValue="members">
        <TabsList>
          <TabsTrigger value="members">
            <Users className="size-3.5" />
            Members
          </TabsTrigger>
          <TabsTrigger value="audit">
            <Clock className="size-3.5" />
            Audit Log
          </TabsTrigger>
        </TabsList>

        {/* Members Tab */}
        <TabsContent value="members">
          {membersLoading && (
            <Card className="py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Member</TableHead>
                    <TableHead>Role</TableHead>
                    <TableHead className="hidden sm:table-cell">Joined</TableHead>
                    <TableHead className="w-[80px]">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {Array.from({ length: 3 }).map((_, i) => (
                    <MemberRowSkeleton key={i} />
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}

          {!membersLoading && memberList.length === 0 && (
            <div className="flex flex-col items-center justify-center py-24 text-center">
              <div className="rounded-full bg-muted p-6 mb-5">
                <Users className="size-10 text-muted-foreground" />
              </div>
              <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">
                No team members yet
              </h2>
              <p className="text-muted-foreground max-w-sm text-sm mb-6">
                Invite colleagues to collaborate on deployments and manage applications together.
              </p>
              <Button onClick={() => setDialogOpen(true)}>
                <UserPlus className="size-4" />
                Invite your first member
              </Button>
            </div>
          )}

          {!membersLoading && memberList.length > 0 && (
            <Card className="py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Member</TableHead>
                    <TableHead>Role</TableHead>
                    <TableHead className="hidden sm:table-cell">Joined</TableHead>
                    <TableHead className="w-[80px] text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {memberList.map((m) => (
                    <TableRow key={m.id} className="group/row hover:bg-muted/50 transition-colors">
                      <TableCell>
                        <div className="flex items-center gap-3">
                          <Avatar className="size-9">
                            <AvatarFallback className="text-xs font-medium bg-primary/10 text-primary">
                              {getInitials(m.name || m.email)}
                            </AvatarFallback>
                          </Avatar>
                          <div className="min-w-0">
                            <p className="font-medium text-foreground truncate">
                              {m.name || '--'}
                            </p>
                            <p className="text-xs text-muted-foreground truncate">
                              {m.email}
                            </p>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <RoleBadge role={m.role} />
                      </TableCell>
                      <TableCell className="hidden sm:table-cell">
                        <span className="text-sm text-muted-foreground tabular-nums">
                          {timeAgo(m.joined_at)}
                        </span>
                      </TableCell>
                      <TableCell className="text-right">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleRemove(m.id)}
                          className="opacity-0 group-hover/row:opacity-100 transition-opacity text-muted-foreground hover:text-destructive"
                          title="Remove member"
                        >
                          <Trash2 className="size-4" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}
        </TabsContent>

        {/* Audit Log Tab */}
        <TabsContent value="audit">
          {auditLoading && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Recent Activity</CardTitle>
              </CardHeader>
              <CardContent>
                <AuditSkeleton />
              </CardContent>
            </Card>
          )}

          {!auditLoading && auditList.length === 0 && (
            <div className="flex flex-col items-center justify-center py-24 text-center">
              <div className="rounded-full bg-muted p-6 mb-5">
                <Shield className="size-10 text-muted-foreground" />
              </div>
              <h2 className="text-xl font-semibold tracking-tight text-foreground mb-2">
                No audit log entries
              </h2>
              <p className="text-muted-foreground max-w-sm text-sm">
                Team activity such as logins, deployments, and configuration changes will be logged here.
              </p>
            </div>
          )}

          {!auditLoading && auditList.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Recent Activity</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="relative">
                  {/* Timeline line */}
                  <div className="absolute left-[9px] top-2 bottom-2 w-px bg-border" />

                  <div className="space-y-0">
                    {auditList.map((entry, index) => {
                      const dotColor = AUDIT_COLORS[entry.action] || 'bg-muted-foreground';
                      const AuditIcon = AUDIT_ICONS[entry.action] || CircleDot;
                      return (
                        <div
                          key={entry.id}
                          className={cn(
                            'relative flex gap-3 py-3 pl-0',
                            index !== auditList.length - 1 && 'border-b border-transparent'
                          )}
                        >
                          {/* Timeline dot */}
                          <div className="relative z-10 flex items-center justify-center shrink-0">
                            <div className={cn(
                              'flex items-center justify-center size-[18px] rounded-full ring-2 ring-background',
                              dotColor
                            )}>
                              <AuditIcon className="size-2.5 text-white" />
                            </div>
                          </div>

                          {/* Content */}
                          <div className="flex-1 min-w-0 -mt-0.5">
                            <p className="text-sm text-foreground leading-snug">
                              <span className="font-medium">{entry.user_name || 'System'}</span>{' '}
                              <span className="text-muted-foreground capitalize">{entry.action}</span>{' '}
                              <span className="text-muted-foreground">{entry.resource_type}</span>
                            </p>
                            {entry.resource_id && (
                              <p className="text-xs text-muted-foreground/80 mt-0.5 truncate font-mono">
                                {entry.resource_id}
                              </p>
                            )}
                            <div className="flex items-center gap-3 mt-1">
                              <p className="text-[11px] text-muted-foreground/60 tabular-nums">
                                {timeAgo(entry.created_at)}
                              </p>
                              {entry.ip_address && (
                                <p className="text-[11px] text-muted-foreground/40 font-mono">
                                  {entry.ip_address}
                                </p>
                              )}
                            </div>
                          </div>

                          {/* Action badge */}
                          <Badge variant="outline" className="text-[10px] font-normal shrink-0 self-start mt-0.5">
                            {entry.action}
                          </Badge>
                        </div>
                      );
                    })}
                  </div>
                </div>
              </CardContent>
            </Card>
          )}
        </TabsContent>
      </Tabs>

      {/* Remove Member Confirmation Dialog */}
      <AlertDialog
        open={removeMemberId !== null}
        onOpenChange={(open) => !open && setRemoveMemberId(null)}
        title="Remove Team Member"
        description={`Remove "${pendingRemoveMember?.name || pendingRemoveMember?.email}" from the team?`}
        confirmLabel="Remove"
        cancelLabel="Cancel"
        variant="destructive"
        onConfirm={async () => {
          if (!removeMemberId) return;
          try {
            await teamAPI.removeMember(removeMemberId);
            toast.success('Member removed');
            refetchMembers();
          } catch {
            toast.error('Failed to remove member');
          } finally {
            setRemoveMemberId(null);
          }
        }}
      />
    </div>
  );
}
