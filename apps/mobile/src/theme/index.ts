import { Platform, useColorScheme, useWindowDimensions, ViewStyle } from 'react-native';

import { BRAND_OVERRIDES, type PaletteName } from './palettes';
import { usePalette, useThemePreference } from './preference';

export { bodyFont, displayFont, fontAssets, fonts } from './fonts';
export { PALETTES, type PaletteInfo, type PaletteName } from './palettes';
export {
  loadThemePreference,
  setPalette,
  setThemePreference,
  usePalette,
  useThemePreference,
  type ThemePreference,
} from './preference';

export interface Palette {
  /** Base color under the aurora backdrop; also used by navigation headers. */
  bg: string;
  /** Opaque surface: sheets on top of a dimmed screen, rings around avatars. */
  card: string;
  /** Recessed fill for inputs and tracks; translucent so it works on glass. */
  cardAlt: string;
  /** Frosted fill of cards — the backdrop shows through. */
  glass: string;
  /** Bright rim that gives glass its edge. */
  glassBorder: string;
  /** Denser frosted fill for sheets, menus and bars that sit above content. */
  glassStrong: string;
  text: string;
  textMuted: string;
  border: string;
  /** Stronger border for outlined buttons and inputs. */
  outline: string;
  /** Brand tint for text/icons on neutral surfaces. */
  primary: string;
  /** Brand tint for text on `primarySoft` surfaces. */
  primaryStrong: string;
  /** Text on `accent` fills (dark ink on the mint accent in the dark theme). */
  primaryText: string;
  primarySoft: string;
  /** Solid fill for primary buttons. */
  accent: string;
  /** Glow under primary buttons and the "+" button. */
  accentGlow: string;
  /** Hero panel gradient stops; white text stays readable on both. */
  hero: [string, string];
  danger: string;
  /** Filled danger surfaces with white text (count badges). */
  dangerSolid: string;
  dangerSoft: string;
  warning: string;
  warningSoft: string;
  positive: string;
  positiveSoft: string;
  negative: string;
  negativeSoft: string;
  needs: string;
  wants: string;
  savings: string;
  chartMuted: string;
  skeleton: string;
  shadow: string;
  /** Three soft light sources of the aurora backdrop. */
  aurora: [string, string, string];
}

const NO_SHADOW = 'rgba(0, 0, 0, 0)';

const light: Palette = {
  bg: '#EDF4F0',
  card: '#FFFFFF',
  cardAlt: 'rgba(9, 60, 42, 0.07)',
  glass: 'rgba(255, 255, 255, 0.62)',
  glassBorder: 'rgba(255, 255, 255, 0.92)',
  glassStrong: 'rgba(255, 255, 255, 0.84)',
  text: '#0B1F17',
  textMuted: '#4A5F55',
  border: 'rgba(11, 31, 23, 0.10)',
  outline: 'rgba(11, 31, 23, 0.26)',
  primary: '#0B7A52',
  primaryStrong: '#075C3D',
  primaryText: '#FFFFFF',
  primarySoft: 'rgba(11, 122, 82, 0.13)',
  accent: '#0B7A52',
  accentGlow: 'rgba(11, 122, 82, 0.34)',
  hero: ['#0C8058', '#0A6470'],
  danger: '#B93226',
  dangerSolid: '#B93226',
  dangerSoft: 'rgba(214, 69, 53, 0.14)',
  warning: '#855600',
  warningSoft: 'rgba(224, 164, 40, 0.20)',
  positive: '#0B7A52',
  positiveSoft: 'rgba(11, 122, 82, 0.13)',
  negative: '#B93226',
  negativeSoft: 'rgba(214, 69, 53, 0.14)',
  needs: '#2F7FB5',
  wants: '#C9772B',
  savings: '#0B7A52',
  chartMuted: 'rgba(11, 31, 23, 0.16)',
  skeleton: 'rgba(11, 31, 23, 0.09)',
  shadow: 'rgba(8, 60, 42, 0.10)',
  aurora: ['rgba(94, 232, 170, 0.50)', 'rgba(110, 205, 240, 0.42)', 'rgba(205, 240, 130, 0.36)'],
};

