import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';

export default function Opsless() {
  return (
    <Layout title="اپس‌لس (Zero-UI)" subtitle="تنظیمات سیستم‌های خودکار و بدون رابط کاربری">
      <div className="card">
        <h3>سیستم خودکار NabuGate (Self-Healing)</h3>
        <p style={{ color: 'var(--ng-muted)', marginBottom: 16 }}>
          در این بخش وضعیت اطلاع‌رسانی خودکار و سوییچ مدل‌ها از طریق Telegram مدیریت می‌شود.
        </p>
        
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <label>
            <div className="label">ربات تلگرام (Telegram Bot Token)</div>
            <input 
              disabled 
              type="password" 
              value="********" 
              dir="ltr" 
              style={{ width: '100%', maxWidth: 400 }} 
            />
            <div style={{ fontSize: 11, color: 'var(--ng-muted)', marginTop: 4 }}>
              از طریق متغیرهای محیطی <code>OPSLESS_TELEGRAM_BOT_TOKEN</code> خوانده می‌شود.
            </div>
          </label>
          
          <label>
            <div className="label">شناسه مدیر (Admin Chat ID)</div>
            <input 
              disabled 
              value="********" 
              dir="ltr" 
              style={{ width: '100%', maxWidth: 400 }} 
            />
            <div style={{ fontSize: 11, color: 'var(--ng-muted)', marginTop: 4 }}>
              از طریق متغیر محیطی <code>OPSLESS_ADMIN_CHAT_ID</code> ست می‌شود.
            </div>
          </label>
        </div>
      </div>
      
      <div className="card" style={{ marginTop: 24 }}>
        <h3>قابلیت‌های فعال Opsless</h3>
        <ul style={{ paddingLeft: 20, color: 'var(--ng-text)' }}>
          <li style={{ marginBottom: 8 }}><strong>نابوگیت Self-Healing:</strong> تشخیص قطعی سرویس‌دهنده و جایگزینی آنی (ثبت رویداد در تلگرام).</li>
          <li style={{ marginBottom: 8 }}><strong>پیگیری مالی (Invoice Chaser):</strong> اطلاع‌رسانی سررسید فاکتورها به مدیر فروش.</li>
          <li style={{ color: 'var(--ng-muted)' }}><strong>نابو ویس (NabuVoice):</strong> در حال توسعه... (بزودی)</li>
        </ul>
      </div>
    </Layout>
  );
}
