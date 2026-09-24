import { useState, useEffect } from 'react';
import * as api from '../api.js';
import { useT } from '../i18n/index.jsx';
import Icon from '../components/Icon.jsx';
import { LangSwitch, Logo, ThemeToggle } from '../components/Brand.jsx';

const T = {
  fa: {
    tagline: 'دروازهٔ مرکزی هوش مصنوعی',
    pitchTitle: 'یک کلید، همهٔ مدل‌ها.',
    pitchBody: 'به GPT، Claude، Gemini، DeepSeek و ده‌ها مدل دیگر از یک API سازگار با OpenAI وصل شوید؛ با fallback خودکار، کلید جدا برای هر برنامه و صورت‌حساب شفاف.',
    p1: 'سازگار با Cursor، Claude Code، Codex و SDKهای OpenAI',
    p2: 'مسیریابی هوشمند و fallback بین ارائه‌دهنده‌ها',
    p3: 'پرداخت به‌ازای مصرف؛ موجودی منقضی نمی‌شود',
    home: 'صفحهٔ اصلی',
    loginTitle: 'خوش برگشتید',
    loginLead: 'برای ادامه وارد حساب NabuGate شوید.',
    adminTitle: 'ورود به مدیریت',
    adminLead: 'این بخش فقط برای مدیران دروازه است.',
    signupTitle: 'ساخت حساب',
    signupLead: 'چند ثانیه طول می‌کشد؛ بعد از آن کلید API بسازید.',
    setupTitle: 'راه‌اندازی اولیه',
    setupNote: 'هنوز حسابی ساخته نشده. اولین حساب را همین‌جا بساز — بعد از آن این فرم بسته می‌شود و کسی نمی‌تواند از بیرون حساب اضافه کند.',
    tabLogin: 'ورود',
    tabSignup: 'ثبت‌نام',
    username: 'نام کاربری',
    email: 'ایمیل',
    emailOrUser: 'ایمیل یا نام کاربری',
    password: 'رمز عبور',
    confirm: 'تکرار رمز عبور',
    pwHint: 'حداقل ۱۰ نویسه. یک عبارت بلند از یک رمز کوتاهِ پیچیده امن‌تر است.',
    mismatch: 'رمز عبور و تکرارش یکی نیستند.',
    failed: 'عملیات ناموفق بود.',
    expired: 'نشست شما منقضی شده است. لطفاً دوباره تلاش کنید.',
    ssoFailed: 'ورود با SSO ناموفق بود.',
    ssoError: 'خطای SSO: {code}',
    submitSetup: 'ساخت حساب مدیر',
    submitSignup: 'ساخت حساب',
    submitLogin: 'ورود',
    or: 'یا',
    nabu: 'ورود با حساب نابو',
    google: 'ورود با حساب گوگل',
  },
  en: {
    tagline: 'The central AI gateway',
    pitchTitle: 'One key. Every model.',
    pitchBody: 'Reach GPT, Claude, Gemini, DeepSeek and dozens more through one OpenAI-compatible API — with automatic fallback, a key per app and billing you can read.',
    p1: 'Works with Cursor, Claude Code, Codex and the OpenAI SDKs',
    p2: 'Smart routing and fallback across providers',
    p3: 'Pay as you go; credit never expires',
    home: 'Home',
    loginTitle: 'Welcome back',
    loginLead: 'Sign in to your NabuGate account to continue.',
    adminTitle: 'Administrator sign-in',
    adminLead: 'This area is for gateway administrators only.',
    signupTitle: 'Create your account',
    signupLead: 'It takes a few seconds; then create an API key.',
    setupTitle: 'First-time setup',
    setupNote: 'No account exists yet. Create the first one here — after that this form closes and nobody can add accounts from outside.',
    tabLogin: 'Sign in',
    tabSignup: 'Sign up',
    username: 'Username',
    email: 'Email',
    emailOrUser: 'Email or username',
    password: 'Password',
    confirm: 'Confirm password',
    pwHint: 'At least 10 characters. A long phrase is safer than a short complex password.',
    mismatch: 'The passwords do not match.',
    failed: 'That did not work.',
    expired: 'Your session expired. Please try again.',
    ssoFailed: 'Single sign-on failed.',
    ssoError: 'SSO error: {code}',
    submitSetup: 'Create admin account',
    submitSignup: 'Create account',
    submitLogin: 'Sign in',
    or: 'or',
    nabu: 'Continue with Nabu',
    google: 'Continue with Google',
  },
};

const GOOGLE = (
  <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"><path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/><path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/><path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/><path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/></svg>
);

/*
 * The console's gate.
 */
