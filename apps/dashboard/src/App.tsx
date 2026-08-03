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
 */
function Routes() {
  useAuthRefresh();
  const { isSignedIn } = useAuth();
  return isSignedIn ? <WorkspaceScreen /> : <AuthScreen />;
}

export function App() {
  return (
    <LanguageProvider>
      <Routes />
    </LanguageProvider>
  );
}
