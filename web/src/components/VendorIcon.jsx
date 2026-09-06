/*
 * A vendor's mark, or its initial.
 *
 * The catalogue carries an SVG path for the vendors whose logo is a single
 * shape and nothing for the rest. A missing icon is a supported state, not a
 * broken image: the tile falls back to a lettermark in the same brand colour,
 * so a provider added to the config tomorrow renders correctly with no
 * frontend change.
 */
export default function VendorIcon({ vendor, size = 34 }) {
  const color = vendor.color || 'var(--ng-muted)';
  const style = {
    width: size,
    height: size,
    borderRadius: 10,
    display: 'grid',
    placeItems: 'center',
    flex: `0 0 ${size}px`,
    background: `color-mix(in srgb, ${color} 14%, transparent)`,
    border: `1px solid color-mix(in srgb, ${color} 30%, transparent)`,
  };

  if (vendor.icon) {
    return (
      <span style={style} aria-hidden="true">
        <svg viewBox="0 0 24 24" width={size * 0.56} height={size * 0.56} fill={color}>
          <path d={vendor.icon} />
        </svg>
      </span>
    );
  }
  return (
    <span style={{ ...style, color, fontWeight: 800, fontSize: size * 0.42 }} aria-hidden="true">
      {(vendor.label || vendor.name || '?').trim().charAt(0).toUpperCase()}
    </span>
  );
}
