import { Tabs } from 'expo-router/js-tabs';
import React from 'react';
import { AppearanceButton } from '@/components/Appearance';
import { HeaderGlass } from '@/components/Glass';
import { GlassTabBar, type GlassTabBarProps } from '@/components/GlassTabBar';
import { InboxBell } from '@/components/InboxBell';
import { ThemeToggle } from '@/components/ThemeToggle';
import { Row } from '@/components/ui';
import { fonts, spacing, useIsWide, useTheme } from '@/theme';

const HEADER_TITLE_SIZE = 24;
const HEADER_TITLE_SIZE_WIDE = 30;

function renderGlassTabBar(props: GlassTabBarProps) {
  return <GlassTabBar {...props} />;
}

function renderThemeToggle() {
  return (
    <Row gap={0}>
      <AppearanceButton />
      <ThemeToggle />
    </Row>
  );
}

function renderGroupsActions() {
  return (
    <Row gap={0}>
      <AppearanceButton />
      <ThemeToggle />
      <InboxBell />
    </Row>
  );
}

function renderHeaderGlass() {
  return <HeaderGlass />;
}

/** On wide windows the root layout shows the sidebar, so the tab bar is not rendered at all. */
function renderNoTabBar() {
  return null;
}

export default function TabsLayout() {
  const { c } = useTheme();
  const wide = useIsWide();
  return (
    <Tabs
      tabBar={wide ? renderNoTabBar : renderGlassTabBar}
      screenOptions={{
        headerTransparent: true,
        headerBackground: renderHeaderGlass,
        headerRight: renderThemeToggle,
        headerRightContainerStyle: { paddingRight: spacing.sm },
        headerTintColor: c.text,
        headerTitleStyle: {
          fontFamily: fonts.display,
          fontSize: wide ? HEADER_TITLE_SIZE_WIDE : HEADER_TITLE_SIZE,
          letterSpacing: -0.6,
        },
        headerTitleContainerStyle: wide ? { paddingLeft: spacing.lg } : undefined,
        headerTitleAlign: 'left',
        headerShadowVisible: false,
        sceneStyle: { backgroundColor: c.bg },
      }}>
      <Tabs.Screen name="index" options={{ title: 'Groups', headerRight: renderGroupsActions }} />
      <Tabs.Screen name="friends" options={{ title: 'Friends' }} />
      <Tabs.Screen name="add" options={{ title: 'Add' }} />
      <Tabs.Screen name="finance" options={{ title: 'Finance' }} />
      <Tabs.Screen name="settings" options={{ title: 'Settings' }} />
    </Tabs>
  );
}
