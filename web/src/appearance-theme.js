export const THEME_KEY = 'im.appearance'
export const themePresets = [
  { name: '暖杏', color: '#b7654c' },
  { name: '湖蓝', color: '#3675b5' },
  { name: '松绿', color: '#35816c' },
  { name: '鸢紫', color: '#8060b0' },
  { name: '玫瑰', color: '#b34f76' },
  { name: '青灰', color: '#607580' },
]
export const defaultColor = themePresets[0].color

export function normalizeColor(value) {
  return typeof value === 'string' && /^#[\da-f]{6}$/i.test(value) ? value.toLowerCase() : defaultColor
}

// Keep surface tints subtle and action colors dark enough for white labels.
export function themeTokens(value) {
  const color = normalizeColor(value)
  const rgb = [1, 3, 5].map(index => parseInt(color.slice(index, index + 2), 16) / 255)
  const max = Math.max(...rgb), min = Math.min(...rgb), delta = max - min
  let hue = 0
  if (delta) {
    if (max === rgb[0]) hue = ((rgb[1] - rgb[2]) / delta) % 6
    else if (max === rgb[1]) hue = (rgb[2] - rgb[0]) / delta + 2
    else hue = (rgb[0] - rgb[1]) / delta + 4
  }
  hue = (hue * 60 + 360) % 360
  const lightness = (max + min) / 2
  const saturation = delta ? delta / (1 - Math.abs(2 * lightness - 1)) : 0
  const sat = Math.round(saturation * 100)
  const hsl = (s, l) => `hsl(${Math.round(hue)} ${s}% ${l}%)`
  const accentSaturation = Math.min(sat, 65)
  let accentLightness = Math.min(Math.round(lightness * 100), 40)
  const luminance = l => {
    const s = accentSaturation / 100, level = l / 100
    const channel = offset => {
      const k = (offset + Math.round(hue) / 30) % 12
      const value = level - s * Math.min(level, 1 - level) * Math.max(-1, Math.min(k - 3, 9 - k, 1))
      return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
    }
    return 0.2126 * channel(0) + 0.7152 * channel(8) + 0.0722 * channel(4)
  }
  while (1.05 / (luminance(accentLightness) + 0.05) < 4.5) accentLightness--
  return {
    '--accent': hsl(accentSaturation, accentLightness),
    '--accent-dark': hsl(Math.min(sat, 65), 30),
    '--ink': hsl(Math.min(sat, 20), 18),
    '--muted': hsl(Math.min(sat, 15), 46),
    '--line': hsl(Math.min(sat, 28), 86),
    '--card': hsl(Math.min(sat, 32), 99),
    '--sage': hsl(Math.min(sat, 25), 49),
    '--theme-page': hsl(Math.min(sat, 32), 96),
    '--theme-sidebar': hsl(Math.min(sat, 36), 98),
    '--theme-surface': hsl(Math.min(sat, 28), 99),
    '--theme-soft': hsl(Math.min(sat, 36), 95),
    '--theme-hover': hsl(Math.min(sat, 36), 91),
    '--theme-selected': hsl(Math.min(sat, 42), 88),
    '--theme-bubble': hsl(Math.min(sat, 32), 94),
  }
}

export function applyTheme(value, root = document.documentElement) {
  const color = normalizeColor(value)
  // The original warm palette remains the default.
  for (const [key, token] of Object.entries(themeTokens(color))) {
    if (color === defaultColor) root.style.removeProperty(key)
    else root.style.setProperty(key, token)
  }
  return color
}

export function loadTheme(storage = localStorage) {
  try { return normalizeColor(storage.getItem(THEME_KEY)) } catch { return defaultColor }
}

export function saveTheme(value, storage = localStorage) {
  const color = normalizeColor(value)
  storage.setItem(THEME_KEY, color)
  return color
}
