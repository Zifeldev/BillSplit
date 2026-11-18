import React from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { tapFeedback } from '@/lib/haptics';
import { fonts, radius, spacing, TAB_BAR_GAP, TAB_BAR_HEIGHT, useTheme } from '@/theme';

import { Glass } from './Glass';
import { Icon, type IconName } from './Icon';
import { QuickAddTabButton } from './QuickAdd';
import { Txt } from './ui';

interface TabItem {
  icon: IconName;
  activeIcon: IconName;
  label: string;
}

/** Tab route name → item. The `add` route is the "+" button in the middle. */
const TABS: Record<string, TabItem> = {
  index: { icon: 'people-outline', activeIcon: 'people', label: 'Groups' },
  friends: { icon: 'heart-outline', activeIcon: 'heart', label: 'Friends' },
  finance: { icon: 'pie-chart-outline', activeIcon: 'pie-chart', label: 'Finance' },
  settings: { icon: 'settings-outline', activeIcon: 'settings', label: 'Settings' },
};
const ADD_ROUTE = 'add';

const BAR_MAX_WIDTH = 440;

/** The slice of the navigator's tab bar props this bar needs. */
export interface GlassTabBarProps {
  state: { index: number; routes: { key: string; name: string }[] };
  navigateToTab: (routeKey: string) => void;
  insets: { bottom: number };
}

/** Floating Liquid Glass tab bar: content scrolls underneath it. */
export function GlassTabBar({ state, navigateToTab, insets }: GlassTabBarProps) {
  const { c } = useTheme();
  const activeKey = state.routes[state.index]?.key;
  return (
    <View pointerEvents="box-none" style={[styles.dock, { paddingBottom: insets.bottom + TAB_BAR_GAP }]}>
      <Glass style={styles.bar}>
        <View accessibilityRole="tablist" style={styles.tabs}>
          {state.routes.map((route, index) => {
            if (route.name === ADD_ROUTE) return <QuickAddTabButton key={route.key} />;
            const tab = TABS[route.name];
            if (!tab) return null;
            const active = route.key === activeKey;
            const tint = active ? c.primary : c.textMuted;
            return (
              <Pressable
                key={route.key}
                accessibilityRole="tab"
                accessibilityState={{ selected: active }}
                accessibilityLabel={`${tab.label}, tab ${index + 1} of ${state.routes.length}`}
                onPress={() => {
                  tapFeedback();
                  if (!active) navigateToTab(route.key);
                }}
                style={({ pressed }) => [styles.tab, pressed && styles.pressed]}>
                <View style={[styles.bubble, active && { backgroundColor: c.primarySoft }]}>
                  <Icon name={active ? tab.activeIcon : tab.icon} size={24} color={tint} />
                </View>
                <Txt variant="small" color={tint} style={styles.label} numberOfLines={1}>
                  {tab.label}
                </Txt>
              </Pressable>
            );
          })}
        </View>
      </Glass>
    </View>
  );
}

const styles = StyleSheet.create({
  dock: {
    position: 'absolute',
    left: 0,
    right: 0,
    bottom: 0,
    alignItems: 'center',
    paddingHorizontal: spacing.lg,
  },
  bar: {
    width: '100%',
    maxWidth: BAR_MAX_WIDTH,
    height: TAB_BAR_HEIGHT,
    borderRadius: radius.pill,
    paddingHorizontal: spacing.sm,
    boxShadow: '0px 12px 32px rgba(4, 30, 21, 0.18)',
  },
  tabs: { flex: 1, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  tab: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 1, minHeight: 52 },
  bubble: { width: 52, height: 30, borderRadius: radius.pill, alignItems: 'center', justifyContent: 'center' },
  label: { fontFamily: fonts.semibold, fontSize: 10.5, lineHeight: 13 },
  pressed: { opacity: 0.6 },
});
