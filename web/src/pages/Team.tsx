import { useState } from 'react';
import {
  Users, UserPlus, Clock, Mail, Trash2,
} from 'lucide-react';
import { api } from '@/api/client';
import { useApi } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
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
import { toast } from '@/components/Toast';
interface TeamMember {
  id: string;
  name: string;
  email: string;
  role: string;
  avatar_url?: string;
  joined_at: string;
}
interface AuditEntry {
  id: number;
  action: string;
  user_name: string;
  resource_type: string;
  resource_id: string;
  ip_address: string;
  created_at: string;
}
function RoleBadge({ role }: { role: string }) {
  const label = role.replace('role_', '');
  switch (label) {
    case 'admin':
      return <Badge>Admin</Badge>;
    case 'developer':
      return <Badge variant="secondary">Developer</Badge>;
    case 'operator':
      return <Badge variant="outline">Operator</Badge>;
    case 'viewer':
      return <Badge variant="outline">Viewer</Badge>;
    default:
      return <Badge variant="outline">{label}</Badge>;
  }
}
function getInitials(name: string) {
  return name
    .split(' ')
    .map((n) => n[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);
}
export function Team() {
  const { data: members, loading: membersLoading, refetch: refetchMembers } = useApi<TeamMember[]>('/team/members');
  const { data: auditLog, loading: auditLoading } = useApi<AuditEntry[]>('/team/audit-log');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteRole, setInviteRole] = useState('role_developer');
  const handleInvite = async () => {
    if (!inviteEmail) return;
    try {
      await api.post('/team/invites', { email: inviteEmail, role_id: inviteRole });
      toast.success('Invite sent');
      setInviteEmail('');
      setDialogOpen(false);
      refetchMembers();
    } catch {
      toast.error('Failed to send invite');
    }
  };
  const handleRemove = async (id: string) => {
    if (!confirm('Remove this team member?')) return;
    try {
      await api.delete(`/team/members/${id}`);
      toast.success('Member removed');
      refetchMembers();
    } catch {
      toast.error('Failed to remove member');
    }
  };
  const memberList = members || [];
  const auditList = auditLog || [];
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Team Management</h1>
          <p className="text-sm text-muted-foreground mt-1">
            {memberList.length} member{memberList.length !== 1 ? 's' : ''}
          </p>
        </div>
        <Button onClick={() => setDialogOpen(true)}>
          <UserPlus /> Invite Member
        </Button>
      </div>
      {/* Invite Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent onClose={() => setDialogOpen(false)}>
          <DialogHeader>
            <DialogTitle>Invite Team Member</DialogTitle>
            <DialogDescription>
              Send an invitation email to a new team member.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="invite-email">Email Address</Label>
              <Input
                id="invite-email"
                type="email"
                value={inviteEmail}
                onChange={(e) => setInviteEmail(e.target.value)}
                placeholder="colleague@company.com"
              />
            </div>
            <div className="space-y-2">
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
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleInvite} disabled={!inviteEmail}>
              <Mail size={14} /> Send Invite
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {/* Tabs */}
      <Tabs defaultValue="members">
        <TabsList>
          <TabsTrigger value="members">
            <Users size={14} /> Members
          </TabsTrigger>
          <TabsTrigger value="audit">
            <Clock size={14} /> Audit Log
          </TabsTrigger>
        </TabsList>
        {/* Members Tab */}
        <TabsContent value="members">
          {membersLoading && (
            <Card>
              <CardContent className="space-y-3 py-2">
                {[1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-14 w-full" />
                ))}
              </CardContent>
            </Card>
          )}
          {!membersLoading && memberList.length === 0 && (
            <Card className="py-16">
              <CardContent className="flex flex-col items-center text-center">
                <Users className="mb-4 text-muted-foreground" size={48} />
                <h2 className="text-lg font-medium mb-2">No team members yet</h2>
                <p className="text-muted-foreground">
                  Use the invite button to add members to your team.
                </p>
              </CardContent>
            </Card>
          )}
          {!membersLoading && memberList.length > 0 && (
            <Card className="py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Member</TableHead>
                    <TableHead>Email</TableHead>
                    <TableHead>Role</TableHead>
                    <TableHead>Joined</TableHead>
                    <TableHead className="w-[80px]">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {memberList.map((m) => (
                    <TableRow key={m.id}>
                      <TableCell>
                        <div className="flex items-center gap-3">
                          <Avatar>
                            <AvatarFallback>{getInitials(m.name || m.email)}</AvatarFallback>
                          </Avatar>
                          <span className="font-medium">{m.name || '--'}</span>
                        </div>
                      </TableCell>
                      <TableCell className="text-muted-foreground">{m.email}</TableCell>
                      <TableCell>
                        <RoleBadge role={m.role} />
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {new Date(m.joined_at).toLocaleDateString()}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleRemove(m.id)}
                          className="text-muted-foreground hover:text-destructive"
                          title="Remove member"
                        >
                          <Trash2 size={14} />
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
              <CardContent className="space-y-3 py-2">
                {[1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </CardContent>
            </Card>
          )}
          {!auditLoading && auditList.length === 0 && (
            <Card className="py-16">
              <CardContent className="flex flex-col items-center text-center">
                <Clock className="mb-4 text-muted-foreground" size={48} />
                <h2 className="text-lg font-medium mb-2">No audit log entries</h2>
                <p className="text-muted-foreground">
                  Team activity will be logged here.
                </p>
              </CardContent>
            </Card>
          )}
          {!auditLoading && auditList.length > 0 && (
            <Card className="py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Timestamp</TableHead>
                    <TableHead>User</TableHead>
                    <TableHead>Action</TableHead>
                    <TableHead>Resource</TableHead>
                    <TableHead>IP Address</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {auditList.map((entry) => (
                    <TableRow key={entry.id}>
                      <TableCell className="text-muted-foreground">
                        {new Date(entry.created_at).toLocaleString()}
                      </TableCell>
                      <TableCell className="font-medium">
                        {entry.user_name || '--'}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline">{entry.action}</Badge>
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {entry.resource_type}/{entry.resource_id}
                      </TableCell>
                      <TableCell className="font-mono text-sm text-muted-foreground">
                        {entry.ip_address}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}
