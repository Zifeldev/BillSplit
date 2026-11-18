import type { Palette } from './index';

/** Color palettes the user can pick. `emerald` is the base palette defined in `index.ts`. */
export type PaletteName = 'emerald' | 'ocean' | 'violet' | 'sunset';

/** The tokens a palette may restyle: brand colors, the page tint and the aurora. Money semantics stay put. */
export type BrandTokens = Pick<
  Palette,
  | 'bg'
  | 'card'
  | 'primary'
  | 'primaryStrong'
  | 'primaryText'
  | 'primarySoft'
  | 'accent'
  | 'accentGlow'
  | 'hero'
  | 'aurora'
>;

export interface PaletteInfo {
  name: PaletteName;
  label: string;
}

export const PALETTES: readonly PaletteInfo[] = [
  { name: 'emerald', label: 'Emerald' },
  { name: 'ocean', label: 'Ocean' },
  { name: 'violet', label: 'Violet' },
  { name: 'sunset', label: 'Sunset' },
];

export const DEFAULT_PALETTE: PaletteName = 'emerald';

/**
 * Overrides on top of the emerald base, per theme. Contrast kept on every entry: white on the
 * light `accent` and on both `hero` stops, dark ink (`primaryText`) on the dark `accent`.
 */
export const BRAND_OVERRIDES: Record<Exclude<PaletteName, 'emerald'>, { light: BrandTokens; dark: BrandTokens }> = {
  ocean: {
    light: {
      bg: '#ECF2F9',
      card: '#FFFFFF',
      primary: '#0B62B8',
      primaryStrong: '#084A8C',
      primaryText: '#FFFFFF',
      primarySoft: 'rgba(11, 98, 184, 0.12)',
      accent: '#0B62B8',
      accentGlow: 'rgba(11, 98, 184, 0.34)',
      hero: ['#0E6FCB', '#0A4F9E'],
      aurora: ['rgba(110, 190, 255, 0.50)', 'rgba(120, 235, 225, 0.40)', 'rgba(175, 170, 255, 0.34)'],
    },
    dark: {
      bg: '#05080D',
      card: '#0F1620',
      primary: '#6CB8FF',
      primaryStrong: '#B3DAFF',
      primaryText: '#04121F',
      primarySoft: 'rgba(108, 184, 255, 0.15)',
      accent: '#58A9F7',
      accentGlow: 'rgba(88, 169, 247, 0.36)',
      hero: ['#0B4F9A', '#0A3A78'],
      aurora: ['rgba(30, 120, 220, 0.34)', 'rgba(20, 170, 190, 0.26)', 'rgba(110, 80, 230, 0.22)'],
    },
  },
  violet: {
    light: {
      bg: '#F1EEF9',
      card: '#FFFFFF',
      primary: '#6B3FD4',
      primaryStrong: '#4F2BA6',
      primaryText: '#FFFFFF',
      primarySoft: 'rgba(107, 63, 212, 0.12)',
      accent: '#6B3FD4',
      accentGlow: 'rgba(107, 63, 212, 0.34)',
      hero: ['#7446DE', '#A03BB8'],
      aurora: ['rgba(180, 150, 255, 0.50)', 'rgba(255, 160, 220, 0.38)', 'rgba(140, 200, 255, 0.36)'],
    },
    dark: {
      bg: '#08060D',
      card: '#15111F',
      primary: '#B79CFF',
      primaryStrong: '#DACBFF',
      primaryText: '#120A26',
      primarySoft: 'rgba(183, 156, 255, 0.15)',
      accent: '#A98BFF',
      accentGlow: 'rgba(169, 139, 255, 0.36)',
      hero: ['#5230B0', '#7A2C92'],
      aurora: ['rgba(120, 80, 230, 0.36)', 'rgba(210, 70, 170, 0.24)', 'rgba(40, 120, 210, 0.24)'],
    },
  },
  sunset: {
    light: {
      bg: '#F9F1EB',
      card: '#FFFFFF',
      primary: '#A8500F',
      primaryStrong: '#7E3A08',
      primaryText: '#FFFFFF',
      primarySoft: 'rgba(168, 80, 15, 0.12)',
      accent: '#A8500F',
      accentGlow: 'rgba(168, 80, 15, 0.34)',
      hero: ['#B9541A', '#B0306A'],
      aurora: ['rgba(255, 175, 110, 0.50)', 'rgba(255, 135, 175, 0.36)', 'rgba(255, 222, 120, 0.40)'],
    },
    dark: {
      bg: '#0D0806',
      card: '#1D1410',
      primary: '#FFB173',
      primaryStrong: '#FFD2B0',
      primaryText: '#1F0C04',
      primarySoft: 'rgba(255, 177, 115, 0.15)',
      accent: '#FF9F5F',
      accentGlow: 'rgba(255, 159, 95, 0.34)',
      hero: ['#9A3F14', '#8A2452'],
      aurora: ['rgba(230, 110, 50, 0.32)', 'rgba(210, 60, 120, 0.24)', 'rgba(230, 170, 40, 0.20)'],
    },
  },
};
