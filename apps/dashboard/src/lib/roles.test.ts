import { describe, expect, it } from 'vitest';

import { assignableRoles, canRemoveMember, isManager } from './roles.js';

// Same matrices as pocketbase/routes/invitations_test.go. If one side changes
// and the other does not, the dashboard offers buttons the backend refuses.

describe('roles', () => {
  it('only owners and admins manage', () => {
    expect(isManager('owner')).toBe(true);
    expect(isManager('admin')).toBe(true);
    expect(isManager('agent')).toBe(false);
    expect(isManager(undefined)).toBe(false);
  });

  it('owners invite admins and agents; admins only agents; agents no one', () => {
    expect(assignableRoles('owner')).toEqual(['admin', 'agent']);
    expect(assignableRoles('admin')).toEqual(['agent']);
    expect(assignableRoles('agent')).toEqual([]);
    expect(assignableRoles(undefined)).toEqual([]);
  });

  it('never removes the owner, lets anyone else leave', () => {
    const cases: [string, string, boolean, boolean][] = [
      ['owner', 'admin', false, true],
      ['owner', 'agent', false, true],
      ['admin', 'agent', false, true],
      ['admin', 'admin', false, false],
      ['agent', 'agent', false, false],
      ['admin', 'owner', false, false],
      ['owner', 'owner', true, false],
      ['agent', 'agent', true, true],
    ];
    for (const [actor, target, self, want] of cases) {
      expect(canRemoveMember(actor, target, self), `${actor} -> ${target} self=${self}`).toBe(want);
    }
  });
});
