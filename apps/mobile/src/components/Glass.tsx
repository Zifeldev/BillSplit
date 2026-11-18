import { GlassView, isLiquidGlassAvailable } from 'expo-glass-effect';
import React from 'react';
import { Platform, StyleProp, StyleSheet, View, ViewStyle } from 'react-native';

import { Palette, useTheme } from '@/theme';

/** Real Liquid Glass exists on iOS 26+; everywhere else the frosted fallback below is used. */
export const HAS_LIQUID_GLASS = Platform.OS === 'ios' && isLiquidGlassAvailable();

const WEB_BLUR = 'blur(22px) saturate(170%)';

/** Web-only blur of whatever is behind the surface; React Native ignores the property elsewhere. */
const webBlur: ViewStyle =
  Platform.OS === 'web' ? ({ backdropFilter: WEB_BLUR, WebkitBackdropFilter: WEB_BLUR } as ViewStyle) : {};

/**
 * Frosted surface style for content cards: translucent fill, bright rim and, on the web, a backdrop blur.
 * Cheap enough for long lists, unlike a native glass view per card.
 */
export function frosted(c: Palette, strong = false): ViewStyle {
  return {
    backgroundColor: strong ? c.glassStrong : c.glass,
    borderColor: c.glassBorder,
    borderWidth: 1,
    ...webBlur,
  };
}

/**
 * Floating chrome (tab bar, sheets, the "+" button): Liquid Glass on iOS 26+, frosted elsewhere.
 * Keep it for the few elements that sit above content — the system material is costly in bulk.
 */
export function Glass({
  children,
  style,
}: {
  children?: React.ReactNode;
  style?: StyleProp<ViewStyle>;
}) {
  const { c, dark } = useTheme();
  if (HAS_LIQUID_GLASS) {
    // The glass follows the app theme, not whatever happens to be behind it (a dimmed modal backdrop).
    return (
      <GlassView glassEffectStyle="regular" colorScheme={dark ? 'dark' : 'light'} style={[styles.clip, style]}>
        {children}
      </GlassView>
    );
  }
  return <View style={[styles.clip, frosted(c, true), style]}>{children}</View>;
}

/** The same `rgba()` color at zero alpha, so a light fades out without a grey fringe. */
function clear(rgba: string): string {
  return rgba.replace(/[\d.]+\)$/, '0)');
}

/** The aurora as a CSS background: three soft light sources for the glass to pick up. */
export function auroraImage(aurora: Palette['aurora']): string {
  const [a, b, d] = aurora;
  return [
    `radial-gradient(circle at 12% 18%, ${a} 0%, ${clear(a)} 46%)`,
    `radial-gradient(circle at 92% 38%, ${b} 0%, ${clear(b)} 44%)`,
    `radial-gradient(circle at 34% 96%, ${d} 0%, ${clear(d)} 48%)`,
  ].join(', ');
}

/** Fixed aurora behind a screen. */
export function Backdrop() {
  const { c } = useTheme();
  return (
    <View
      pointerEvents="none"
      style={[StyleSheet.absoluteFill, { backgroundColor: c.bg, backgroundImage: auroraImage(c.aurora) }]}
    />
  );
}

/** Frosted bar behind a transparent navigation header, so content can scroll under it. */
export function HeaderGlass() {
  const { c } = useTheme();
  return (
    <View
      pointerEvents="none"
      style={[StyleSheet.absoluteFill, frosted(c, true), styles.headerEdge, { borderColor: c.border }]}
    />
  );
}

const styles = StyleSheet.create({
  /** Only the bottom hairline of the glass rim. */
  headerEdge: { borderTopWidth: 0, borderLeftWidth: 0, borderRightWidth: 0, borderBottomWidth: StyleSheet.hairlineWidth },
  clip: { overflow: 'hidden' },
});