export default function SignIn({ needsSetup, onAuthenticated }) {
  const t = useT(T);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [mode, setMode] = useState(needsSetup ? 'setup' : 'login'); // 'setup', 'login', 'signup'
  const [nabu, setNabu] = useState(null);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const errParam = params.get('nabu_error');
    if (errParam) {
      // Stored as a key rather than a sentence so a language switch
      // re-renders it in the new language.
      if (errParam === 'expired') setError({ key: 'expired' });
      else if (errParam === 'failed') setError({ key: 'ssoFailed' });
      else setError({ key: 'ssoError', vars: { code: errParam } });
    }

    if (!needsSetup) {
      api.statusNabu().then(setNabu).catch(console.error);
    }
  }, [needsSetup]);

  const creating = mode === 'setup' || mode === 'signup';
  const isPanel = window.location.pathname.startsWith('/panel');
  const isAdmin = mode === 'login' && !isPanel;

  async function submit(e) {
    e.preventDefault();
    setError(null);

    if (creating && password !== confirm) {
      setError({ key: 'mismatch' });
      return;
    }
    setBusy(true);
    try {
      if (mode === 'setup') await api.setup(username, password);
      else if (mode === 'signup') await api.signup(username, password);
      else await api.login(username, password);
      onAuthenticated();
    } catch (err) {
      setError(err.message ? { text: err.message } : { key: 'failed' });
    } finally {
      setBusy(false);
    }
  }

  const switchMode = (m) => { setMode(m); setError(null); };

  const title = mode === 'setup' ? t('setupTitle') : mode === 'signup' ? t('signupTitle') : isAdmin ? t('adminTitle') : t('loginTitle');
  const lead = mode === 'signup' ? t('signupLead') : isAdmin ? t('adminLead') : t('loginLead');
  const errorText = error && (error.text || t(error.key, error.vars));

  return (
    <div className="auth">
      <aside className="auth-art" aria-hidden="true">
        <div className="auth-brand">
          <Logo size={42} />
          <div>NabuGate<small>{t('tagline')}</small></div>
        </div>
        <div className="auth-pitch">
          <h1>{t('pitchTitle')}</h1>
          <p>{t('pitchBody')}</p>
          <ul className="auth-points">
            <li><span className="ic"><Icon name="terminal" size={17} /></span>{t('p1')}</li>
            <li><span className="ic"><Icon name="route" size={17} /></span>{t('p2')}</li>
            <li><span className="ic"><Icon name="wallet" size={17} /></span>{t('p3')}</li>
          </ul>
        </div>
        <div className="auth-models" dir="ltr">
          {['gpt-5', 'claude-sonnet', 'gemini-2.5-pro', 'deepseek-v3', 'llama-4', 'nabu-smart'].map((m) => <span key={m}>{m}</span>)}
        </div>
      </aside>

      <main className="auth-side">
        <div className="auth-top">
          <a href="/" className="auth-home"><Icon name="arrow" size={15} className="flip-back" />{t('home')}</a>
          <div className="auth-top-controls">
            <LangSwitch size="sm" />
            <ThemeToggle />
          </div>
        </div>

        <form className="auth-card fade-in" onSubmit={submit}>
          <h2>{title}</h2>
          {mode !== 'setup' && <p className="auth-lead">{lead}</p>}

          {mode === 'setup' && <p className="auth-note">{t('setupNote')}</p>}

          {!needsSetup && (
            <div className="seg auth-tabs" role="tablist">
              <button type="button" role="tab" aria-selected={mode === 'login'} className={'seg-btn' + (mode === 'login' ? ' active' : '')} onClick={() => switchMode('login')}>{t('tabLogin')}</button>
              <button type="button" role="tab" aria-selected={mode === 'signup'} className={'seg-btn' + (mode === 'signup' ? ' active' : '')} onClick={() => switchMode('signup')}>{t('tabSignup')}</button>
            </div>
          )}

          <label className="field">
            <span>{mode === 'setup' ? t('username') : mode === 'signup' ? t('email') : t('emailOrUser')}</span>
            <input
              className="input"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              dir="ltr"
              required
            />
          </label>

          <label className="field">
            <span>{t('password')}</span>
            <input
              className="input"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete={creating ? 'new-password' : 'current-password'}
              dir="ltr"
              required
            />
            {creating && <span className="hint">{t('pwHint')}</span>}
          </label>

          {creating && (
            <label className="field">
              <span>{t('confirm')}</span>
              <input
                className="input"
                type="password"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                autoComplete="new-password"
                dir="ltr"
                required
              />
            </label>
          )}

          {errorText && <p className="auth-error" role="alert"><Icon name="alert" size={16} />{errorText}</p>}

          <button className="btn btn-primary auth-submit" disabled={busy}>
            {busy ? '…' : mode === 'setup' ? t('submitSetup') : mode === 'signup' ? t('submitSignup') : t('submitLogin')}
            {!busy && <Icon name="arrow" size={16} className="flip-rtl" />}
          </button>

          {nabu && nabu.enabled && mode === 'login' && (
            <>
              <div className="auth-or">{t('or')}</div>
              <div className="auth-sso">
                <a href="/api/nabu" className="btn btn-outline"><Logo size={18} />{t('nabu')}</a>
                <a href="/api/nabu?provider=google" className="btn btn-outline">{GOOGLE}{t('google')}</a>
              </div>
            </>
          )}

        </form>
      </main>
    </div>
  );
}
