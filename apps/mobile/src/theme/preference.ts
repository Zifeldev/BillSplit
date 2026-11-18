import { useSyncExternalStore } from 'react';
import { Appearance, Platform } from 'react-native';

import { getItem, setItem } from '@/api/tokens';

import { DEFAULT_PALETTE, PALETTES, type PaletteName } from './palettes';

/** `system` follows the device setting; the other two pin the theme. */
export type ThemePreference = 'system' | 'light' | 'dark';

const STORAGE_KEY = 'billsplit.theme';
const PALETTE_STORAGE_KEY = 'billsplit.palette';
const PREFERENCES: readonly ThemePreference[] = ['system', 'light', 'dark'];

let current: ThemePreference = 'system';
let currentPalette: PaletteName = DEFAULT_PALETTE;
const listeners = new Set<() => void>();

function isPaletteName(value: unknown): value is PaletteName {
  return PALETTES.some((p) => p.name === value);
}

function isPreference(value: unknown): value is ThemePreference {
  return typeof value === 'string' && (PREFERENCES as readonly string[]).includes(value);
}

/** Keeps system-drawn UI (keyboard, alerts, native glass) on the same theme as the app. */
function syncNativeAppearance(preference: ThemePreference): void {
  if (Platform.OS === 'web') return;
  Appearance.setColorScheme(preference === 'system' ? 'unspecified' : preference);
}

function notify(): void {
  listeners.forEach((listener) => listener());
}

function apply(preference: ThemePreference): void {
  current = preference;
  syncNativeAppearance(preference);
  notify();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function snapshot(): ThemePreference {
  return current;
}

function palette(): PaletteName {
  return currentPalette;
}

/** Restores the saved theme and palette; call once at startup. Unknown or missing values keep the defaults. */
export async function loadThemePreference(): Promise<void> {
  const [saved, savedPalette] = await Promise.all([getItem(STORAGE_KEY), getItem(PALETTE_STORAGE_KEY)]);
  if (isPaletteName(savedPalette)) currentPalette = savedPalette;
  if (isPreference(saved)) apply(saved);
  else notify();
}

export function setThemePreference(preference: ThemePreference): void {
  apply(preference);
  void setItem(STORAGE_KEY, preference);
}

export function useThemePreference(): ThemePreference {
  return useSyncExternalStore(subscribe, snapshot, snapshot);
}

export function setPalette(name: PaletteName): void {
  currentPalette = name;
  notify();
  void setItem(PALETTE_STORAGE_KEY, name);
}

export function usePalette(): PaletteName {
  return useSyncExternalStore(subscribe, palette, palette);
}
