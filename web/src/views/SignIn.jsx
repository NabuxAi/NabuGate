import { useState, useEffect } from 'react';
import * as api from '../api.js';
import { useT, fmtDigits } from '../i18n/index.jsx';
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
    adminTitle: 'ورود به مدیریت',
    adminLead: 'این بخش فقط برای مدیران دروازه است.',
    setupTitle: 'راه‌اندازی اولیه',
    setupNote: 'هنوز حسابی ساخته نشده. اولین حساب را همین‌جا بساز — بعد از آن این فرم بسته می‌شود و کسی نمی‌تواند از بیرون حساب اضافه کند.',
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
    or: 'یا',
    continueTitle: 'ورود یا ثبت‌نام',
    continueLead: 'ایمیل و رمزتان را بزنید؛ اگر حساب دارید وارد می‌شوید و اگر نه، همین‌جا ساخته می‌شود.',
    submitContinue: 'ادامه',
    newAccount: 'با این ایمیل هنوز حسابی نیست. رمز را یک بار دیگر بزنید تا حساب ساخته شود.',
    submitCreate: 'ساخت حساب و ورود',
    otherEmail: 'ایمیل دیگری می‌زنم',
    wrongPassword: 'ایمیل یا رمز عبور درست نیست.',
    useSso: 'این حساب با حساب نابو یا گوگل ساخته شده؛ با همان دکمهٔ پایین وارد شوید.',
    shortPassword: 'رمز عبور باید دست‌کم ۱۰ نویسه باشد.',
    invalidEmail: 'یک ایمیل درست وارد کنید.',
    throttled: 'تلاش‌های ناموفق زیاد بود؛ چند دقیقه بعد دوباره امتحان کنید.',
    nabu: 'ادامه با حساب نابو',
    google: 'ادامه با حساب گوگل',
    emailTitle: 'ورود یا ثبت‌نام',
    emailLead: 'ایمیل‌تان را وارد کنید. اگر حساب دارید رمز را می‌پرسیم، و اگر نه، در قدم بعد حساب تازه می‌سازیم.',
    emailPlaceholder: 'you@example.com',
    next: 'ادامه',
    passwordTitle: 'خوش برگشتید',
    passwordLead: 'رمز عبور حساب‌تان را وارد کنید.',
    submitSignIn: 'ورود',
    registerTitle: 'ساخت حساب تازه',
    registerLead: 'با این ایمیل هنوز حسابی نیست. چند مشخصه بدهید تا حساب‌تان ساخته شود.',
    firstName: 'نام',
    lastName: 'نام خانوادگی',
    mobile: 'شماره موبایل',
    mobilePlaceholder: '۰۹۱۲ ۱۲۳ ۴۵۶۷',
    mobileHint: 'موبایل ایران را با ۰۹ بزنید؛ شمارهٔ خارج از ایران را با کد کشور.',
    submitRegister: 'ساخت حساب و ورود',
    ssoTitle: 'ورود با حساب نابو یا گوگل',
    ssoLead: 'این ایمیل با حساب نابو یا گوگل ساخته شده و رمز جداگانه ندارد. با یکی از دکمه‌های زیر وارد شوید.',
    change: 'تغییر',
    step: 'مرحلهٔ {n} از ۲',
    invalidName: 'نام و نام خانوادگی را وارد کنید.',
    invalidMobile: 'شمارهٔ موبایل درست نیست؛ مثلاً ۰۹۱۲۱۲۳۴۵۶۷.',
    accountExists: 'با این ایمیل از قبل حساب هست؛ رمز را بزنید و وارد شوید.',
    ssoFailedLong: 'ورود با حساب نابو یا گوگل کامل نشد. دوباره امتحان کنید، یا با ایمیل و رمز وارد شوید.',
  },
  en: {
    tagline: 'The central AI gateway',
    pitchTitle: 'One key. Every model.',
    pitchBody: 'Reach GPT, Claude, Gemini, DeepSeek and dozens more through one OpenAI-compatible API — with automatic fallback, a key per app and billing you can read.',
    p1: 'Works with Cursor, Claude Code, Codex and the OpenAI SDKs',
    p2: 'Smart routing and fallback across providers',
    p3: 'Pay as you go; credit never expires',
    home: 'Home',
    adminTitle: 'Administrator sign-in',
    adminLead: 'This area is for gateway administrators only.',
    setupTitle: 'First-time setup',
    setupNote: 'No account exists yet. Create the first one here — after that this form closes and nobody can add accounts from outside.',
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
    or: 'or',
    continueTitle: 'Sign in or sign up',
    continueLead: 'Enter your email and password: you are signed in if you have an account, and one is created here if you do not.',
    submitContinue: 'Continue',
    newAccount: 'There is no account with this email yet. Type the password once more to create it.',
    submitCreate: 'Create account and sign in',
    otherEmail: 'Use a different email',
    wrongPassword: 'The email or password is not right.',
    useSso: 'This account was made with Nabu or Google; sign in with that button below.',
    shortPassword: 'The password must be at least 10 characters.',
    invalidEmail: 'Enter a valid email address.',
    throttled: 'Too many failed attempts; try again in a few minutes.',
    nabu: 'Continue with Nabu',
    google: 'Continue with Google',
    emailTitle: 'Sign in or sign up',
    emailLead: 'Enter your email. If you have an account we ask for its password; if not, we create one in the next step.',
    emailPlaceholder: 'you@example.com',
    next: 'Continue',
    passwordTitle: 'Welcome back',
    passwordLead: 'Enter the password for your account.',
    submitSignIn: 'Sign in',
    registerTitle: 'Create your account',
    registerLead: 'There is no account with this email yet. A few details and it is ready.',
    firstName: 'First name',
    lastName: 'Last name',
    mobile: 'Mobile number',
    mobilePlaceholder: '+1 415 555 0100',
    mobileHint: 'An Iranian mobile starts with 09; any other number needs its country code (like +44).',
    submitRegister: 'Create account and sign in',
    ssoTitle: 'Sign in with Nabu or Google',
    ssoLead: 'This email was set up with a Nabu or Google account and has no separate password. Use one of the buttons below.',
    change: 'Change',
    step: 'Step {n} of 2',
    invalidName: 'Enter your first and last name.',
    invalidMobile: 'That mobile number does not look right.',
    accountExists: 'This email already has an account; enter its password to sign in.',
    ssoFailedLong: 'Signing in with Nabu or Google did not finish. Try again, or sign in with email and password.',
  },
};

