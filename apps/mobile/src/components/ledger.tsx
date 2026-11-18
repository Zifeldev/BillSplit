import { Icon } from '@/components/Icon';
import React, { useState } from 'react';
import { Modal, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import type { FriendLedger, LedgerEntry, LedgerGroup } from '@/api/types';
import { useMountProgress } from '@/lib/anim';
import { groupCover } from '@/lib/colors';
import { formatDate, formatMoney, formatSigned } from '@/lib/format';
import { tapFeedback } from '@/lib/haptics';
import { MAX_WIDTH, radius, spacing, tone, useTheme } from '@/theme';

import { Badge, Banner, Button, Card, IconButton, IconName, IconTile, ListRow, Money, Row, Txt } from './ui';
import { frosted } from './Glass';

// ---------- entries ----------
const KIND: Record<string, { icon: IconName; color: string; label: string }> = {
  LEDGER_ENTRY_KIND_RECEIPT: { icon: 'receipt', color: tone.forest, label: 'Receipt' },
  LEDGER_ENTRY_KIND_EXPENSE: { icon: 'cash', color: tone.ochre, label: 'Expense' },
  LEDGER_ENTRY_KIND_SETTLEMENT: { icon: 'swap-horizontal', color: tone.pine, label: 'Payment' },
  LEDGER_ENTRY_KIND_UNSPECIFIED: { icon: 'ellipse', color: tone.stone, label: 'Entry' },
};

function kindOf(e: LedgerEntry) {
  return KIND[e.kind] ?? KIND.LEDGER_ENTRY_KIND_UNSPECIFIED;
}

/** Whether the entry links to a receipt / expense screen. */
export function entryOpensReceipt(e: LedgerEntry): boolean {
  return !!e.receiptId && e.kind !== 'LEDGER_ENTRY_KIND_SETTLEMENT';
}

function EntryRow({
  entry,
  currency,
  friend,
  divider,
  onOpen,
}: {
  entry: LedgerEntry;
  currency: string;
  friend: string;
  divider: boolean;
  onOpen: (receiptId: string) => void;
}) {
  const { c } = useTheme();
  const k = kindOf(entry);
  const settlement = entry.kind === 'LEDGER_ENTRY_KIND_SETTLEMENT';
  const who = settlement ? (entry.amountCents < 0 ? `${friend} paid you` : 'You paid') : k.label;
  const date = formatDate(entry.occurredAt);
  const opens = entryOpensReceipt(entry);
  return (
    <ListRow
      divider={divider}
      onPress={opens ? () => onOpen(entry.receiptId) : undefined}
      left={<IconTile icon={k.icon} color={k.color} size={38} solid />}
      title={entry.title || k.label}
      subtitle={date ? `${who} · ${date}` : who}
      right={
        <Row gap={4}>
          <Money
            cents={entry.amountCents}
            currency={currency}
            signed
            colored={!settlement}
            color={settlement ? c.textMuted : undefined}
          />
          {opens ? <Icon name="chevron-forward" size={16} color={c.textMuted} /> : null}
        </Row>
      }
    />
  );
}

const PREVIEW = 5;

/** A common group with its net and expandable history. */
export function LedgerGroupCard({
  group,
  homeCurrency,
  friend,
  initiallyOpen,
  onOpenReceipt,
  onOpenGroup,
}: {
  group: LedgerGroup;
  homeCurrency: string;
  friend: string;
  initiallyOpen: boolean;
  onOpenReceipt: (receiptId: string) => void;
  onOpenGroup: (groupId: string) => void;
}) {
  const { c } = useTheme();
  const [open, setOpen] = useState(initiallyOpen);
  const [all, setAll] = useState(false);
  const cover = groupCover(group.groupId, group.groupName);
  const net = group.netCents;
  const converted = group.currency !== homeCurrency && net !== 0;
  const shown = all ? group.entries : group.entries.slice(0, PREVIEW);
  const n = group.entries.length;
  return (
    <Card padded={false}>
      <Pressable
        accessibilityRole="button"
        accessibilityState={{ expanded: open }}
        accessibilityLabel={`${group.groupName}, ${net > 0 ? `${friend} owes you` : net < 0 ? `you owe ${friend}` : 'settled'} ${formatMoney(Math.abs(net), group.currency)}`}
        onPress={() => {
          tapFeedback();
          setOpen((v) => !v);
        }}
        style={({ pressed }) => [styles.groupHead, pressed && { backgroundColor: c.cardAlt }]}>
        <IconTile icon={cover.icon} color={cover.color} solid size={46} />
        <View style={styles.flex1}>
          <Txt variant="h3" numberOfLines={1}>
            {group.groupName || 'Group'}
          </Txt>
          <Txt variant="small" numberOfLines={1}>
            {n} {n === 1 ? 'entry' : 'entries'} · {group.currency}
          </Txt>
        </View>
        <View style={styles.groupRight}>
          {net === 0 ? (
            <Badge label="settled" tone="muted" icon="checkmark" />
          ) : (
            <View style={[styles.pill, { backgroundColor: net > 0 ? c.positiveSoft : c.negativeSoft }]}>
              <Money cents={net} currency={group.currency} signed colored />
            </View>
          )}
          {converted ? (
            <Txt variant="small" style={styles.tabular}>
              ≈ {formatSigned(group.netHomeCents, homeCurrency)}
            </Txt>
          ) : null}
        </View>
        <Icon name={open ? 'chevron-up' : 'chevron-down'} size={18} color={c.textMuted} />
      </Pressable>
      {open ? (
        <View style={[styles.groupBody, { borderColor: c.border }]}>
          {n === 0 ? (
            <Txt variant="muted" style={styles.emptyEntries}>
              No entries between you two yet.
            </Txt>
          ) : (
            shown.map((e, i) => (
              <EntryRow
                key={`${e.receiptId || 'p'}-${e.occurredAt ?? ''}-${i}`}
                entry={e}
                currency={group.currency}
                friend={friend}
                divider={i > 0}
                onOpen={onOpenReceipt}
              />
            ))
          )}
          <Row gap={spacing.sm} style={styles.groupFoot}>
            {n > PREVIEW ? (
              <Button
                small
                variant="secondary"
                icon={all ? 'chevron-up' : 'list'}
                title={all ? 'Show less' : `Show all ${n}`}
                onPress={() => setAll((v) => !v)}
                style={styles.flex1}
              />
            ) : null}
            <Button
              small
              variant="ghost"
              icon="open-outline"
              title="Open group"
              onPress={() => onOpenGroup(group.groupId)}
              style={styles.flex1}
            />
          </Row>
        </View>
      ) : null}
    </Card>
  );
}

// ---------- netting ----------
/** "Flat −$30 + Trip +$10 = −$20": how groups offset each other (home currency). */
export function NettingCard({ ledger }: { ledger: FriendLedger }) {
  const { c } = useTheme();
  const open = ledger.groups.filter((g) => g.netHomeCents !== 0 || g.netCents !== 0);
  const p = useMountProgress(ledger.netCents, 900);
  if (open.length < 2) return null;
  const max = Math.max(1, ...open.map((g) => Math.abs(g.netHomeCents)));
  return (
    <Card>
      <Row gap={spacing.sm}>
        <IconTile icon="git-merge" color={c.primary} size={34} />
        <View style={styles.flex1}>
          <Txt variant="h3">How it nets out</Txt>
          <Txt variant="small">Debts in one group cancel out credits in another.</Txt>
        </View>
      </Row>
      {open.map((g) => {
        const cover = groupCover(g.groupId, g.groupName);
        const pos = g.netHomeCents >= 0;
        const w = `${Math.max(4, (Math.abs(g.netHomeCents) / max) * 100 * p)}%` as const;
        return (
          <View key={g.groupId} style={styles.netRow}>
            <Row gap={spacing.sm}>
              <Icon name={cover.icon} size={18} color={cover.color} weight="duotone" />
              <Txt weight="600" numberOfLines={1} style={styles.flex1}>
                {g.groupName}
              </Txt>
              <Money cents={g.netHomeCents} currency={ledger.currency} signed colored />
            </Row>
            <View style={[styles.track, { backgroundColor: c.cardAlt }]}>
              <View style={[styles.bar, { width: w, backgroundColor: pos ? c.positive : c.negative }]} />
            </View>
          </View>
        );
      })}
      <View style={[styles.netTotal, { borderColor: c.border }]}>
        <Row gap={spacing.sm}>
          <Txt variant="h3" style={styles.flex1}>
            Net
          </Txt>
          <Money cents={ledger.netCents} currency={ledger.currency} signed colored variant="h3" />
        </Row>
      </View>
    </Card>
  );
}

// ---------- confirm sheet ----------
/** Bottom sheet that explains a SettleWithFriend before it is recorded. */
export function SettleSheet({
  visible,
  ledger,
  friend,
  loading,
  error,
  onConfirm,
  onClose,
}: {
  visible: boolean;
  ledger: FriendLedger;
  friend: string;
  loading: boolean;
  error: string | null;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const { c } = useTheme();
  const insets = useSafeAreaInsets();
  const open = ledger.groups.filter((g) => g.netCents !== 0);
  const n = open.length;
  const net = ledger.netCents;
  const amount = formatMoney(Math.abs(net), ledger.currency);
  const title = net < 0 ? `Settle up with ${friend}?` : net > 0 ? 'Mark all as received?' : 'Close out balances?';
  const netLabel = net < 0 ? `You pay ${friend}` : net > 0 ? `${friend} pays you` : 'Nothing changes hands';
  const tone = net < 0 ? c.negativeSoft : net > 0 ? c.positiveSoft : c.cardAlt;
  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={loading ? undefined : onClose}>
        <Pressable
          onPress={() => undefined}
          style={[styles.sheet, frosted(c, true), { paddingBottom: spacing.lg + insets.bottom }]}>
          <View style={[styles.grabber, { backgroundColor: c.border }]} />
          <Row>
            <Txt variant="h2" style={styles.flex1}>
              {title}
            </Txt>
            <IconButton icon="close" onPress={onClose} label="Close" filled />
          </Row>
          <ScrollView style={styles.sheetList} contentContainerStyle={styles.sheetContent}>
            <Txt variant="muted">
              This records a settlement in {n === 1 ? 'your 1 group' : `all ${n} groups`} with {friend}, so every balance
              between you two goes to zero at once.
            </Txt>
            <View style={[styles.sheetGroups, { backgroundColor: c.cardAlt }]}>
              {open.map((g, i) => {
                const cover = groupCover(g.groupId, g.groupName);
                return (
                  <Row
                    key={g.groupId}
                    gap={spacing.sm}
                    style={[styles.sheetRow, i > 0 && { borderTopWidth: StyleSheet.hairlineWidth, borderColor: c.border }]}>
                    <IconTile icon={cover.icon} color={cover.color} solid size={32} />
                    <Txt weight="600" numberOfLines={1} style={styles.flex1}>
                      {g.groupName}
                    </Txt>
                    <View style={styles.groupRight}>
                      <Money cents={g.netCents} currency={g.currency} signed colored />
                      {g.currency !== ledger.currency ? (
                        <Txt variant="small" style={styles.tabular}>
                          ≈ {formatSigned(g.netHomeCents, ledger.currency)}
                        </Txt>
                      ) : null}
                    </View>
                  </Row>
                );
              })}
            </View>
            <View style={[styles.netBox, { backgroundColor: tone }]}>
              <Txt variant="label" color={net < 0 ? c.negative : net > 0 ? c.positive : c.textMuted}>
                {netLabel}
              </Txt>
              <Money
                cents={Math.abs(net)}
                currency={ledger.currency}
                variant="big"
                color={net < 0 ? c.negative : net > 0 ? c.positive : c.text}
              />
              {n > 1 ? (
                <Txt variant="small">
                  One payment instead of {n} — the groups offset each other.
                </Txt>
              ) : null}
            </View>
            <Row gap={spacing.sm} style={styles.note}>
              <Icon name="information-circle-outline" size={16} color={c.textMuted} />
              <Txt variant="small" style={styles.flex1}>
                Billsplit doesn&apos;t move money — confirm once it has actually been paid.
              </Txt>
            </Row>
            {error ? <Banner kind="error" message={error} /> : null}
          </ScrollView>
          <Button
            title={net < 0 ? `I paid ${amount}` : net > 0 ? `I received ${amount}` : `Close out ${n === 1 ? 'group' : `${n} groups`}`}
            icon="checkmark-done"
            loading={loading}
            onPress={onConfirm}
          />
          <Button title="Cancel" variant="ghost" disabled={loading} onPress={onClose} />
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  tabular: { fontVariant: ['tabular-nums'] },
  groupHead: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    padding: spacing.lg,
    minHeight: 72,
  },
  groupRight: { alignItems: 'flex-end', gap: 2 },
  pill: { paddingHorizontal: 10, paddingVertical: 4, borderRadius: radius.pill },
  groupBody: {
    borderTopWidth: StyleSheet.hairlineWidth,
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.md,
  },
  groupFoot: { marginTop: spacing.xs },
  emptyEntries: { paddingVertical: spacing.md },
  netRow: { gap: 6, paddingVertical: 4 },
  track: { height: 8, borderRadius: 4, overflow: 'hidden' },
  bar: { height: 8, borderRadius: 4 },
  netTotal: { borderTopWidth: StyleSheet.hairlineWidth, paddingTop: spacing.sm, marginTop: spacing.xs },
  backdrop: { flex: 1, backgroundColor: 'rgba(13,20,16,0.55)', justifyContent: 'flex-end', alignItems: 'center' },
  sheet: {
    width: '100%',
    maxWidth: MAX_WIDTH,
    borderTopLeftRadius: radius.xl,
    borderTopRightRadius: radius.xl,
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.sm,
    gap: spacing.sm,
    maxHeight: '90%',
  },
  grabber: { alignSelf: 'center', width: 40, height: 5, borderRadius: 3, marginBottom: spacing.xs },
  sheetList: { flexGrow: 0 },
  sheetContent: { gap: spacing.md, paddingBottom: spacing.sm },
  sheetGroups: { borderRadius: radius.md, paddingHorizontal: spacing.md },
  sheetRow: { paddingVertical: spacing.sm, minHeight: 48 },
  netBox: { borderRadius: radius.md, padding: spacing.lg, gap: 4, alignItems: 'center' },
  note: { alignItems: 'flex-start' },
});
