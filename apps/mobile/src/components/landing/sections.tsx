import { Icon } from '@/components/Icon';
import React from 'react';
import { StyleSheet, View } from 'react-native';

import { frosted } from '@/components/Glass';
import { Logo } from '@/components/Logo';
import { Button, Txt } from '@/components/ui';
import { fonts, heroGradient, onHero, radius, spacing, useIsWide, useTheme } from '@/theme';

import { BillCard } from './BillCard';
import { FEATURES, STEPS } from './content';

export const PAGE_MAX_WIDTH = 1200;
/** Label on the white button inside the green call-to-action panel (same in both themes). */
const CTA_INK = '#1F4A37';

interface AuthActions {
  onRegister: () => void;
  onLogin: () => void;
}

/** Centers a section's content on the page grid. */
function Container({ children, style }: { children: React.ReactNode; style?: object }) {
  return <View style={[styles.container, style]}>{children}</View>;
}

function SectionTitle({ children }: { children: string }) {
  const wide = useIsWide();
  const { c } = useTheme();
  return (
    <Txt variant="title" color={c.text} style={[styles.sectionTitle, wide && styles.sectionTitleWide]}>
      {children}
    </Txt>
  );
}

export function Hero({ onRegister, onLogin }: AuthActions) {
  const { c } = useTheme();
  const wide = useIsWide();
  return (
    <View style={[styles.section, styles.heroSection]}>
      <Container style={styles.heroRow}>
        <View style={styles.heroCopy}>
          <View style={[styles.heroPill, { backgroundColor: c.primarySoft }]}>
            <Icon name="checkmark" size={16} color={c.primaryStrong} />
            <Txt variant="muted" weight="600" color={c.primaryStrong}>
              Free for friends, flatmates and trips
            </Txt>
          </View>
          <Txt variant="hero" style={[styles.h1, wide && styles.h1Wide]}>
            Split the bill, not the friendship.
          </Txt>
          <Txt color={c.textMuted} style={styles.lead}>
            Scan the receipt, let everyone tap what they had, and billsplit works out who owes what. Tax and tip
            included, no awkward math.
          </Txt>
          <View style={styles.actions}>
            <Button title="Create an account" onPress={onRegister} />
            <Button title="Log in" variant="ghost" onPress={onLogin} />
          </View>
        </View>
        <View style={styles.heroArt}>
          <BillCard />
        </View>
      </Container>
    </View>
  );
}

export function HowItWorks() {
  const { c } = useTheme();
  return (
    <View style={[styles.section, { backgroundColor: c.glass }]}>
      <Container style={styles.sectionBody}>
        <SectionTitle>Three steps from receipt to settled</SectionTitle>
        <View style={styles.grid}>
          {STEPS.map((step, i) => (
            <View key={step.title} style={[styles.step, frosted(c)]}>
              <View style={[styles.stepNumber, { backgroundColor: c.accent }]}>
                <Txt variant="h2" color={c.primaryText}>
                  {i + 1}
                </Txt>
              </View>
              <Txt variant="h2" style={styles.cardTitle}>
                {step.title}
              </Txt>
              <Txt color={c.textMuted} style={styles.cardBody}>
                {step.body}
              </Txt>
            </View>
          ))}
        </View>
      </Container>
    </View>
  );
}

export function Features() {
  const { c } = useTheme();
  return (
    <View style={styles.section}>
      <Container style={styles.sectionBody}>
        <SectionTitle>Everything a group needs to stay even</SectionTitle>
        <View style={styles.grid}>
          {FEATURES.map((feature) => (
            <View key={feature.title} style={[styles.feature, frosted(c)]}>
              <View style={[styles.featureIcon, { backgroundColor: c.primarySoft }]}>
                <Icon name={feature.icon} size={24} color={c.primaryStrong} />
              </View>
              <View style={styles.featureCopy}>
                <Txt variant="h2">{feature.title}</Txt>
                <Txt color={c.textMuted} style={styles.featureBody}>
                  {feature.body}
                </Txt>
              </View>
            </View>
          ))}
        </View>
      </Container>
    </View>
  );
}

