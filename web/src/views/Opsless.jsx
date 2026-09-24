import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import { useT } from '../i18n/index.jsx';

const T = {
  fa: {
    title: 'اپس‌لس (Zero-UI)',
    subtitle: 'تنظیمات سیستم‌های خودکار و بدون رابط کاربری',
    selfHealing: 'سیستم خودکار NabuGate (Self-Healing)',
    intro: 'در این بخش وضعیت اطلاع‌رسانی خودکار و سوییچ مدل‌ها از طریق Telegram مدیریت می‌شود.',
    botToken: 'ربات تلگرام (Telegram Bot Token)',
    botTokenHint: (v) => <>از طریق متغیرهای محیطی <code>{v.env}</code> خوانده می‌شود.</>,
    chatId: 'شناسه مدیر (Admin Chat ID)',
    chatIdHint: (v) => <>از طریق متغیر محیطی <code>{v.env}</code> ست می‌شود.</>,
    features: 'قابلیت‌های فعال Opsless',
    f1Name: 'نابوگیت Self-Healing:',
    f1: 'تشخیص قطعی سرویس‌دهنده و جایگزینی آنی (ثبت رویداد در تلگرام).',
    f2Name: 'پیگیری مالی (Invoice Chaser):',
    f2: 'اطلاع‌رسانی سررسید فاکتورها به مدیر فروش.',
    f3Name: 'نابو ویس (NabuVoice):',
    f3: 'در حال توسعه... (بزودی)',
  },
  en: {
    title: 'Opsless (Zero-UI)',
    subtitle: 'Settings for the automated, UI-less systems',
    selfHealing: 'NabuGate automation (self-healing)',
    intro: 'This section manages automatic alerts and model switching via Telegram.',
    botToken: 'Telegram bot token',
    botTokenHint: (v) => <>Read from the <code>{v.env}</code> environment variable.</>,
    chatId: 'Admin chat ID',
    chatIdHint: (v) => <>Set via the <code>{v.env}</code> environment variable.</>,
    features: 'Active Opsless features',
    f1Name: 'NabuGate self-healing:',
    f1: 'Detects a provider outage and switches over instantly (event logged to Telegram).',
    f2Name: 'Invoice Chaser:',
    f2: 'Notifies the sales manager when invoices fall due.',
    f3Name: 'NabuVoice:',
    f3: 'In development… (coming soon)',
  },
};

export default function Opsless() {
  const t = useT(T);
  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      <div className="card">
        <h3>{t('selfHealing')}</h3>
        <p style={{ color: 'var(--ng-muted)', marginBottom: 16 }}>{t('intro')}</p>
        
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <label>
            <div className="label">{t('botToken')}</div>
            <input
              className="input"
              disabled 
              type="password" 
              value="********" 
              dir="ltr" 
              style={{ width: '100%', maxWidth: 400 }} 
            />
            <div style={{ fontSize: 11, color: 'var(--ng-muted)', marginTop: 4 }}>
              {t('botTokenHint', { env: 'OPSLESS_TELEGRAM_BOT_TOKEN' })}
            </div>
          </label>
          
          <label>
            <div className="label">{t('chatId')}</div>
            <input
              className="input"
              disabled 
              value="********" 
              dir="ltr" 
              style={{ width: '100%', maxWidth: 400 }} 
            />
            <div style={{ fontSize: 11, color: 'var(--ng-muted)', marginTop: 4 }}>
              {t('chatIdHint', { env: 'OPSLESS_ADMIN_CHAT_ID' })}
            </div>
          </label>
        </div>
      </div>
      
      <div className="card" style={{ marginTop: 24 }}>
        <h3>{t('features')}</h3>
        <ul style={{ paddingInlineStart: 20, color: 'var(--ng-text)' }}>
          <li style={{ marginBottom: 8 }}><strong>{t('f1Name')}</strong> {t('f1')}</li>
          <li style={{ marginBottom: 8 }}><strong>{t('f2Name')}</strong> {t('f2')}</li>
          <li style={{ color: 'var(--ng-muted)' }}><strong>{t('f3Name')}</strong> {t('f3')}</li>
        </ul>
      </div>
    </Layout>
  );
}
