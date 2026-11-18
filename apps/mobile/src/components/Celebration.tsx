import React, { useEffect, useMemo } from 'react';
import { Modal, Pressable, StyleSheet, View } from 'react-native';
import Animated, {
  Easing,
  useAnimatedStyle,
  useSharedValue,
  withDelay,
  withSpring,
  withTiming,
} from 'react-native-reanimated';

import { successFeedback } from '@/lib/haptics';
import { radius, spacing, tone, useTheme } from '@/theme';

import { frosted } from './Glass';
import { Icon, type IconName } from './Icon';
import { Button, Txt } from './ui';

/** Confetti colors: the brand tones, so the burst belongs to the palette. */
const CONFETTI = [tone.forest, tone.pine, tone.slate, tone.plum, tone.clay, tone.ochre];

function Particle({ i, n }: { i: number; n: number }) {
  const t = useSharedValue(0);
  const angle = (i / n) * Math.PI * 2 + (i % 3) * 0.3;
  const dist = 110 + (i % 4) * 34;
  useEffect(() => {
    t.value = withDelay((i % 5) * 30, withTiming(1, { duration: 1100, easing: Easing.out(Easing.cubic) }));
  }, [t, i]);
  const style = useAnimatedStyle(() => ({
    opacity: t.value < 0.7 ? 1 : 1 - (t.value - 0.7) / 0.3,
    transform: [
      { translateX: Math.cos(angle) * dist * t.value },
      { translateY: Math.sin(angle) * dist * t.value + 60 * t.value * t.value },
      { scale: 0.4 + t.value * 0.8 },
      { rotate: `${(i % 2 ? 1 : -1) * 120 * t.value}deg` },
    ],
  }));
  return (
    <Animated.View style={[styles.particle, style]} pointerEvents="none">
      <View style={[styles.confetti, { backgroundColor: CONFETTI[i % CONFETTI.length] }]} />
    </Animated.View>
  );
}

/** Success sheet with a confetti burst. Shown after finalize / settle up. */
export function Celebration({
  visible,
  title,
  message,
  icon = 'confetti',
  onClose,
  actionLabel = 'Nice!',
}: {
  visible: boolean;
  title: string;
  message?: string;
  icon?: IconName;
  onClose: () => void;
  actionLabel?: string;
}) {
  const { c } = useTheme();
  const pop = useSharedValue(0);
  useEffect(() => {
    if (visible) {
      successFeedback();
      pop.value = 0;
      pop.value = withSpring(1, { damping: 11, stiffness: 160 });
    }
  }, [visible, pop]);
  const popStyle = useAnimatedStyle(() => ({ transform: [{ scale: 0.6 + 0.4 * pop.value }], opacity: pop.value }));
  const particles = useMemo(() => Array.from({ length: 18 }, (_, i) => i), []);
  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose} accessibilityLabel="Close">
        <Animated.View style={[styles.sheet, frosted(c, true), popStyle]}>
          <View style={[styles.badge, { backgroundColor: c.primarySoft }]}>
            <Icon name={icon} size={42} color={c.primary} weight="duotone" />
          </View>
          <Txt variant="title" style={styles.center}>
            {title}
          </Txt>
          {message ? (
            <Txt variant="muted" style={styles.center}>
              {message}
            </Txt>
          ) : null}
          <Button title={actionLabel} onPress={onClose} style={styles.button} />
        </Animated.View>
        <View style={styles.burst} pointerEvents="none">
          {visible ? particles.map((i) => <Particle key={i} i={i} n={particles.length} />) : null}
        </View>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    backgroundColor: 'rgba(13,20,16,0.6)',
    alignItems: 'center',
    justifyContent: 'center',
    padding: spacing.lg,
  },
  burst: { position: 'absolute', top: '40%', left: '50%', width: 0, height: 0 },
  particle: { position: 'absolute', left: -14, top: -14 },
  confetti: { width: 10, height: 16, borderRadius: 3 },
  sheet: {
    width: '100%',
    maxWidth: 380,
    borderRadius: radius.xl,
    padding: spacing.xl,
    alignItems: 'center',
    gap: spacing.md,
  },
  badge: {
    width: 84,
    height: 84,
    borderRadius: 42,
    alignItems: 'center',
    justifyContent: 'center',
    overflow: 'hidden',
    marginTop: -6,
  },
  center: { textAlign: 'center' },
  button: { alignSelf: 'stretch', marginTop: spacing.sm },
});