const GOOGLE = (
  <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"><path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/><path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/><path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/><path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/></svg>
);

/*
 * The console's gate.
 *
 * The user panel signs in email-first: step one asks only for the address and
 * the server says what it can do — sign in with a password, register (first
 * and last name, mobile, password), or use the Nabu/Google button. The admin
 * area and first-time setup keep the username-and-password form.
 */
export default function SignIn({ needsSetup, onAuthenticated }) {
  const t = useT(T);
  const isPanel = window.location.pathname.startsWith('/panel');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [mobile, setMobile] = useState('');
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  // 'setup' creates the first administrator and 'continue' is the admin
  // area's sign-in. The panel walks 'email' → 'password' | 'register' | 'sso'.
  const [mode, setMode] = useState(needsSetup ? 'setup' : isPanel ? 'email' : 'continue');
  const [nabu, setNabu] = useState(null);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const errParam = params.get('nabu_error');
    if (errParam) {
      // Stored as a key rather than a sentence so a language switch
      // re-renders it in the new language.
      if (errParam === 'expired') setError({ key: 'expired' });
      else if (errParam === 'failed') setError({ key: 'ssoFailedLong' });
      else setError({ key: 'ssoError', vars: { code: errParam } });
      // Drop the flag from the address bar so a reload does not show it again.
      params.delete('nabu_error');
      const q = params.toString();
      window.history.replaceState(null, '', window.location.pathname + (q ? `?${q}` : '') + window.location.hash);
    }

    if (!needsSetup) {
      api.statusNabu().then(setNabu).catch(console.error);
    }
  }, [needsSetup]);

  const creating = mode === 'setup' || mode === 'register';
  const isAdmin = !isPanel && mode !== 'setup';
  const emailStep = mode === 'email';
  const panelStep2 = mode === 'password' || mode === 'register' || mode === 'sso';

  // The server's answers, by status and code, as sentences in the page's
  // language; a raw English message never reaches the form.
  function errorFor(err) {
    if (err.unauthenticated || err.status === 401) return { key: 'wrongPassword' };
    if (err.status === 429) return { key: 'throttled' };
    switch (err.code) {
      case 'use_sso': return { key: 'useSso' };
      case 'short_password': return { key: 'shortPassword' };
      case 'invalid_email': return { key: 'invalidEmail' };
      case 'invalid_name': return { key: 'invalidName' };
      case 'invalid_mobile': return { key: 'invalidMobile' };
      case 'account_exists': return { key: 'accountExists' };
      default: return { key: 'failed' };
    }
  }

  async function submit(e) {
    e.preventDefault();
    setError(null);

    if (creating && password !== confirm) {
      setError({ key: 'mismatch' });
      return;
    }
    if (creating && password.length < 10) {
      setError({ key: 'shortPassword' });
      return;
    }
    if (mode === 'register' && (!firstName.trim() || !lastName.trim())) {
      setError({ key: 'invalidName' });
      return;
    }
    setBusy(true);
    try {
      if (mode === 'setup') {
        await api.setup(username, password);
      } else if (mode === 'email') {
        const res = await api.checkEmail(username.trim());
        setPassword('');
        setConfirm('');
        setMode(res.state === 'none' ? 'register' : res.state === 'sso' ? 'sso' : 'password');
        return;
      } else if (mode === 'register') {
        await api.register({ email: username.trim(), firstName, lastName, mobile, password });
      } else if (mode === 'password') {
        await api.login(username.trim(), password);
      } else {
        const res = await api.continueAuth(username, password, false);
        if (res && res.new_account) {
          setError({ key: 'wrongPassword' });
          return;
        }
      }
      onAuthenticated();
    } catch (err) {
      const mapped = errorFor(err);
      // Someone registered this address meanwhile: offer the password step.
      if (mode === 'register' && err.code === 'account_exists') setMode('password');
      if (err.code === 'use_sso') setMode(isPanel ? 'sso' : mode);
      setError(mapped);
    } finally {
      setBusy(false);
    }
  }

  const backToEmail = () => { setMode('email'); setPassword(''); setConfirm(''); setError(null); };

  const title = mode === 'setup' ? t('setupTitle')
    : isAdmin ? t('adminTitle')
    : mode === 'password' ? t('passwordTitle')
    : mode === 'register' ? t('registerTitle')
    : mode === 'sso' ? t('ssoTitle')
    : t('emailTitle');
  const lead = isAdmin ? t('adminLead')
    : mode === 'password' ? t('passwordLead')
    : mode === 'register' ? t('registerLead')
    : mode === 'sso' ? t('ssoLead')
    : t('emailLead');
  const errorText = error && (error.text || t(error.key, error.vars));
  const submitLabel = mode === 'setup' ? t('submitSetup')
    : mode === 'email' ? t('next')
    : mode === 'password' ? t('submitSignIn')
    : mode === 'register' ? t('submitRegister')
    : t('submitContinue');
  const showSso = nabu && nabu.enabled && mode !== 'setup' && mode !== 'password' && mode !== 'register';

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

        <form className="auth-card fade-in" onSubmit={submit} key={mode}>
          {(emailStep || panelStep2) && (
            <p className="auth-step">{t('step', { n: fmtDigits(emailStep ? '1' : '2') })}</p>
          )}
          <h2>{title}</h2>
          {mode !== 'setup' && <p className="auth-lead">{lead}</p>}

          {mode === 'setup' && <p className="auth-note">{t('setupNote')}</p>}

          {panelStep2 ? (
            <div className="auth-email-chip">
              <Icon name="user" size={15} />
              <bdi dir="ltr">{username.trim()}</bdi>
              <button type="button" className="auth-link" onClick={backToEmail}>{t('change')}</button>
            </div>
          ) : (
            <label className="field">
              <span>{mode === 'setup' ? t('username') : isPanel ? t('email') : t('emailOrUser')}</span>
              <input
                className="input"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                // An administrator signs in with a username; only the user
                // panel is email-only.
                type={isPanel && mode !== 'setup' ? 'email' : 'text'}
                inputMode="email"
                autoComplete={isPanel ? 'email' : 'username'}
                placeholder={isPanel ? t('emailPlaceholder') : undefined}
                dir="ltr"
                autoFocus
                required
              />
            </label>
          )}

          {mode === 'register' && (
            <>
              <div className="field-row">
                <label className="field">
                  <span>{t('firstName')}</span>
                  <input className="input" value={firstName} onChange={(e) => setFirstName(e.target.value)} autoComplete="given-name" maxLength={60} autoFocus required />
                </label>
                <label className="field">
                  <span>{t('lastName')}</span>
                  <input className="input" value={lastName} onChange={(e) => setLastName(e.target.value)} autoComplete="family-name" maxLength={60} required />
                </label>
              </div>
              <label className="field">
                <span>{t('mobile')}</span>
                <input
                  className="input"
                  type="tel"
                  inputMode="tel"
                  value={mobile}
                  onChange={(e) => setMobile(e.target.value)}
                  autoComplete="tel"
                  placeholder={t('mobilePlaceholder')}
                  dir="ltr"
                  required
                />
                <span className="hint">{t('mobileHint')}</span>
              </label>
            </>
          )}

          {!emailStep && mode !== 'sso' && (
            <label className="field">
              <span>{t('password')}</span>
              <input
                className="input"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete={creating ? 'new-password' : 'current-password'}
                autoFocus={mode === 'password'}
                dir="ltr"
                required
              />
              {creating && <span className="hint">{t('pwHint')}</span>}
            </label>
          )}

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

          {mode !== 'sso' && (
            <button className="btn btn-primary auth-submit" disabled={busy}>
              {busy ? '…' : submitLabel}
              {!busy && <Icon name="arrow" size={16} className="flip-rtl" />}
            </button>
          )}

          {showSso && (
            <>
              {mode !== 'sso' && <div className="auth-or">{t('or')}</div>}
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