export function CallToAction({ onRegister, onLogin }: AuthActions) {
  const { c } = useTheme();
  const wide = useIsWide();
  return (
    <View style={styles.ctaSection}>
      <Container>
        <View style={[styles.cta, heroGradient(c.hero)]}>
          <Txt variant="title" color={onHero.text} style={[styles.ctaTitle, wide && styles.sectionTitleWide]}>
            Ready for the next bill?
          </Txt>
          <Txt color={onHero.soft} style={styles.ctaBody}>
            Create a group in under a minute and invite your friends with a link.
          </Txt>
          <View style={[styles.actions, styles.ctaActions]}>
            <Button
              title="Create an account"
              onPress={onRegister}
              variant="secondary"
              style={styles.ctaPrimary}
              textColor={CTA_INK}
            />
            <Button
              title="Log in"
              onPress={onLogin}
              variant="ghost"
              style={styles.ctaSecondary}
              textColor={onHero.text}
            />
          </View>
        </View>
      </Container>
    </View>
  );
}

export function Footer() {
  const { c } = useTheme();
  return (
    <View style={[styles.footer, { borderColor: c.border }]}>
      <Container style={styles.footerRow}>
        <Logo size={30} />
        <Txt variant="muted">© 2026 billsplit</Txt>
      </Container>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { width: '100%', maxWidth: PAGE_MAX_WIDTH, alignSelf: 'center' },
  section: { paddingHorizontal: spacing.xl, paddingVertical: 72 },
  heroSection: { paddingTop: 40 },
  sectionBody: { gap: 40 },
  sectionTitle: { fontSize: 30, lineHeight: 34, letterSpacing: -0.9, maxWidth: 640 },
  sectionTitleWide: { fontSize: 44, lineHeight: 48, letterSpacing: -1.2, maxWidth: 820 },
  heroRow: { flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: 56 },
  heroCopy: { flexGrow: 1, flexShrink: 1, flexBasis: 440, minWidth: 0, gap: spacing.xl, alignItems: 'flex-start' },
  heroArt: { flexGrow: 1, flexShrink: 1, flexBasis: 360, minWidth: 0, alignItems: 'center' },
  heroPill: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    paddingHorizontal: spacing.lg,
    paddingVertical: spacing.sm,
    borderRadius: radius.pill,
  },
  h1: { fontFamily: fonts.display, fontSize: 40, lineHeight: 43, letterSpacing: -1.4 },
  h1Wide: { fontSize: 64, lineHeight: 67, letterSpacing: -1.8 },
  lead: { fontSize: 19, lineHeight: 30, maxWidth: 520 },
  actions: { flexDirection: 'row', flexWrap: 'wrap', gap: 14 },
  grid: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing.xl },
  step: { flexGrow: 1, flexShrink: 1, flexBasis: 300, minWidth: 0, borderRadius: radius.lg, padding: spacing.xxl, gap: 14 },
  stepNumber: { width: 44, height: 44, borderRadius: 14, alignItems: 'center', justifyContent: 'center' },
  cardTitle: { fontSize: 22, lineHeight: 27 },
  cardBody: { fontSize: 16, lineHeight: 26 },
  feature: {
    flexGrow: 1,
    flexShrink: 1,
    flexBasis: 340,
    minWidth: 0,
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: 18,
    borderRadius: radius.lg,
    borderWidth: 1,
    padding: 28,
  },
  featureIcon: { width: 48, height: 48, borderRadius: 14, alignItems: 'center', justifyContent: 'center' },
  featureCopy: { flex: 1, minWidth: 0, gap: 6 },
  featureBody: { lineHeight: 24 },
  ctaSection: { paddingHorizontal: spacing.xl, paddingBottom: 72 },
  cta: { borderRadius: 36, paddingVertical: 64, paddingHorizontal: spacing.xxl, alignItems: 'center', gap: 22 },
  ctaTitle: { fontSize: 30, lineHeight: 34, letterSpacing: -0.9, textAlign: 'center', maxWidth: 640 },
  ctaBody: { fontSize: 18, lineHeight: 28, textAlign: 'center', maxWidth: 520 },
  ctaActions: { justifyContent: 'center', marginTop: spacing.sm },
  ctaPrimary: { backgroundColor: '#FFFFFF', borderColor: '#FFFFFF' },
  ctaSecondary: { borderColor: 'rgba(255,255,255,0.55)' },
  footer: { paddingHorizontal: spacing.xl, paddingVertical: 40, borderTopWidth: 1 },
  footerRow: { flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 20 },
});
