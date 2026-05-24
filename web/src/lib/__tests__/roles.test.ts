import { describe, expect, it } from 'vitest';
import { canAccessAdmin, ROLE_SUPER_ADMIN } from '../roles';

describe('role helpers', () => {
  it('allows only platform super admins into the admin surface', () => {
    expect(canAccessAdmin(ROLE_SUPER_ADMIN)).toBe(true);
    expect(canAccessAdmin('role_admin')).toBe(false);
    expect(canAccessAdmin('role_owner')).toBe(false);
    expect(canAccessAdmin(undefined)).toBe(false);
  });
});
