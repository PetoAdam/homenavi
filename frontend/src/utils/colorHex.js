function clampColorByte(v) {
  if (Number.isNaN(v)) return 0;
  return Math.min(255, Math.max(0, Math.round(Number(v))));
}

function rgbToHex(r, g, b) {
  const toHex = (value) => clampColorByte(value).toString(16).padStart(2, '0');
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`.toUpperCase();
}

function clamp01(v) {
  if (Number.isNaN(v)) return 0;
  return Math.min(1, Math.max(0, Number(v)));
}

function hsvToRgb({ h, s, v }) {
  const hh = Number(h);
  if (!Number.isFinite(hh)) return null;
  const ss = clamp01(s);
  const vv = clamp01(v);

  const hue = ((hh % 360) + 360) % 360;
  const c = vv * ss;
  const x = c * (1 - Math.abs(((hue / 60) % 2) - 1));
  const m = vv - c;

  let r1 = 0; let g1 = 0; let b1 = 0;
  if (hue < 60) {
    r1 = c; g1 = x; b1 = 0;
  } else if (hue < 120) {
    r1 = x; g1 = c; b1 = 0;
  } else if (hue < 180) {
    r1 = 0; g1 = c; b1 = x;
  } else if (hue < 240) {
    r1 = 0; g1 = x; b1 = c;
  } else if (hue < 300) {
    r1 = x; g1 = 0; b1 = c;
  } else {
    r1 = c; g1 = 0; b1 = x;
  }

  return {
    r: (r1 + m) * 255,
    g: (g1 + m) * 255,
    b: (b1 + m) * 255,
  };
}

function xyToRgb({ x, y, Y = 1 }) {
  const xx = Number(x);
  const yy = Number(y);
  if (!Number.isFinite(xx) || !Number.isFinite(yy) || yy <= 0) return null;

  // Convert CIE xyY to XYZ with Y=1.
  const X = (xx * Y) / yy;
  const Z = ((1 - xx - yy) * Y) / yy;

  // Convert XYZ to linear sRGB (D65).
  let rLin = 3.2406 * X - 1.5372 * Y - 0.4986 * Z;
  let gLin = -0.9689 * X + 1.8758 * Y + 0.0415 * Z;
  let bLin = 0.0557 * X - 0.2040 * Y + 1.0570 * Z;

  const gamma = (c) => {
    const cc = Math.max(0, c);
    return cc <= 0.0031308 ? 12.92 * cc : 1.055 * Math.pow(cc, 1 / 2.4) - 0.055;
  };

  rLin = gamma(rLin);
  gLin = gamma(gLin);
  bLin = gamma(bLin);

  return {
    r: clamp01(rLin) * 255,
    g: clamp01(gLin) * 255,
    b: clamp01(bLin) * 255,
  };
}

function tryParsePairFromString(raw) {
  if (typeof raw !== 'string') return null;
  const trimmed = raw.trim();
  if (!trimmed) return null;
  const parts = trimmed.split(/[\s,;/]+/).filter(Boolean);
  if (parts.length < 2) return null;
  const a = Number(parts[0]);
  const b = Number(parts[1]);
  if (!Number.isFinite(a) || !Number.isFinite(b)) return null;
  return [a, b];
}

function tryExtractRgbLike(value) {
  if (!value) return null;

  // Array formats
  if (Array.isArray(value)) {
    if (value.length >= 3) {
      const r = Number(value[0]);
      const g = Number(value[1]);
      const b = Number(value[2]);
      if ([r, g, b].every(Number.isFinite)) return { r, g, b };
    }
    if (value.length >= 2) {
      const a = Number(value[0]);
      const b = Number(value[1]);
      if (!Number.isFinite(a) || !Number.isFinite(b)) return null;

      // Heuristic: 0..1 pair => xy, otherwise hue/sat.
      if (a >= 0 && a <= 1 && b >= 0 && b <= 1) {
        return xyToRgb({ x: a, y: b });
      }
      const hue = a;
      const sat = b > 1 ? b / 100 : b;
      return hsvToRgb({ h: hue, s: sat, v: 1 });
    }
  }

  if (typeof value === 'string') {
    const pair = tryParsePairFromString(value);
    if (pair) {
      const [a, b] = pair;
      if (a >= 0 && a <= 1 && b >= 0 && b <= 1) {
        return xyToRgb({ x: a, y: b });
      }
      const hue = a;
      const sat = b > 1 ? b / 100 : b;
      return hsvToRgb({ h: hue, s: sat, v: 1 });
    }
  }

  if (typeof value === 'object') {
    // {r,g,b}
    if (typeof value.r === 'number' && typeof value.g === 'number' && typeof value.b === 'number') {
      return { r: value.r, g: value.g, b: value.b };
    }

    // {x,y} or {xy:[x,y]}
    if (typeof value.x === 'number' && typeof value.y === 'number') {
      return xyToRgb({ x: value.x, y: value.y, Y: typeof value.Y === 'number' ? value.Y : 1 });
    }
    if (Array.isArray(value.xy) && value.xy.length >= 2) {
      const x = Number(value.xy[0]);
      const y = Number(value.xy[1]);
      if (Number.isFinite(x) && Number.isFinite(y)) return xyToRgb({ x, y });
    }

    // HSV-ish objects
    const hue = value.h ?? value.hue;
    const sat = value.s ?? value.saturation;
    const val = value.v ?? value.value ?? value.brightness;
    if (hue !== undefined && sat !== undefined) {
      const hNum = Number(hue);
      const sNum = Number(sat);
      const vNum = val === undefined ? 1 : Number(val);
      if (Number.isFinite(hNum) && Number.isFinite(sNum)) {
        const s01 = sNum > 1 ? sNum / 100 : sNum;
        const v01 = Number.isFinite(vNum) ? (vNum > 1 ? vNum / 100 : vNum) : 1;
        return hsvToRgb({ h: hNum, s: s01, v: v01 });
      }
    }
  }

  return null;
}

function extractHexColor(value) {
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (/^#[0-9a-f]{6}$/i.test(trimmed)) return trimmed.toUpperCase();
    if (/^[0-9a-f]{6}$/i.test(trimmed)) return `#${trimmed.toUpperCase()}`;
    // fall through to parsing pairs (xy/hs)
  }
  if (value && typeof value === 'object') {
    if (typeof value.hex === 'string') {
      return extractHexColor(value.hex);
    }
  }

  const rgb = tryExtractRgbLike(value);
  if (rgb && typeof rgb.r === 'number' && typeof rgb.g === 'number' && typeof rgb.b === 'number') {
    return rgbToHex(rgb.r, rgb.g, rgb.b);
  }
  return null;
}

export function normalizeColorHex(value, fallback = '#FFFFFF') {
  return extractHexColor(value) || fallback;
}
