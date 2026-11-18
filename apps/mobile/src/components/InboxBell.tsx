import { Icon } from '@/components/Icon';
import { useIsFocused, useRouter } from 'expo-router';
import React from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { useNotifications } from '@/api/queries';
import { fonts, useTheme } from '@/theme';

/** Header bell with the unread count; polls the inbox every 30s while its screen is focused. */
export function InboxBell() {
  const router = useRouter();
  const { c } = useTheme();
  const focused = useIsFocused();
  const q = useNotifications(focused);
  const unread = q.data?.unreadCount ?? 0;
  return (
    <Pressable
      onPress={() => router.push('/notifications')}
      hitSlop={10}
      accessibilityLabel={unread > 0 ? `Notifications, ${unread} unread` : 'Notifications'}
      style={({ pressed }) => [styles.wrap, pressed && { opacity: 0.6 }]}>
      <Icon name={unread > 0 ? 'notifications' : 'notifications-outline'} size={24} color={c.text} />
      {unread > 0 ? (
        <View style={[styles.badge, { backgroundColor: c.dangerSolid, borderColor: c.card }]}>
          <Text style={styles.badgeText}>{unread > 99 ? '99+' : unread}</Text>
        </View>
      ) : null}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  wrap: { paddingHorizontal: 14, paddingVertical: 4 },
  badge: {
    position: 'absolute',
    top: -2,
    right: 6,
    minWidth: 18,
    height: 18,
    borderRadius: 9,
    paddingHorizontal: 4,
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 2,
  },
  badgeText: { color: '#FFFFFF', fontFamily: fonts.bold, fontSize: 10 },
});
