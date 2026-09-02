import { navigate } from '../nav.js';
import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { faInt, usd } from '../data/mock.js';
import { Skeleton, SkeletonStats, SkeletonTable } from '../components/Skeleton.jsx';
import EmptyState from '../components/EmptyState.jsx';

export default function Dashboard() {
  const [data, setData] = useState(null);
  const [tokens, setTokens] = useState(null);
  const [user, setUser] = useState(null);
  const [mine, setMine] = useState(null);
  const [status, setStatus] = useState(null);
  const [error, setError] = useState(null);
  const [showOnboarding, setShowOnboarding] = useState(false);
  const [onboardingStep, setOnboardingStep] = useState(1);

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
  const username = user?.name || user?.email?.split('@')[0] || 'کاربر';
  const paymentsEnabled = status?.payments_enabled;

  // Per-key spend, sorted, with a share so the bars mean something relative to
  // each other rather than to an arbitrary maximum.
  const projects = Object.entries(mine?.projects || {}).sort((a, b) => (b[1].cost_usd || 0) - (a[1].cost_usd || 0));
  const maxCost = projects.reduce((m, [, c]) => Math.max(m, c.cost_usd || 0), 0);
  const totalTokens = (mine?.prompt_tokens || 0) + (mine?.completion_tokens || 0);

  const steps = [
    { done: (tokens || []).length > 0, title: 'یک کلید API بسازید', hint: 'در «کلیدهای API». متن کامل کلید فقط یک‌بار نمایش داده می‌شود.', go: 'tokens' },
    { done: balance > 0, title: 'حساب را شارژ کنید', hint: 'پرداخت از درگاه بانکی؛ موجودی بعد از تأیید درگاه اضافه می‌شود.', go: 'plans' },
    { done: (mine?.requests || 0) > 0, title: 'اولین درخواست را بفرستید', hint: 'آدرس پایه و کلید را در ابزارتان بگذارید؛ نمونه‌ها در «اتصال به دروازه».', go: 'integration' },
  ];
  const remaining = steps.filter((s) => !s.done).length;

  return (
    <Layout title="داشبورد" subtitle="نمای کلی حساب، موجودی و مصرف شما">
      {error && <div className="card banner-error">{error}</div>}

      <div className="row-between" style={{ marginBottom: 6 }}>
        <div>
          <h2 style={{ fontSize: 24, fontWeight: 800, margin: 0, color: 'var(--ng-heading)' }}>
            {loading ? <Skeleton w={220} h={26} /> : <>سلام، {username} 👋</>}
          </h2>
          <p style={{ color: 'var(--ng-muted)', fontSize: 13, marginTop: 6 }}>
            {loading ? <Skeleton w={260} h={12} /> : (remaining === 0 ? 'همه‌چیز آماده است. مصرف امروزتان را پایین ببینید.' : `${faInt(remaining)} گام تا اولین درخواست باقی مانده.`)}
          </p>
        </div>
        <div style={{ display: 'flex', gap: 10 }}>
          <button className="btn btn-outline" onClick={() => navigate('plans')}>+ شارژ حساب</button>
          <button className="btn btn-primary" onClick={() => navigate('tokens')}>🔑 ساخت کلید</button>
        </div>
      </div>

      {loading ? (
        <SkeletonStats n={4} />
      ) : (
        <div className="grid-auto stagger">
          <div className="card kpi card-hover" style={{ background: 'linear-gradient(135deg, var(--ng-accent-soft), var(--ng-surface) 60%)' }}>
            <div className="kpi-label"><span className="kpi-icon">💳</span> موجودی حساب</div>
            <div className="kpi-value ltr">{usd(balance)}</div>
            <div className="kpi-sub">
              {balance <= 0
                ? 'با موجودی صفر، کلیدها با خطای ۴۰۲ رد می‌شوند.'
                : balance < 1
                  ? 'موجودی کم است؛ پیش از توقف، شارژ کنید.'
                  : 'با هر درخواست به اندازهٔ مصرف کم می‌شود.'}
            </div>
          </div>
          <div className="card kpi card-hover">
            <div className="kpi-label"><span className="kpi-icon pass">◴</span> توکن مصرف‌شده</div>
            <div className="kpi-value">{faInt(totalTokens)}</div>
            <div className="kpi-sub">ورودی {faInt(mine?.prompt_tokens)} · خروجی {faInt(mine?.completion_tokens)}</div>
          </div>
          <div className="card kpi card-hover">
            <div className="kpi-label"><span className="kpi-icon ok">➤</span> درخواست‌ها</div>
            <div className="kpi-value">{faInt(mine?.requests)}</div>
            <div className="kpi-sub">{mine?.denied ? `${faInt(mine.denied)} درخواست رد شده` : 'بدون درخواست ردشده'}</div>
          </div>
          <div className="card kpi card-hover">
            <div className="kpi-label"><span className="kpi-icon warn">🔑</span> کلیدهای فعال</div>
            <div className="kpi-value">{faInt((tokens || []).filter((t) => !t.disabled).length)}</div>
            <div className="kpi-sub">هزینهٔ کل {usd(mine?.cost_usd)}</div>
          </div>
        </div>
      )}

      <div className="grid grid-2-wide" style={{ marginTop: 4 }}>
        <div className="card">
          <div className="card-head">
            <h3>مصرف به تفکیک کلید</h3>
            <button className="btn btn-ghost" onClick={() => navigate('account')}>جزئیات</button>
          </div>
          {loading ? (
            <SkeletonTable rows={4} cols={3} />
          ) : projects.length === 0 ? (
            <EmptyState
              icon="◴"
              title="هنوز مصرفی ثبت نشده"
              hint="بعد از اولین درخواست با کلیدتان، سهم هر کلید اینجا با نمودار نمایش داده می‌شود."
              action={<button className="btn btn-outline btn-sm" onClick={() => navigate('integration')}>نمونهٔ اتصال</button>}
            />
          ) : (
            <div className="stagger" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
              {projects.slice(0, 6).map(([name, c]) => (
                <div key={name}>
                  <div className="row-between" style={{ marginBottom: 6 }}>
                    <span className="mono" dir="ltr" style={{ fontSize: 12.5, fontWeight: 700, color: 'var(--ng-heading)' }}>{name}</span>
                    <span style={{ fontSize: 12, color: 'var(--ng-muted)' }}>
                      {faInt(c.requests)} درخواست · <span className="ltr">{usd(c.cost_usd)}</span>
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
            <div className="card-head"><h3>شروع سریع</h3><span className="badge badge-info">{faInt(steps.length - remaining)}/{faInt(steps.length)}</span></div>
            {loading ? (
              <div className="sk-stack"><Skeleton h={48} /><Skeleton h={48} /><Skeleton h={48} /></div>
            ) : (
              <div className="checklist stagger">
                {steps.map((s, i) => (
                  <button key={i} type="button" className={'check-item' + (s.done ? ' done' : '')} onClick={() => navigate(s.go)} style={{ textAlign: 'start', cursor: 'pointer', font: 'inherit' }}>
                    <span className="mark">{s.done ? '✓' : faInt(i + 1)}</span>
                    <span className="txt"><strong>{s.title}</strong><span>{s.hint}</span></span>
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="card">
            <div className="card-head"><h3>وضعیت حساب</h3></div>
            <div className="kv">
              <div className="line"><span>ایمیل</span><strong dir="ltr">{loading ? <Skeleton w={140} h={12} /> : (user?.email || '—')}</strong></div>
              <div className="line"><span>درگاه پرداخت</span>
                <strong className={paymentsEnabled === false ? 'warn' : ''}>
                  {status === null ? <Skeleton w={60} h={12} /> : paymentsEnabled ? 'متصل' : 'در این استقرار فعال نیست'}
                </strong>
              </div>
              <div className="line"><span>آدرس پایه</span><strong dir="ltr" className="mono" style={{ fontSize: 11.5 }}>{window.location.origin}/v1</strong></div>
            </div>
            <div style={{ display: 'flex', gap: 8, marginTop: 14 }}>
              <button className="btn btn-outline btn-sm" onClick={() => navigate('profile')}>پروفایل</button>
              <button className="btn btn-outline btn-sm" onClick={() => navigate('docs')}>مستندات</button>
            </div>
          </div>
        </div>
      </div>

      {showOnboarding && (
        <div className="modal-backdrop" style={{ backdropFilter: 'blur(8px)', background: 'rgba(2,6,23,0.7)' }}>
          <div className="card fade-in" style={{ width: 500, maxWidth: '95%', padding: 0, overflow: 'hidden', position: 'relative' }}>
            <div style={{ height: 5, background: 'var(--ng-accent-grad)' }} />
            <div style={{ padding: '32px 32px 16px' }}>
              {onboardingStep === 1 && (
                <div className="fade-in" key="s1">
                  <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 20, fontSize: 44 }}>👋</div>
                  <h2 style={{ fontSize: 22, fontWeight: 800, textAlign: 'center', marginBottom: 10 }}>به کنسول NabuGate خوش آمدید</h2>
                  <p style={{ color: 'var(--ng-muted)', lineHeight: 1.9, textAlign: 'center', fontSize: 14 }}>
                    یک آدرس، یک کلید، دسترسی به مدل‌های OpenAI، Anthropic، Gemini و بقیه. هر ابزاری که با OpenAI کار می‌کند، با NabuGate هم کار می‌کند.
                  </p>
                </div>
              )}
              {onboardingStep === 2 && (
                <div className="fade-in" key="s2">
                  <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 20, fontSize: 44 }}>🔑</div>
                  <h2 style={{ fontSize: 22, fontWeight: 800, textAlign: 'center', marginBottom: 10 }}>کلید API بسازید</h2>
                  <p style={{ color: 'var(--ng-muted)', lineHeight: 1.9, textAlign: 'center', fontSize: 14 }}>
                    از بخش <strong>«کلیدهای API»</strong> یک کلید بسازید و در Cursor، Cline، Claude Code یا SDK وارد کنید. متن کلید فقط یک‌بار نمایش داده می‌شود.
                  </p>
                </div>
              )}
              {onboardingStep === 3 && (
                <div className="fade-in" key="s3">
                  <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 20, fontSize: 44 }}>💳</div>
                  <h2 style={{ fontSize: 22, fontWeight: 800, textAlign: 'center', marginBottom: 10 }}>پرداخت به‌ازای مصرف</h2>
                  <p style={{ color: 'var(--ng-muted)', lineHeight: 1.9, textAlign: 'center', fontSize: 14 }}>
                    فقط به اندازهٔ توکنی که مصرف می‌کنید از موجودی کم می‌شود. موجودی منقضی نمی‌شود و شارژ از درگاه بانکی انجام می‌شود؛ اطلاعات کارت هرگز به این پنل نمی‌رسد.
                  </p>
                </div>
              )}
            </div>
            <div className="row-between" style={{ padding: '12px 32px 28px' }}>
              <div style={{ display: 'flex', gap: 6 }}>
                {[1, 2, 3].map((step) => (
                  <div key={step} style={{ width: step === onboardingStep ? 18 : 8, height: 8, borderRadius: 999, background: step === onboardingStep ? 'var(--ng-accent)' : 'var(--ng-border-card)', transition: 'width 240ms var(--ng-ease-out)' }} />
                ))}
              </div>
              <div style={{ display: 'flex', gap: 10 }}>
                <button onClick={closeOnboarding} className="btn" style={{ background: 'transparent', color: 'var(--ng-muted)' }}>رد کردن</button>
                {onboardingStep < 3 ? (
                  <button onClick={() => setOnboardingStep((s) => s + 1)} className="btn btn-primary">مرحلهٔ بعد</button>
                ) : (
                  <button onClick={closeOnboarding} className="btn btn-primary">شروع کنید</button>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </Layout>
  );
}
