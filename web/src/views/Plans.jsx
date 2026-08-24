import Layout from '../components/Layout.jsx';

export default function Plans() {
  return (
    <Layout title="پلن‌ها" subtitle="انتخاب و ارتقای اشتراک">
      <div className="card" style={{ padding: 24, textAlign: 'center' }}>
        <h3>پلن فعلی: پرداخت به اندازه مصرف (PAYG)</h3>
        <p className="muted" style={{ margin: '16px 0' }}>با شارژ موجودی حساب، هزینه استفاده از مدل‌ها از موجودی شما کسر می‌شود.</p>
        <button className="btn btn-primary" onClick={() => window.location.hash = '#/account'}>شارژ حساب</button>
      </div>
    </Layout>
  );
}
