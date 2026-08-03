/**
 * The single PocketBase client for the dashboard.
 *
 * One instance per tab, because `authStore` is per-instance state: a second
 * client would not see a login performed on the first, and the symptom is an
 * intermittently-401ing page that works after a refresh.
 */

import PocketBase from 'pocketbase';

/**
 * In development this is a relative path, so requests go through the Vite
 * proxy to 127.0.0.1:8090 and are same-origin. In production Caddy serves the
 * dashboard from app.anis.chat and the API from api.anis.chat, so the built
 * app needs the absolute URL.
 */
const baseUrl = import.meta.env['VITE_API_URL'] ?? '/';

export const pb = new PocketBase(baseUrl);

/**
 * Don't auto-cancel in-flight requests.
 *
 * PocketBase's SDK cancels a pending request when an identical one is issued.
 * That is a sensible default for a search box and a bad one for a dashboard
 * that fires the same list query from several mounted panels — the losers
 * reject with a cancellation the components then render as an error.
 */
pb.autoCancellation(false);

/** The signed-in user, or null. Not reactive — see `useAuth`. */
export function currentUser() {
  return pb.authStore.record;
}

export function isSignedIn(): boolean {
  return pb.authStore.isValid;
}

/**
 * Subscribe to auth changes. Returns an unsubscribe function.
 *
 * Fires on sign-in, sign-out, and token refresh — including refreshes made in
 * ANOTHER tab, because the SDK's default store persists to localStorage and
 * listens for storage events.
 */
export function onAuthChange(fn: () => void): () => void {
  return pb.authStore.onChange(fn);
}
