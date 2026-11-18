import { useRouter } from 'expo-router';
import React, { useRef, useState } from 'react';
import { Pressable, ScrollView, StyleSheet, View, type LayoutChangeEvent } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { AppearanceButton } from '@/components/Appearance';
import { Backdrop } from '@/components/Glass';
import { Logo } from '@/components/Logo';
import { ThemeToggle } from '@/components/ThemeToggle';
import { Button, Txt } from '@/components/ui';
import { spacing, useIsWide, useTheme } from '@/theme';

import { CallToAction, Features, Footer, Hero, HowItWorks, PAGE_MAX_WIDTH } from './sections';

type SectionKey = 'how' | 'features';

const SECTION_LINKS: { key: SectionKey; label: string }[] = [
  { key: 'how', label: 'How it works' },
  { key: 'features', label: 'Features' },
];

/** Public page for signed-out visitors on the web. */
export function Landing() {
  const { c } = useTheme();
  const router = useRouter();
  const wide = useIsWide();
  const scroll = useRef<React.ComponentRef<typeof ScrollView>>(null);
  const [offsets, setOffsets] = useState<Partial<Record<SectionKey, number>>>({});

  const onRegister = () => router.push('/register');
  const onLogin = () => router.push('/login');
  const [mainTop, setMainTop] = useState(0);
  // Section offsets are measured inside <main>; `mainTop` shifts them into scroll coordinates.
  const remember = (key: SectionKey) => (e: LayoutChangeEvent) => {
    const y = e.nativeEvent.layout.y;
    setOffsets((prev) => (prev[key] === y ? prev : { ...prev, [key]: y }));
  };
  const scrollTo = (key: SectionKey) => scroll.current?.scrollTo({ y: mainTop + (offsets[key] ?? 0), animated: true });

  return (
    <SafeAreaView style={[styles.fill, { backgroundColor: c.bg }]}>
      <Backdrop />
      <ScrollView ref={scroll} style={styles.fill}>
        <View role="banner" style={styles.navBar}>
          <View style={styles.nav}>
            <Logo />
            <View style={styles.navLinks}>
              <AppearanceButton />
              <ThemeToggle />
              {wide
                ? SECTION_LINKS.map((link) => (
                    <Pressable
                      key={link.key}
                      accessibilityRole="link"
                      onPress={() => scrollTo(link.key)}
                      style={({ pressed }) => [styles.navLink, pressed && styles.pressed]}>
                      <Txt weight="500">{link.label}</Txt>
                    </Pressable>
                  ))
                : null}
              <Pressable
                accessibilityRole="link"
                onPress={onLogin}
                style={({ pressed }) => [styles.navLink, pressed && styles.pressed]}>
                <Txt weight="500">Log in</Txt>
              </Pressable>
              {wide ? <Button title="Create an account" small onPress={onRegister} /> : null}
            </View>
          </View>
        </View>
        <View role="main" onLayout={(e) => setMainTop(e.nativeEvent.layout.y)}>
          <Hero onRegister={onRegister} onLogin={onLogin} />
          <View onLayout={remember('how')}>
            <HowItWorks />
          </View>
          <View onLayout={remember('features')}>
            <Features />
          </View>
          <CallToAction onRegister={onRegister} onLogin={onLogin} />
        </View>
        <Footer />
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1 },
  navBar: { paddingHorizontal: spacing.xl, paddingVertical: 20 },
  nav: {
    width: '100%',
    maxWidth: PAGE_MAX_WIDTH,
    alignSelf: 'center',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: spacing.lg,
  },
  navLinks: { flexDirection: 'row', alignItems: 'center', gap: spacing.xl },
  navLink: { minHeight: 44, justifyContent: 'center' },
  pressed: { opacity: 0.6 },
});
