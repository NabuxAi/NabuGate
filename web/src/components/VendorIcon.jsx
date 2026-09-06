/*
 * A vendor's mark, or its initial.
 *
 * The catalogue carries an SVG path for the vendors whose logo is a single
 * shape and nothing for the rest. A missing icon is a supported state, not a
 * broken image: the tile falls back to a lettermark in the same brand colour,
 * so a provider added to the config tomorrow renders correctly with no
 * frontend change.
 *
 * Several brands are black or near-black — OpenAI, ElevenLabs, Speechmatics,
 * xAI, Replicate, Runway. Painted at their real colour on this console's dark
 * surface they are an invisible blob, so a mark that dark is lightened toward
 * the page's foreground under the night theme. The decision is made in CSS
 * from a class rather than here, so it follows the theme toggle without a
 * re-render.
 */

// isDark is the relative luminance test, on the brand hex. Anything under a
// fifth of full brightness cannot be seen on the night theme's surface.
function isDarkMark(hex) {
  const m = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(hex || '');
  if (!m) return false;
  let h = m[1];
  if (h.length === 3) h = h.split('').map((c) => c + c).join('');
  const [r, g, b] = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
  // Rec. 709 luma, close enough for a yes/no on a logo.
  return 0.2126 * r + 0.7152 * g + 0.0722 * b < 0.22;
}

export default function VendorIcon({ vendor, size = 34 }) {
  const color = vendor.color || 'var(--ng-muted)';
  const cls = 'vendor-icon' + (isDarkMark(vendor.color) ? ' is-dark-mark' : '');
  const style = {
    '--mark': color,
    width: size,
    height: size,
    borderRadius: 10,
    flex: `0 0 ${size}px`,
  };

  if (vendor.icon) {
    return (
      <span className={cls} style={style} aria-hidden="true">
        <svg viewBox="0 0 24 24" width={size * 0.56} height={size * 0.56}>
          <path d={vendor.icon} fill="currentColor" />
        </svg>
      </span>
    );
  }
  return (
    <span className={cls} style={{ ...style, fontWeight: 800, fontSize: size * 0.42 }} aria-hidden="true">
      {(vendor.label || vendor.name || '?').trim().charAt(0).toUpperCase()}
    </span>
  );
}
