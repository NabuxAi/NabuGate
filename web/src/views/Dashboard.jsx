import { navigate } from '../nav.js';
import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { fmtInt, usd, useT } from '../i18n/index.jsx';
import Icon from '../components/Icon.jsx';
import { Skeleton, SkeletonStats, SkeletonTable } from '../components/Skeleton.jsx';
import EmptyState from '../components/EmptyState.jsx';

const T = {
  fa: {
    title: 'داشبورد',
    subtitle: 'نمای کلی حساب، موجودی و مصرف شما',
    user: 'کاربر',
    hello: 'سلام، {name} 👋',
    allReady: 'همه‌چیز آماده است. مصرف امروزتان را پایین ببینید.',
    stepsLeft: '{n} گام تا اولین درخواست باقی مانده.',
    progress: '{done} از {total} گام',
    topUp: 'شارژ حساب',
    newKey: 'ساخت کلید',
    balance: 'موجودی حساب',
    balanceZero: 'با موجودی صفر، کلیدها با خطای ۴۰۲ رد می‌شوند.',
    balanceLow: 'موجودی کم است؛ پیش از توقف، شارژ کنید.',
    balanceOk: 'با هر درخواست به اندازهٔ مصرف کم می‌شود.',
    tokens: 'توکن مصرف‌شده',
    tokensSub: 'ورودی {in} · خروجی {out}',
    requests: 'درخواست‌ها',
    denied: '{n} درخواست رد شده',
    noDenied: 'بدون درخواست ردشده',
    activeKeys: 'کلیدهای فعال',
    totalCost: 'هزینهٔ کل {cost}',
    byKey: 'مصرف به تفکیک کلید',
    details: 'جزئیات',
    emptyTitle: 'هنوز مصرفی ثبت نشده',
    emptyHint: 'بعد از اولین درخواست با کلیدتان، سهم هر کلید اینجا با نمودار نمایش داده می‌شود.',
    sample: 'نمونهٔ اتصال',
    reqCount: '{n} درخواست',
    quickStart: 'شروع سریع',
    s1: 'یک کلید API بسازید',
    s1h: 'در «کلیدهای API». متن کامل کلید فقط یک‌بار نمایش داده می‌شود.',
    s2: 'حساب را شارژ کنید',
    s2h: 'پرداخت از درگاه بانکی؛ موجودی بعد از تأیید درگاه اضافه می‌شود.',
    s3: 'اولین درخواست را بفرستید',
    s3h: 'آدرس پایه و کلید را در ابزارتان بگذارید؛ نمونه‌ها در «اتصال به دروازه».',
    account: 'وضعیت حساب',
    email: 'ایمیل',
    gateway: 'درگاه پرداخت',
    connected: 'متصل',
    notEnabled: 'در این استقرار فعال نیست',
    baseUrl: 'آدرس پایه',
    profile: 'پروفایل',
    docs: 'مستندات',
    ob1t: 'به کنسول NabuGate خوش آمدید',
    ob1b: 'یک آدرس، یک کلید، دسترسی به مدل‌های OpenAI، Anthropic، Gemini و بقیه. هر ابزاری که با OpenAI کار می‌کند، با NabuGate هم کار می‌کند.',
    ob2t: 'کلید API بسازید',
    ob2b: 'از بخش «کلیدهای API» یک کلید بسازید و در Cursor، Cline، Claude Code یا SDK وارد کنید. متن کلید فقط یک‌بار نمایش داده می‌شود.',
    ob3t: 'پرداخت به‌ازای مصرف',
    ob3b: 'فقط به اندازهٔ توکنی که مصرف می‌کنید از موجودی کم می‌شود. موجودی منقضی نمی‌شود و شارژ از درگاه بانکی انجام می‌شود؛ اطلاعات کارت هرگز به این پنل نمی‌رسد.',
    skip: 'رد کردن',
    next: 'مرحلهٔ بعد',
    start: 'شروع کنید',
  },
  en: {
    title: 'Dashboard',
    subtitle: 'Your account, balance and usage at a glance',
    user: 'there',
    hello: 'Hi, {name} 👋',
    allReady: 'You are all set. Today’s usage is below.',
    stepsLeft: '{n} step(s) left before your first request.',
    progress: '{done} of {total} steps',
    topUp: 'Top up',
    newKey: 'Create key',
    balance: 'Balance',
    balanceZero: 'With a zero balance, keys are refused with 402.',
    balanceLow: 'Running low — top up before requests stop.',
    balanceOk: 'Each request deducts exactly what it used.',
    tokens: 'Tokens used',
    tokensSub: 'Input {in} · Output {out}',
    requests: 'Requests',
    denied: '{n} denied',
    noDenied: 'No denied requests',
    activeKeys: 'Active keys',
    totalCost: 'Total spend {cost}',
    byKey: 'Usage by key',
    details: 'Details',
    emptyTitle: 'No usage yet',
    emptyHint: 'After your first request, each key’s share of spend appears here.',
    sample: 'See an example',
    reqCount: '{n} requests',
    quickStart: 'Quick start',
    s1: 'Create an API key',
    s1h: 'Under “API keys”. The full key is shown only once.',
    s2: 'Top up your balance',
    s2h: 'Pay through the bank gateway; credit lands once the gateway confirms.',
    s3: 'Send your first request',
    s3h: 'Put the base URL and key in your tool; examples under “Connect”.',
    account: 'Account status',
    email: 'Email',
    gateway: 'Payment gateway',
    connected: 'Connected',
    notEnabled: 'Not enabled on this deployment',
    baseUrl: 'Base URL',
    profile: 'Profile',
    docs: 'Docs',
    ob1t: 'Welcome to the NabuGate console',
    ob1b: 'One URL, one key, models from OpenAI, Anthropic, Gemini and more. Anything that works with OpenAI works with NabuGate.',
    ob2t: 'Create an API key',
    ob2b: 'Create a key under “API keys” and paste it into Cursor, Cline, Claude Code or your SDK. The key text is shown only once.',
    ob3t: 'Pay only for what you use',
    ob3b: 'Your balance goes down by exactly the tokens you use. Credit never expires, top-ups go through the bank gateway, and card details never reach this console.',
    skip: 'Skip',
    next: 'Next',
    start: 'Get started',
  },
};

