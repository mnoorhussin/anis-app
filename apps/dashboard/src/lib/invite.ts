/**
 * The invitation link a visitor arrived on, held until they are signed in.
 *
 * An invitation link is `/invite/<token>`. The token is read once at load and
 * the address bar is reset to `/`, so the secret does not sit in the visible
 * URL, in a screenshot, or in history entries made afterwards. It is kept in
 * sessionStorage so that signing up — which can mean a reload — does not lose
 * it, and in memory as well because storage can be unavailable.
 */

import { errorStatus } from './apiError.js';
import { pb } from './pocketbase.js';

const KEY = 'anis.invite';
let memory: string | null = null;

const SHAPE = /^inv_[0-9A-Z]{52}$/;

export interface InvitePreview {
  workspace_name: string;
  inviter: string;
  email: string;
  role: 'admin' | 'agent';
}

/** A link being acted on: what it is for, or why it cannot be used. */
export interface PendingInvite {
  token: string;
  preview: InvitePreview | null;
  problem: 'expired' | 'invalid' | null;
}

/** Looks a token up. Public: works before the visitor has an account. */
export async function loadInvite(token: string): Promise<PendingInvite> {
  try {
    const preview = (await pb.send(`/api/anis/invitations/preview/${encodeURIComponent(token)}`, {
      method: 'GET',
    })) as InvitePreview;
    return { token, preview, problem: null };
  } catch (err) {
    return { token, preview: null, problem: errorStatus(err) === 410 ? 'expired' : 'invalid' };
  }
}

/** Moves a token from the URL into storage. Call once, before rendering. */
export function captureInviteFromUrl(): void {
  const match = /^\/invite\/([^/?#]+)\/?$/.exec(window.location.pathname);
  if (!match) return;
  const token = decodeURIComponent(match[1] ?? '');
  window.history.replaceState(null, '', '/');
  if (!SHAPE.test(token)) return;
  memory = token;
  try {
    sessionStorage.setItem(KEY, token);
  } catch {
    // Private mode or blocked storage: the in-memory copy still works for
    // everything short of a reload.
  }
}

export function pendingInvite(): string | null {
  if (memory) return memory;
  try {
    return sessionStorage.getItem(KEY);
  } catch {
    return null;
  }
}

export function clearPendingInvite(): void {
  memory = null;
  try {
    sessionStorage.removeItem(KEY);
  } catch {
    // Nothing to clear.
  }
}
