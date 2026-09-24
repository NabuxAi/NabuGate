import { navigate } from "../nav.js";

import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { usd, fmtDate, useT } from '../i18n/index.jsx';
import { SkeletonTable, Skeleton } from '../components/Skeleton.jsx';
import Icon from '../components/Icon.jsx';

const T = {
  fa: {
    title: 'پروفایل کاربری',
    subtitle: 'مدیریت اطلاعات حساب و تاریخچه تراکنش‌ها',
    accountInfo: 'اطلاعات حساب',
    emailLabel: 'ایمیل (شناسه ورود)',
    emailHint: 'این ایمیل برای ورود و ارتباطات استفاده می‌شود.',
    balance: 'موجودی حساب',
    topUp: 'شارژ حساب',
    password: 'رمز عبور و امنیت',
    passwordNote: (v) => <>
      رمز عبور حساب را در بخش <button type="button" className="linklike" onClick={v.onSecurity}>امنیت</button> تغییر دهید.
      اگر با حساب نابو (SSO) وارد شده‌اید، رمزتان در NabuAuth مدیریت می‌شود و اینجا رمزی برای تغییر وجود ندارد.
    </>,
    history: 'تاریخچه پرداخت‌ها',
    noTx: 'تراکنشی یافت نشد',
    noTxHint: 'شما هنوز هیچ پرداختی انجام نداده‌اید.',
    date: 'تاریخ',
    amount: 'مبلغ',
    trackingCode: 'کد پیگیری',
    status: 'وضعیت',
    success: 'موفق',
    pending: 'در انتظار',
    failed: 'ناموفق',
  },
  en: {
    title: 'Profile',
    subtitle: 'Manage your account details and transaction history',
    accountInfo: 'Account details',
    emailLabel: 'Email (sign-in ID)',
    emailHint: 'Used for signing in and for account emails.',
    balance: 'Balance',
    topUp: 'Top up',
    password: 'Password & security',
    passwordNote: (v) => <>
      Change your account password under <button type="button" className="linklike" onClick={v.onSecurity}>Security</button>.
      If you signed in with a Nabu account (SSO), your password is managed in NabuAuth and there is nothing to change here.
    </>,
    history: 'Payment history',
    noTx: 'No transactions yet',
    noTxHint: 'You haven’t made any payments yet.',
    date: 'Date',
    amount: 'Amount',
    trackingCode: 'Reference',
    status: 'Status',
    success: 'Successful',
    pending: 'Pending',
    failed: 'Failed',
  },
};

export default function Profile() {
  const t = useT(T);
  const [user, setUser] = useState(null);
  const [error, setError] = useState(null);

  const load = () => {
    api.getMe().then(setUser).catch((e) => setError(e.message));
  };

  useEffect(load, []);

  const balance = user ? user.balance || 0 : 0;
  const payments = (user && user.payments) ? [...user.payments].reverse() : [];

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {error && <div className="card banner-error">{error}</div>}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: 24, paddingBottom: 32 }}>
        
        {/* Account Info */}
        <div className="card">
          <h3 style={{ fontSize: 16, marginBottom: 20, paddingBottom: 12, borderBottom: '1px solid var(--ng-border)' }}>{t('accountInfo')}</h3>
          
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div>
              <label style={{ display: 'block', fontSize: 13, color: 'var(--ng-muted)', marginBottom: 6 }}>{t('emailLabel')}</label>
              <input type="text" className="input" value={user?.email || ''} readOnly style={{ width: '100%', opacity: 0.7 }} dir="ltr" />
              <p style={{ fontSize: 12, color: 'var(--ng-muted)', marginTop: 4 }}>{t('emailHint')}</p>
            </div>
            
            <div>
              <label style={{ display: 'block', fontSize: 13, color: 'var(--ng-muted)', marginBottom: 6 }}>{t('balance')}</label>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <input type="text" className="input" value={usd(balance)} readOnly style={{ width: 150, fontWeight: 700 }} dir="ltr" />
                <button className="btn btn-primary" style={{ marginInlineStart: 'auto' }} onClick={() => navigate('plans')}>
                  {t('topUp')}
                </button>
              </div>
            </div>
            
            <div>
              <label style={{ display: 'block', fontSize: 13, color: 'var(--ng-muted)', marginBottom: 6 }}>{t('password')}</label>
              {/* This used to state flatly that the visitor had signed in
                  through NabuAuth and had to leave the panel to change a
                  password. Accounts created with the local sign-up form have a
                  password stored right here, and were being sent away to change
                  something this panel owns. */}
              <div style={{ fontSize: 13, padding: '12px 16px', background: 'var(--ng-surface-soft)', borderRadius: 8, border: '1px solid var(--ng-border)', lineHeight: 1.6 }}>
                {t('passwordNote', { onSecurity: () => navigate('security') })}
              </div>
            </div>
          </div>
        </div>

        {/* Transaction History */}
        <div className="card" style={{ display: 'flex', flexDirection: 'column' }}>
          <h3 style={{ fontSize: 16, marginBottom: 20, paddingBottom: 12, borderBottom: '1px solid var(--ng-border)' }}>{t('history')}</h3>
          
          <div style={{ flex: 1 }}>
            {user === null ? (
              <SkeletonTable rows={4} cols={4} />
            ) : payments.length === 0 ? (
              <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--ng-muted)' }}>
                <div style={{ fontSize: 32, marginBottom: 12, opacity: 0.5 }}><Icon name="receipt" size={32} /></div>
                <div style={{ fontSize: 14, fontWeight: 700, marginBottom: 8, color: 'var(--ng-heading)' }}>{t('noTx')}</div>
                <div style={{ fontSize: 13 }}>{t('noTxHint')}</div>
              </div>
            ) : (
              <table className="tbl" style={{ border: 'none', margin: 0 }}>
                <thead>
                  <tr>
                    <th>{t('date')}</th>
                    <th>{t('amount')}</th>
                    <th>{t('trackingCode')}</th>
                    <th>{t('status')}</th>
                  </tr>
                </thead>
                <tbody>
                  {payments.map(p => (
                    <tr key={p.id}>
                      <td style={{ fontSize: 13, color: 'var(--ng-muted)' }} dir="ltr">
                        {fmtDate(p.created_at)}
                      </td>
                      <td style={{ fontWeight: 700 }} dir="ltr">
                        {usd(p.amount)}
                      </td>
                      <td className="mono" style={{ fontSize: 12 }}>
                        {p.id.split('_')[1] || p.id}
                      </td>
                      <td>
                        {/* Read from the record. This was hard-coded to
                            "موفق", so a failed payment was reported as a
                            successful one. */}
                        <span className={'badge ' + (p.status === 'success' ? 'badge-ok' : p.status === 'pending' ? 'badge-warn' : 'badge-fail')} style={{ fontSize: 11 }}>
                          {p.status === 'success' ? t('success') : p.status === 'pending' ? t('pending') : t('failed')}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>

      </div>
    </Layout>
  );
}
