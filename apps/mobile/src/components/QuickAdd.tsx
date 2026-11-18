import { Icon } from '@/components/Icon';
import { useRouter, type Href } from 'expo-router';
import React, { useState } from 'react';
import { Modal, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { useGroups } from '@/api/queries';
import { groupCover } from '@/lib/colors';
import { impactFeedback, tapFeedback } from '@/lib/haptics';
import { MAX_WIDTH, radius, spacing, useIsWide, useTheme } from '@/theme';

import { IconButton, IconName, IconTile, ListRow, Row, Txt } from './ui';
import { frosted } from './Glass';

const DIALOG_MAX_WIDTH = 480;

type GroupAction = 'scan' | 'manual' | 'expense';

interface Action {
  key: string;
  icon: IconName;
  title: string;
  subtitle: string;
  group?: GroupAction;
  href?: Href;
}

const ACTIONS: Action[] = [
  { key: 'scan', icon: 'scan', title: 'Scan receipt', subtitle: 'Photo, gallery or PDF', group: 'scan' },
  { key: 'expense', icon: 'cash', title: 'Add expense', subtitle: 'Split an amount in a group', group: 'expense' },
  { key: 'manual', icon: 'create', title: 'Enter receipt manually', subtitle: 'Type the items yourself', group: 'manual' },
  { key: 'tx', icon: 'swap-vertical', title: 'New transaction', subtitle: 'Personal income or spending', href: '/finance/new-transaction' },
  { key: 'import', icon: 'document-text', title: 'Import statement', subtitle: 'CSV, OFX or PDF', href: '/finance/import' },
  { key: 'group', icon: 'people', title: 'New group', subtitle: 'Trip, flat, dinner…', href: '/group/new' },
];

function groupHref(groupId: string, a: GroupAction): Href {
  if (a === 'expense') return `/group/${groupId}/add-expense`;
  return `/group/${groupId}/add-receipt?mode=${a}`;
}

/** Bottom sheet with the "+" quick actions; asks for a group when needed. */
export function QuickAddSheet({ visible, onClose }: { visible: boolean; onClose: () => void }) {
  const { c } = useTheme();
  const wide = useIsWide();
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const groups = useGroups();
  const [pending, setPending] = useState<GroupAction | null>(null);

  const close = () => {
    setPending(null);
    onClose();
  };
  const go = (href: Href) => {
    close();
    router.push(href);
  };
  const choose = (a: Action) => {
    tapFeedback();
    if (a.href) return go(a.href);
    if (!a.group) return;
    const list = groups.data?.groups ?? [];
    if (list.length === 1) return go(groupHref(list[0].id, a.group));
    if (list.length === 0) return go('/group/new');
    setPending(a.group);
  };

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={close}>
      <Pressable style={[styles.backdrop, wide && styles.backdropWide]} onPress={close}>
        <Pressable
          onPress={() => undefined}
          style={[
            styles.sheet,
            wide && styles.dialog,
            frosted(c, true),
            { paddingBottom: spacing.lg + insets.bottom },
          ]}>
          {wide ? null : <View style={[styles.grabber, { backgroundColor: c.border }]} />}
          <Row>
            {pending ? <IconButton icon="chevron-back" onPress={() => setPending(null)} label="Back" /> : null}
            <Txt variant="h2" style={styles.flex1}>
              {pending ? 'Which group?' : 'Add something'}
            </Txt>
            <IconButton icon="close" onPress={close} label="Close" filled />
          </Row>
          <ScrollView style={styles.list} contentContainerStyle={styles.listContent}>
            {pending
              ? (groups.data?.groups ?? []).map((g) => {
                  const cover = groupCover(g.id, g.name);
                  return (
                    <ListRow
                      key={g.id}
                      style={styles.row}
                      left={<IconTile icon={cover.icon} color={cover.color} solid size={44} />}
                      title={g.name}
                      subtitle={`${(g.members ?? []).length} members · ${g.currency}`}
                      right={<Icon name="chevron-forward" size={18} color={c.textMuted} />}
                      onPress={() => go(groupHref(g.id, pending))}
                    />
                  );
                })
              : ACTIONS.map((a) => (
                  <ListRow
                    key={a.key}
                    style={styles.row}
                    left={<IconTile icon={a.icon} color={c.primary} size={44} />}
                    title={a.title}
                    subtitle={a.subtitle}
                    right={<Icon name="chevron-forward" size={18} color={c.textMuted} />}
                    onPress={() => choose(a)}
                  />
                ))}
          </ScrollView>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

/** Glowing "+" button in the middle of the tab bar. */
export function QuickAddTabButton() {
  const { c } = useTheme();
  const [open, setOpen] = useState(false);
  return (
    <View style={styles.tabSlot}>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel="Add receipt, expense or transaction"
        onPress={() => {
          impactFeedback();
          setOpen(true);
        }}
        style={({ pressed }) => [
          styles.fab,
          { backgroundColor: c.accent, boxShadow: `0px 6px 20px ${c.accentGlow}` },
          pressed && styles.fabPressed,
        ]}>
        <Icon name="add" size={26} color={c.primaryText} weight="bold" />
      </Pressable>
      <QuickAddSheet visible={open} onClose={() => setOpen(false)} />
    </View>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  backdrop: { flex: 1, backgroundColor: 'rgba(13,20,16,0.55)', justifyContent: 'flex-end', alignItems: 'center' },
  sheet: {
    width: '100%',
    maxWidth: MAX_WIDTH,
    borderTopLeftRadius: radius.xl,
    borderTopRightRadius: radius.xl,
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.sm,
    gap: spacing.sm,
    maxHeight: '85%',
  },
  /** Desktop: a centered dialog instead of a bottom sheet. */
  backdropWide: { justifyContent: 'center' },
  dialog: { maxWidth: DIALOG_MAX_WIDTH, borderRadius: radius.xl, paddingTop: spacing.lg },
  grabber: { alignSelf: 'center', width: 40, height: 5, borderRadius: 3, marginBottom: spacing.xs },
  list: { flexGrow: 0 },
  listContent: { paddingBottom: spacing.sm },
  row: { paddingHorizontal: spacing.xs },
  tabSlot: { paddingHorizontal: spacing.xs, alignItems: 'center', justifyContent: 'center' },
  fab: { width: 52, height: 52, borderRadius: 26, alignItems: 'center', justifyContent: 'center' },
  fabPressed: { transform: [{ scale: 0.94 }] },
});
