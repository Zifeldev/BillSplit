import { Geologica_600SemiBold } from '@expo-google-fonts/geologica/600SemiBold';
import { Geologica_700Bold } from '@expo-google-fonts/geologica/700Bold';
import { Onest_400Regular } from '@expo-google-fonts/onest/400Regular';
import { Onest_500Medium } from '@expo-google-fonts/onest/500Medium';
import { Onest_600SemiBold } from '@expo-google-fonts/onest/600SemiBold';
import { Onest_700Bold } from '@expo-google-fonts/onest/700Bold';
import type { TextStyle } from 'react-native';

/** Passed to `useFonts` in the root layout; keys are the family names used below. */
export const fontAssets = {
  Geologica_600SemiBold,
  Geologica_700Bold,
  Onest_400Regular,
  Onest_500Medium,
  Onest_600SemiBold,
  Onest_700Bold,
};

/**
 * Geologica (display) and Onest (text) both cover Latin and Cyrillic.
 * Every weight is its own family: setting `fontWeight` next to a custom family makes
 * web and Android synthesize a fake bold, so pick a family here and leave weight unset.
 */
export const fonts = {
  display: 'Geologica_700Bold',
  displaySemi: 'Geologica_600SemiBold',
  regular: 'Onest_400Regular',
  medium: 'Onest_500Medium',
  semibold: 'Onest_600SemiBold',
  bold: 'Onest_700Bold',
} as const;

type Weight = TextStyle['fontWeight'];

const SEMIBOLD_FROM = 600;
const BOLD_FROM = 700;
const MEDIUM_FROM = 500;

function numericWeight(weight: Weight): number {
  if (weight === 'bold') return BOLD_FROM;
  const n = Number(weight);
  return Number.isFinite(n) ? n : 400;
}

/** Onest family for a CSS-style weight. */
export function bodyFont(weight?: Weight): string {
  const n = numericWeight(weight);
  if (n >= BOLD_FROM) return fonts.bold;
  if (n >= SEMIBOLD_FROM) return fonts.semibold;
  if (n >= MEDIUM_FROM) return fonts.medium;
  return fonts.regular;
}

/** Geologica family for a CSS-style weight (only 600 and 700 are bundled). */
export function displayFont(weight?: Weight): string {
  return weight !== undefined && numericWeight(weight) < BOLD_FROM ? fonts.displaySemi : fonts.display;
}
