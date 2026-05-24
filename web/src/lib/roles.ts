export const ROLE_SUPER_ADMIN = 'role_super_admin';

export function canAccessAdmin(role: string | null | undefined): boolean {
  return role === ROLE_SUPER_ADMIN;
}