const dark: Palette = {
  bg: '#050A08',
  card: '#101B17',
  cardAlt: 'rgba(255, 255, 255, 0.08)',
  glass: 'rgba(255, 255, 255, 0.07)',
  glassBorder: 'rgba(255, 255, 255, 0.15)',
  glassStrong: 'rgba(16, 28, 24, 0.86)',
  text: '#EAF5F0',
  textMuted: '#B0C4BA',
  border: 'rgba(255, 255, 255, 0.11)',
  outline: 'rgba(255, 255, 255, 0.30)',
  primary: '#5EF2B0',
  primaryStrong: '#A8F8D4',
  primaryText: '#03130C',
  primarySoft: 'rgba(94, 242, 176, 0.14)',
  accent: '#4DE3A1',
  accentGlow: 'rgba(77, 227, 161, 0.36)',
  hero: ['#0B6B49', '#08505C'],
  danger: '#FF8F7F',
  dangerSolid: '#C2402E',
  dangerSoft: 'rgba(255, 120, 100, 0.16)',
  warning: '#F5C451',
  warningSoft: 'rgba(245, 196, 81, 0.15)',
  positive: '#5EF2B0',
  positiveSoft: 'rgba(94, 242, 176, 0.14)',
  negative: '#FF8F7F',
  negativeSoft: 'rgba(255, 120, 100, 0.16)',
  needs: '#5BA8D9',
  wants: '#E0954A',
  savings: '#4DE3A1',
  chartMuted: 'rgba(255, 255, 255, 0.16)',
  skeleton: 'rgba(255, 255, 255, 0.09)',
  shadow: NO_SHADOW,
  aurora: ['rgba(20, 190, 125, 0.34)', 'rgba(20, 130, 190, 0.28)', 'rgba(110, 80, 230, 0.22)'],
};

/**
 * Identity tones for avatars, group covers and "kind" tiles. Mid-tones on purpose:
 * a white glyph stays readable on each (at least 4.5:1).
 */
export const tone = {
  forest: '#0B7A52',
  moss: '#5F7F35',
  pine: '#127C86',
  slate: '#4466A8',
  plum: '#8A4F9E',
  brick: '#B23A2E',
  clay: '#B85C38',
  ochre: '#8F5F14',
  stone: '#5E6B63',
} as const;

/** Text on hero panels. */
export const onHero = { text: '#FFFFFF', soft: 'rgba(255,255,255,0.84)', chip: 'rgba(255,255,255,0.16)' } as const;

export const spacing = { xs: 4, sm: 8, md: 12, lg: 16, xl: 24, xxl: 32 } as const;
export const radius = { sm: 12, md: 18, lg: 26, xl: 32, pill: 999 } as const;
export const MAX_WIDTH = 720;
/** From this window width the web app swaps the bottom tab bar for a sidebar. */
export const WIDE_BREAKPOINT = 1024;
export const SIDEBAR_WIDTH = 264;
/** Floating tab bar metrics; screens pad their content by the bar plus the bottom safe area. */
export const TAB_BAR_HEIGHT = 68;
export const TAB_BAR_GAP = 10;

const resolved = new Map<string, Palette>();

/** Full token set for a palette and theme: the emerald base with that palette's brand colors on top. */
export function paletteTokens(name: PaletteName, isDark: boolean): Palette {
  const base = isDark ? dark : light;
  if (name === 'emerald') return base;
  const key = `${name}:${isDark ? 'dark' : 'light'}`;
  const cached = resolved.get(key);
  if (cached) return cached;
  const overrides = BRAND_OVERRIDES[name];
  const tokens: Palette = { ...base, ...(isDark ? overrides.dark : overrides.light) };
  resolved.set(key, tokens);
  return tokens;
}

export function useTheme(): { c: Palette; dark: boolean } {
  const system = useColorScheme();
  const preference = useThemePreference();
  const palette = usePalette();
  const isDark = preference === 'system' ? system === 'dark' : preference === 'dark';
  return { c: paletteTokens(palette, isDark), dark: isDark };
}

/**
 * True on desktop-sized browser windows, where there is room for a sidebar and side-by-side panels.
 * Web only: tablets keep the touch layout with the bottom tab bar.
 */
export function useIsWide(): boolean {
  const { width } = useWindowDimensions();
  return Platform.OS === 'web' && width >= WIDE_BREAKPOINT;
}

/** Soft card elevation (light theme); dark theme relies on glass rims instead. */
export function softShadow(c: Palette, strength: 1 | 2 = 1): ViewStyle {
  if (c.shadow === NO_SHADOW) return {};
  return {
    boxShadow: strength === 1 ? `0px 8px 28px ${c.shadow}` : `0px 20px 48px rgba(8, 60, 42, 0.20)`,
  };
}

/** Diagonal hero gradient as a style (CSS gradients work on native and web). */
export function heroGradient(stops: readonly [string, string]): ViewStyle {
  return { backgroundColor: stops[0], backgroundImage: `linear-gradient(135deg, ${stops[0]} 0%, ${stops[1]} 100%)` };
}
