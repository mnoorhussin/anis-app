import { useCallback, useEffect, useSyncExternalStore } from 'react';

import { pb } from './pocketbase.js';

/**
 * The signed-in user, kept in step with the SDK's auth store.
 *
 * `useSyncExternalStore` rather than `useState` + an effect because the auth
 * store is genuinely external state that changes outside React: a token
 * refresh, or a sign-out performed in another tab, both fire `onChange`. With
 * an effect, those updates arrive a render late and a signed-out tab keeps
 * showing the dashboard until something else re-renders it.
 */
/**
 * The snapshot is cached in a module variable and refreshed only when the auth
 * store notifies.
 *
 * `useSyncExternalStore` requires `getSnapshot` to return a stable reference
 * between changes, and reading `pb.authStore.record` directly does NOT satisfy
 * that — the SDK hands back a fresh object each access. React then sees a new
 * value on every render, re-renders to catch up, sees another new value, and
 * gives up with "The result of getSnapshot should be cached to avoid an
 * infinite loop", rendering nothing at all. Symptom is a blank page
 * immediately after sign-in.
 */
let snapshot = pb.authStore.record;

function subscribe(onChange: () => void): () => void {
  return pb.authStore.onChange(() => {
    snapshot = pb.authStore.record;
    onChange();
  });
}

function getSnapshot() {
  return snapshot;
}

export interface AuthState {
  user: ReturnType<typeof getSnapshot>;
  isSignedIn: boolean;
}

export function useAuth(): AuthState {
  const user = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  return { user, isSignedIn: user !== null };
}

/**
 * Refresh the stored token once on mount.
 *
 * The SDK persists the token to localStorage, so a returning visitor appears
 * signed in immediately — even if the token expired last week, or the account
 * was deleted. Without this, the app renders a dashboard and then fails every
 * request with a 401, which reads as "the app is broken" rather than "please
 * sign in again". A failed refresh clears the store and drops them at the
 * sign-in screen, which is the honest outcome.
 */
export function useAuthRefresh(): void {
  useEffect(() => {
    if (!pb.authStore.isValid) return;
    pb.collection('users')
      .authRefresh()
      .catch(() => {
        pb.authStore.clear();
      });
  }, []);
}

export function useSignOut(): () => void {
  return useCallback(() => {
    pb.authStore.clear();
  }, []);
}
