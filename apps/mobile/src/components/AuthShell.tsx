import { Link } from 'expo-router';
import React from 'react';
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { fonts, heroGradient, onHero, spacing, useIsWide, useTheme } from '@/theme';

import { AppearanceButton } from './Appearance';
import { Backdrop } from './Glass';
import { BillCard } from './landing/BillCard';
import { Logo } from './Logo';
import { ThemeToggle } from './ThemeToggle';
import { Card, Txt } from './ui';

const FORM_MAX_WIDTH = 440;
const PANEL_RADIUS = 36;

/** Brand panel shown beside the form on desktop-sized windows. */
function BrandPanel() {
  const { c } = useTheme();
  return (
    <View style={[styles.panel, heroGradient(c.hero)]}>
      <Txt variant="title" color={onHero.text} style={styles.panelTitle}>
        Split the bill, not the friendship.
      </Txt>
      <Txt color={onHero.soft} style={styles.panelLead}>
        Scan the receipt, let everyone tap what they had, and settle in the fewest payments.
      </Txt>
      <View style={styles.panelArt}>
        <BillCard />
      </View>
    </View>
  );
}

export function AuthShell({ title, subtitle, children }: { title: string; subtitle: string; children: React.ReactNode }) {
  const { c } = useTheme();
  const wide = useIsWide();
  return (
    <SafeAreaView style={[styles.fill, { backgroundColor: c.bg }]}>
      <Backdrop />
      <View style={[styles.fill, wide && styles.split]}>
        {wide ? <BrandPanel /> : null}
        <KeyboardAvoidingView style={styles.fill} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
          <ScrollView contentContainerStyle={[styles.scroll, wide && styles.scrollWide]} keyboardShouldPersistTaps="handled">
            <View style={styles.inner}>
              <View style={styles.top}>
                {/* On the web the logo leads back to the landing page. */}
                {Platform.OS === 'web' ? (
                  <Link href="/" accessibilityLabel="billsplit home">
                    <Logo />
                  </Link>
                ) : (
                  <Logo />
                )}
                <View style={styles.actions}>
                  <AppearanceButton />
                  <ThemeToggle />
                </View>
              </View>
              <View style={styles.heading}>
                <Txt variant="title" style={styles.title}>
                  {title}
                </Txt>
                <Txt variant="body" color={c.textMuted}>
                  {subtitle}
                </Txt>
              </View>
              <Card style={styles.card}>
                <View style={styles.form}>{children}</View>
              </Card>
            </View>
          </ScrollView>
        </KeyboardAvoidingView>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1 },
  split: { flexDirection: 'row' },
  scroll: { flexGrow: 1, padding: spacing.xl, paddingTop: spacing.xxl },
  scrollWide: { justifyContent: 'center' },
  inner: { width: '100%', maxWidth: FORM_MAX_WIDTH, alignSelf: 'center', gap: spacing.xl },
  actions: { flexDirection: 'row', alignItems: 'center' },
  top: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  heading: { gap: spacing.xs, marginTop: spacing.sm },
  title: { fontFamily: fonts.display, fontSize: 32, lineHeight: 36, letterSpacing: -1 },
  card: { padding: spacing.xl },
  form: { gap: spacing.lg },
  panel: {
    flex: 1,
    margin: spacing.xl,
    marginRight: 0,
    borderRadius: PANEL_RADIUS,
    padding: 48,
    gap: spacing.lg,
    overflow: 'hidden',
  },
  panelTitle: { fontSize: 44, lineHeight: 47, letterSpacing: -1.5, maxWidth: 460 },
  panelLead: { fontSize: 18, lineHeight: 28, maxWidth: 440 },
  panelArt: { flex: 1, justifyContent: 'flex-end', alignItems: 'flex-start', marginTop: spacing.xl },
});
