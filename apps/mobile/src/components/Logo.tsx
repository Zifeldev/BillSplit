import React from 'react';
import { StyleSheet, Text, View } from 'react-native';
import Svg, { Path } from 'react-native-svg';

import { fonts, heroGradient, useTheme } from '@/theme';

const MARK_RATIO = 0.66;
const MARK_RADIUS_RATIO = 0.28;
const WORDMARK_RATIO = 0.62;

/** Brand mark: a coin split in two halves that slide apart, as on the app icon. */
export function LogoMark({ size = 36 }: { size?: number }) {
  const { c } = useTheme();
  return (
    <View style={[styles.mark, { width: size, height: size, borderRadius: size * MARK_RADIUS_RATIO }, heroGradient(c.hero)]}>
      <Svg width={size * MARK_RATIO} height={size * MARK_RATIO} viewBox="0 0 24 24">
        <Path d="M11.2 2.6a8 8 0 0 0 0 16z" fill="#FFFFFF" />
        <Path d="M12.8 5.4a8 8 0 0 1 0 16z" fill="#FFFFFF" fillOpacity={0.68} />
      </Svg>
    </View>
  );
}

/** Mark plus the lowercase wordmark. */
export function Logo({ size = 36, color }: { size?: number; color?: string }) {
  const { c } = useTheme();
  return (
    <View style={styles.row} accessible accessibilityRole="image" accessibilityLabel="billsplit">
      <LogoMark size={size} />
      <Text style={[styles.wordmark, { fontSize: size * WORDMARK_RATIO, color: color ?? c.text }]}>billsplit</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  mark: { alignItems: 'center', justifyContent: 'center' },
  wordmark: { fontFamily: fonts.display, letterSpacing: -0.5 },
});
