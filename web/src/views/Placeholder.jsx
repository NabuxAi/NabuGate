import Layout from '../components/Layout.jsx';

export default function Placeholder({ title, subtitle, icon, body }) {
  return (
    <Layout title={title} subtitle={subtitle}>
      <div className="placeholder">
        <div className="big" aria-hidden="true">
          {icon}
        </div>
        <h3>{title}</h3>
        <p>{body}</p>
      </div>
    </Layout>
  );
}
