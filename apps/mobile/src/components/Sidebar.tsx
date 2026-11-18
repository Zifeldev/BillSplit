import { frosted } from '@/components/Glass';
import { Icon } from '@/components/Icon';
import { usePathname, useRouter, type Href } from 'expo-router';
import React, { useState } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { useNotifications } from '@/api/queries';
import { useAuth } from '@/auth/AuthContext';
import { radius, SIDEBAR_WIDTH, spacing, useTheme } from '@/theme';

import { Logo } from './Logo';
import { QuickAddSheet } from './QuickAdd';
import { Avatar, Button, IconName, Txt } from './ui';

interface NavItem {
  key: string;
  href: Href;
  icon: IconName;
  activeIcon: IconName;
  label: string;
  /** Path prefixes of the screens that belong to this section. */
  owns: string[];
}

const NAV: NavItem[] = [
  { key: 'groups', href: '/', icon: 'people-outline', activeIcon: 'people', label: 'Groups', owns: ['/group', '/receipt'] },
  { key: 'friends', href: '/friends', icon: 'heart-outline', activeIcon: 'heart', label: 'Friends', owns: ['/friends', '/person'] },
  { key: 'finance', href: '/finance', icon: 'pie-chart-outline', activeIcon: 'pie-chart', label: 'Finance', owns: ['/finance'] },
  { key: 'settings', href: '/settings', icon: 'settings-outline', activeIcon: 'settings', label: 'Settings', owns: ['/settings'] },
  {
    key: 'notifications',
    href: '/notifications',
    icon: 'notifications-outline',
    activeIcon: 'notifications',
    label: 'Notifications',
    owns: ['/notifications'],
  },
];

const MAX_BADGE = 99;
const ON_DANGER = '#FFFFFF';

function isActive(item: NavItem, pathname: string): boolean {
  if (pathname === '/') return item.href === '/';
  return item.owns.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}

function NavRow({
  item,
  active,
  onPress,
  badge,
}: {
  item: NavItem;
  active: boolean;
  onPress: () => void;
  badge?: number;
}) {
  const { c } = useTheme();
  const tint = active ? c.primaryStrong : c.textMuted;
  return (
    <Pressable
      accessibilityRole="link"
      accessibilityState={{ selected: active }}
      aria-current={active ? 'page' : undefined}
      onPress={onPress}
      style={({ pressed, hovered }: { pressed: boolean; hovered?: boolean }) => [
        styles.navRow,
        active ? { backgroundColor: c.primarySoft } : hovered ? { backgroundColor: c.cardAlt } : null,
        pressed && styles.pressed,
      ]}>
      <Icon name={active ? item.activeIcon : item.icon} size={20} color={tint} />
      <Txt weight={active ? '600' : '500'} color={active ? c.primaryStrong : c.text} style={styles.flex1}>
        {item.label}
      </Txt>
      {badge ? (
        <View style={[styles.badge, { backgroundColor: c.dangerSolid }]}>
          <Txt variant="small" weight="700" color={ON_DANGER}>
            {badge > MAX_BADGE ? `${MAX_BADGE}+` : badge}
          </Txt>
        </View>
      ) : null}
    </Pressable>
  );
}

/** Desktop navigation: sits beside every signed-in screen and replaces the bottom tab bar. */
export function Sidebar() {
  const { c } = useTheme();
  const router = useRouter();
  const { user } = useAuth();
  const [adding, setAdding] = useState(false);
  const unread = useNotifications(true).data?.unreadCount ?? 0;
  const pathname = usePathname();

  return (
    <View style={[styles.sidebar, frosted(c, true), styles.sidebarEdge]}>
      <View style={styles.brand}>
        <Logo size={34} />
      </View>
      <Button title="Add" icon="add" onPress={() => setAdding(true)} />
      <View role="navigation" style={styles.nav}>
        {NAV.map((item) => (
          <NavRow
            key={item.key}
            item={item}
            active={isActive(item, pathname)}
            badge={item.key === 'notifications' ? unread : undefined}
            onPress={() => router.navigate(item.href)}
          />
        ))}
      </View>
      <View style={styles.flex1} />
      {user ? (
        <View style={[styles.user, { borderColor: c.border }]}>
          <Avatar name={user.name} id={user.id} size={38} />
          <View style={styles.flex1}>
            <Txt weight="600" numberOfLines={1}>
              {user.name}
            </Txt>
            <Txt variant="small" numberOfLines={1}>
              {user.email}
            </Txt>
          </View>
        </View>
      ) : null}
      <QuickAddSheet visible={adding} onClose={() => setAdding(false)} />
    </View>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  sidebar: {
    width: SIDEBAR_WIDTH,
    padding: spacing.lg,
    gap: spacing.lg,
  },
  /** Only the inner edge keeps the glass rim. */
  sidebarEdge: { borderTopWidth: 0, borderBottomWidth: 0, borderLeftWidth: 0 },
  brand: { paddingHorizontal: spacing.sm, paddingTop: spacing.sm, paddingBottom: spacing.sm },
  nav: { gap: 2 },
  navRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    minHeight: 46,
    paddingHorizontal: spacing.lg,
    borderRadius: radius.pill,
  },
  pressed: { opacity: 0.7 },
  badge: { minWidth: 22, height: 22, borderRadius: 11, paddingHorizontal: 6, alignItems: 'center', justifyContent: 'center' },
  user: { flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingTop: spacing.lg, borderTopWidth: 1 },
});
