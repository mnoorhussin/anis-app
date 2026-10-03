/**
 * Workspace roles, mirroring pocketbase/routes/invitations.go.
 *
 * The backend enforces all of this; the dashboard only uses it to avoid
 * offering a control that would be refused. If the two ever disagree the
 * backend wins, and the user sees an error instead of a missing button — so
 * drift here is a UX bug, never a security one.
 */
export type Role = 'owner' | 'admin' | 'agent';

/** May change what a workspace knows and how it is set up. */
export function isManager(role: string | undefined): boolean {
  return role === 'owner' || role === 'admin';
}

/** Roles someone with `actor` may invite people as. */
export function assignableRoles(actor: string | undefined): Role[] {
  if (actor === 'owner') return ['admin', 'agent'];
  if (actor === 'admin') return ['agent'];
  return [];
}

/** Whether `actor` may remove a member with role `target` (`self`: leaving). */
export function canRemoveMember(actor: string | undefined, target: string, self: boolean): boolean {
  if (target === 'owner') return false;
  if (self) return true;
  if (actor === 'owner') return true;
  if (actor === 'admin') return target === 'agent';
  return false;
}
