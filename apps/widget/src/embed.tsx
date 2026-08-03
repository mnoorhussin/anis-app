/**
 * Widget entry point — this is what `cdn.anis.chat/widget.js` is.
 *
 * The one-line install:
 *
 *   <script src="https://cdn.anis.chat/widget.js" data-anis-key="wk_..." defer></script>
 *
 * Everything else (name, colours, greeting, suggested questions, language) is
 * fetched from the API using that key, so a business changes its widget in the
 * dashboard without touching its site.
 *
 * Rules this file exists to keep:
 *  - Never throw into the host page. A broken widget must not break a
 *    customer's storefront. Every failure path degrades to "no widget".
 *  - Never touch the host document beyond one element and its shadow root.
 *  - Never render before config arrives, so the widget cannot flash the
 *    default brand and then repaint into the customer's.
 */

import { render } from 'preact';

import { App } from './App.js';
import { loadConfig } from './api.js';
import { hostVars, resolveTheme } from './theme.js';
// `?inline` gives the compiled Tailwind output as a string instead of a
// separate stylesheet request. It has to be a string: the styles belong inside
// the shadow root, and a normal CSS import would put them in document.head
// where they would both miss the shadow tree and leak onto the host page.
import css from './widget.css?inline';

const HOST_TAG = 'anis-chat-widget';

function currentScript(): HTMLScriptElement | null {
  if (document.currentScript instanceof HTMLScriptElement) return document.currentScript;
  // `defer`/`async` execution can leave currentScript null; fall back to
  // finding our own tag.
  return document.querySelector<HTMLScriptElement>('script[data-anis-key]');
}

async function boot(): Promise<void> {
  if (customElements.get(HOST_TAG) || document.querySelector(HOST_TAG)) {
    // Two copies of the install snippet on one page is a common
    // copy-paste mistake. Render once.
    return;
  }

  const script = currentScript();
  const key = script?.dataset['anisKey'];
  if (!key) {
    // Deliberately quiet in production: a console error on a customer's site
    // for our misconfiguration is noise they cannot act on. The dashboard's
    // install screen is where this gets reported.
    if (import.meta.env.DEV) {
      console.error('[anis] missing data-anis-key on the widget script tag');
    }
    return;
  }

  const apiUrl = script?.dataset['anisApi'] ?? import.meta.env['VITE_API_URL'] ?? '';

  const config = await loadConfig(apiUrl, key);
  if (!config) return; // key unknown, domain not allow-listed, or API down

  const host = document.createElement(HOST_TAG);
  host.setAttribute('data-position', config.position);
  const shadow = host.attachShadow({ mode: 'open' });

  const theme = resolveTheme(config.theme);

  // Constructable stylesheets keep the CSS out of the DOM and let every future
  // widget instance share one parsed copy. Falls back to a <style> element for
  // browsers without adoptedStyleSheets.
  const styleText = `${hostVars(theme, config.accentColor)}\n${css}`;
  if ('adoptedStyleSheets' in Document.prototype && typeof CSSStyleSheet === 'function') {
    try {
      const sheet = new CSSStyleSheet();
      sheet.replaceSync(styleText);
      shadow.adoptedStyleSheets = [sheet];
    } catch {
      appendStyleTag(shadow, styleText);
    }
  } else {
    appendStyleTag(shadow, styleText);
  }

  const mount = document.createElement('div');
  shadow.appendChild(mount);
  document.body.appendChild(host);

  render(<App apiUrl={apiUrl} widgetKey={key} config={config} theme={theme} />, mount);
}

function appendStyleTag(root: ShadowRoot, text: string): void {
  const style = document.createElement('style');
  style.textContent = text;
  root.appendChild(style);
}

// Never let an exception escape into the host page.
boot().catch((err: unknown) => {
  if (import.meta.env.DEV) console.error('[anis] widget failed to start', err);
});
