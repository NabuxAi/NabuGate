import { useSyncExternalStore } from 'react';

/*
 * One theme for the whole page.
 *
 * This used to be a useState in every component that called it, so the
 * sidebar, the sign-in screen and the app root each held their own copy: a
 * toggle in one left the others showing the wrong label until they remounted.
 * A single module-level store with subscribers keeps them in step.
 */

const KEY = 'theme';
const listeners = new Set();

function read() {
  try {
    const saved = localStorage.getItem(KEY);
    if (saved === 'light' || saved === 'dark') return saved;
  } catch {
    /* storage blocked */
  }
  return 'dark';
}

// The phone's browser bar takes this colour; left at the dark default it drew
// a black band above the light theme. Same values as --ng-bg in tokens.css.
const CHROME = { dark: '#070b16', light: '#f5f7fc' };

function apply(next) {
  document.documentElement.setAttribute('data-theme', next);
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', CHROME[next]);
}

let theme = read();
apply(theme);

function set(next) {
  theme = next;
  apply(next);
  try { localStorage.setItem(KEY, next); } catch { /* storage blocked */ }
  listeners.forEach((l) => l());
}

const subscribe = (l) => {
  listeners.add(l);
  return () => listeners.delete(l);
};

export function useTheme() {
  const value = useSyncExternalStore(subscribe, () => theme);
  return { theme: value, setTheme: set, toggleTheme: () => set(value === 'dark' ? 'light' : 'dark') };
}