const ONBOARDING = [
  { icon: 'sparkles', title: 'ob1t', body: 'ob1b' },
  { icon: 'key', title: 'ob2t', body: 'ob2b' },
  { icon: 'wallet', title: 'ob3t', body: 'ob3b' },
];

export default function Dashboard() {
  const t = useT(T);
  const [data, setData] = useState(null);
  const [tokens, setTokens] = useState(null);
  const [user, setUser] = useState(null);
  const [mine, setMine] = useState(null);
  const [status, setStatus] = useState(null);
  const [error, setError] = useState(null);
  const [showOnboarding, setShowOnboarding] = useState(false);
  const [onboardingStep, setOnboardingStep] = useState(0);

  useEffect(() => {
    api.overview().then(setData).catch((e) => setError(e.message));
    api.listTokens().then((r) => setTokens(r.tokens || [])).catch(() => setTokens([]));
    api.getMe().then(setUser).catch(() => setUser({}));
    api.myUsage().then(setMine).catch(() => setMine({}));
    api.status().then(setStatus).catch(() => setStatus({}));
    if (!localStorage.getItem('nabugate_onboarding_done')) setShowOnboarding(true);
  }, []);

  const closeOnboarding = () => {
    localStorage.setItem('nabugate_onboarding_done', 'true');
    setShowOnboarding(false);
  };

  const loading = user === null || tokens === null || mine === null;
  const balance = user?.balance || 0;
  const username = user?.name || user?.email?.split('@')[0] || t('user');
  const paymentsEnabled = status?.payments_enabled;

  // Per-key spend, sorted, with a share so the bars mean something relative to
  // each other rather than to an arbitrary maximum.
  const projects = Object.entries(mine?.projects || {}).sort((a, b) => (b[1].cost_usd || 0) - (a[1].cost_usd || 0));
  const maxCost = projects.reduce((m, [, c]) => Math.max(m, c.cost_usd || 0), 0);
  const totalTokens = (mine?.prompt_tokens || 0) + (mine?.completion_tokens || 0);

  const steps = [
    { done: (tokens || []).length > 0, title: t('s1'), hint: t('s1h'), go: 'tokens' },
    { done: balance > 0, title: t('s2'), hint: t('s2h'), go: 'plans' },
    { done: (mine?.requests || 0) > 0, title: t('s3'), hint: t('s3h'), go: 'integration' },
  ];
  const remaining = steps.filter((s) => !s.done).length;
  const done = steps.length - remaining;
  const ob = ONBOARDING[onboardingStep];

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {error && <div className="card banner-error">{error}</div>}

      <section className="welcome">
        <div>
          <h1>{loading ? <Skeleton w={220} h={26} /> : t('hello', { name: username })}</h1>
          <p>{loading ? <Skeleton w={260} h={12} /> : (remaining === 0 ? t('allReady') : t('stepsLeft', { n: fmtInt(remaining) }))}</p>
          {!loading && (
            <div className="progress-ring">
              <span className="track"><i style={{ width: `${(done / steps.length) * 100}%` }} /></span>
              {t('progress', { done: fmtInt(done), total: fmtInt(steps.length) })}
            </div>
          )}
        </div>
        <div className="welcome-actions">
          <button className="btn btn-outline" onClick={() => navigate('plans')}><Icon name="plus" size={16} />{t('topUp')}</button>
          <button className="btn btn-primary" onClick={() => navigate('tokens')}><Icon name="key" size={16} />{t('newKey')}</button>
        </div>
      </section>

      {loading ? (
        <SkeletonStats n={4} />
      ) : (
        <div className="grid-auto stagger">
          <div className="card kpi card-hover kpi-hero">
            <div className="kpi-label"><span className="kpi-icon"><Icon name="wallet" size={18} /></span>{t('balance')}</div>
            <div className="kpi-value ltr">{usd(balance)}</div>
            <div className="kpi-sub">
              {balance <= 0 ? t('balanceZero') : balance < 1 ? t('balanceLow') : t('balanceOk')}
            </div>
          </div>
          <div className="card kpi card-hover">
            <div className="kpi-label"><span className="kpi-icon pass"><Icon name="layers" size={18} /></span>{t('tokens')}</div>
            <div className="kpi-value">{fmtInt(totalTokens)}</div>
            <div className="kpi-sub">{t('tokensSub', { in: fmtInt(mine?.prompt_tokens), out: fmtInt(mine?.completion_tokens) })}</div>
          </div>
          <div className="card kpi card-hover">
            <div className="kpi-label"><span className="kpi-icon ok"><Icon name="activity" size={18} /></span>{t('requests')}</div>
            <div className="kpi-value">{fmtInt(mine?.requests)}</div>
            <div className="kpi-sub">{mine?.denied ? t('denied', { n: fmtInt(mine.denied) }) : t('noDenied')}</div>
          </div>
          <div className="card kpi card-hover">
            <div className="kpi-label"><span className="kpi-icon warn"><Icon name="key" size={18} /></span>{t('activeKeys')}</div>
            <div className="kpi-value">{fmtInt((tokens || []).filter((k) => !k.disabled).length)}</div>
            <div className="kpi-sub">{t('totalCost', { cost: usd(mine?.cost_usd) })}</div>
          </div>
        </div>
      )}

      <div className="grid grid-2-wide">
        <div className="card">
          <div className="card-head">
            <h3><Icon name="chart" size={17} />{t('byKey')}</h3>
            <button className="btn btn-ghost" onClick={() => navigate('account')}>{t('details')}</button>
          </div>
          {loading ? (
            <SkeletonTable rows={4} cols={3} />
          ) : projects.length === 0 ? (
            <EmptyState
              icon={<Icon name="chart" size={22} />}
              title={t('emptyTitle')}
              hint={t('emptyHint')}
              action={<button className="btn btn-outline btn-sm" onClick={() => navigate('integration')}>{t('sample')}</button>}
            />
          ) : (
            <div className="stagger" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              {projects.slice(0, 6).map(([name, c]) => (
                <div key={name}>
                  <div className="row-between" style={{ marginBottom: 7 }}>
                    <span className="mono" dir="ltr" style={{ fontSize: 12.5, fontWeight: 700, color: 'var(--ng-heading)' }}>{name}</span>
                    <span style={{ fontSize: 12, color: 'var(--ng-muted)' }}>
                      {t('reqCount', { n: fmtInt(c.requests) })} · <span className="ltr">{usd(c.cost_usd)}</span>
                    </span>
                  </div>
                  <div className="bar"><i style={{ width: `${maxCost > 0 ? Math.max(4, ((c.cost_usd || 0) / maxCost) * 100) : 4}%` }} /></div>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="col">
          <div className="card">
            <div className="card-head"><h3><Icon name="zap" size={17} />{t('quickStart')}</h3><span className="badge badge-info">{fmtInt(done)}/{fmtInt(steps.length)}</span></div>
            {loading ? (
              <div className="sk-stack"><Skeleton h={48} /><Skeleton h={48} /><Skeleton h={48} /></div>
            ) : (
              <div className="checklist stagger">
                {steps.map((s, i) => (
                  <button key={i} type="button" className={'check-item' + (s.done ? ' done' : '')} onClick={() => navigate(s.go)} style={{ textAlign: 'start', cursor: 'pointer', font: 'inherit' }}>
                    <span className="mark">{s.done ? <Icon name="check" size={14} stroke={2.6} /> : fmtInt(i + 1)}</span>
                    <span className="txt"><strong>{s.title}</strong><span>{s.hint}</span></span>
                    <Icon name="chevron" size={16} className="chev" />
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="card">
            <div className="card-head"><h3><Icon name="user" size={17} />{t('account')}</h3></div>
            <div className="kv">
              <div className="line"><span>{t('email')}</span><strong dir="ltr">{loading ? <Skeleton w={140} h={12} /> : (user?.email || '—')}</strong></div>
              <div className="line"><span>{t('gateway')}</span>
                <strong className={paymentsEnabled === false ? 'warn' : ''}>
                  {status === null ? <Skeleton w={60} h={12} /> : paymentsEnabled ? t('connected') : t('notEnabled')}
                </strong>
              </div>
              <div className="line"><span>{t('baseUrl')}</span><strong dir="ltr" className="mono" style={{ fontSize: 11.5 }}>{window.location.origin}/v1</strong></div>
            </div>
            <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
              <button className="btn btn-outline btn-sm" onClick={() => navigate('profile')}><Icon name="user" size={14} />{t('profile')}</button>
              <button className="btn btn-outline btn-sm" onClick={() => navigate('docs')}><Icon name="book" size={14} />{t('docs')}</button>
            </div>
          </div>
        </div>
      </div>

      {showOnboarding && (
        <div className="modal-backdrop" style={{ backdropFilter: 'blur(8px)' }}>
          <div className="card fade-in onboard" role="dialog" aria-modal="true" aria-labelledby="ob-title">
            <div className="onboard-bar" />
            <div className="onboard-body fade-in" key={onboardingStep}>
              <div className="onboard-icon"><Icon name={ob.icon} size={30} /></div>
              <h2 id="ob-title">{t(ob.title)}</h2>
              <p>{t(ob.body)}</p>
            </div>
            <div className="row-between onboard-foot">
              <div style={{ display: 'flex', gap: 6 }}>
                {ONBOARDING.map((_, step) => (
                  <div key={step} style={{ width: step === onboardingStep ? 20 : 8, height: 8, borderRadius: 999, background: step === onboardingStep ? 'var(--ng-accent)' : 'var(--ng-border-card)', transition: 'width 240ms var(--ng-ease-out)' }} />
                ))}
              </div>
              <div style={{ display: 'flex', gap: 10 }}>
                <button onClick={closeOnboarding} className="btn" style={{ background: 'transparent', color: 'var(--ng-muted)' }}>{t('skip')}</button>
                {onboardingStep < ONBOARDING.length - 1 ? (
                  <button onClick={() => setOnboardingStep((s) => s + 1)} className="btn btn-primary">{t('next')}<Icon name="arrow" size={15} className="flip-rtl" /></button>
                ) : (
                  <button onClick={closeOnboarding} className="btn btn-primary">{t('start')}</button>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </Layout>
  );
}
