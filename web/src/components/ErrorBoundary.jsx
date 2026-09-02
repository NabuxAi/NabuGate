import { Component } from 'react';

/*
 * A page that throws while rendering must not take the sidebar with it. The
 * message is shown, because "something went wrong" sends the person to the
 * console to learn what this box already knows.
 */
export default class ErrorBoundary extends Component {
  constructor(props) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error) {
    return { error };
  }

  componentDidUpdate(prev) {
    // A new page is a new chance; keep the error scoped to the one that threw.
    if (prev.resetKey !== this.props.resetKey && this.state.error) this.setState({ error: null });
  }

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <div className="main">
        <div className="content">
          <div className="callout danger fade-in" role="alert">
            <span className="ci">⚠️</span>
            <div>
              <strong>این صفحه نتوانست رسم شود.</strong>
              <div className="mono" dir="ltr" style={{ fontSize: 12, marginTop: 6, whiteSpace: 'pre-wrap' }}>{String(this.state.error?.message || this.state.error)}</div>
              <button type="button" className="btn btn-outline btn-sm" style={{ marginTop: 10 }} onClick={() => window.location.reload()}>بارگذاری دوباره</button>
            </div>
          </div>
        </div>
      </div>
    );
  }
}
