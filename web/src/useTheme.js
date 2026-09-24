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

let theme = read();
document.documentElement.setAttribute('data-theme', theme);

function set(next) {
  theme = next;
  document.documentElement.setAttribute('data-theme', next);
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
