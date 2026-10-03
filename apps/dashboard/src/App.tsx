import { useCallback, useEffect, useState } from 'react';

import {
  captureInviteFromUrl,
  clearPendingInvite,
  loadInvite,
  pendingInvite,
  type PendingInvite,
} from './lib/invite.js';
import { LanguageProvider } from './lib/LanguageContext.js';
import { useAuth, useAuthRefresh } from './lib/useAuth.js';
import { AuthScreen } from './screens/AuthScreen.js';
import { WorkspaceScreen } from './screens/WorkspaceScreen.js';

/**
 * No router yet, deliberately.
 *
 * There are two states — signed out and signed in — and a router would add a
 * dependency, a layout abstraction and a set of route guards to express an
 * `if`. It goes in when there is a second authenticated screen to navigate to.
 *
 * The one path that means something, `/invite/<token>`, is read once at load
 * (see lib/invite.ts) and carried through sign-in as state.
 */
function Routes() {
  useAuthRefresh();
  const { isSignedIn } = useAuth();

  const [token, setToken] = useState<string | null>(() => {
    captureInviteFromUrl();
    return pendingInvite();
  });
  const [invite, setInvite] = useState<PendingInvite | null>(null);

  useEffect(() => {
    if (!token) {
      setInvite(null);
      return;
    }
    let cancelled = false;
    void loadInvite(token).then((i) => {
      if (!cancelled) setInvite(i);
    });
    return () => {
      cancelled = true;
    };
  }, [token]);

  const done = useCallback(() => {
    clearPendingInvite();
    setToken(null);
  }, []);

  return isSignedIn ? (
    <WorkspaceScreen invite={invite} onInviteDone={done} />
  ) : (
    <AuthScreen invite={invite} />
  );
}

export function App() {
  return (
    <LanguageProvider>
      <Routes />
    </LanguageProvider>
  );
}
