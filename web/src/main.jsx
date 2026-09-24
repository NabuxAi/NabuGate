import React from 'react';
import { createRoot } from 'react-dom/client';

// Global styles first: a view's own stylesheet (landing.css, docs.css) is
// pulled in through App's imports, and has to come after these in the bundle
// so it can refine them rather than be overridden by them.
import './styles/fonts.css';
import './styles/tokens.css';
import './styles/app.css';
import './styles/polish.css';
import './styles/shell.css';
import { I18nProvider } from './i18n/index.jsx';
import App from './App.jsx';

createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <I18nProvider>
      <App />
    </I18nProvider>
  </React.StrictMode>
);
