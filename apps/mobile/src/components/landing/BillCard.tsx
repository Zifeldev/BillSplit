import { Icon } from '@/components/Icon';
import React from 'react';
import { StyleSheet, View } from 'react-native';

import { frosted } from '@/components/Glass';
import { Txt } from '@/components/ui';
import { fonts, radius, spacing, useTheme } from '@/theme';

interface Share {
  name: string;
  note: string;
  amount?: string;
  payer?: boolean;
  settled?: boolean;
}

const SHARES: Share[] = [
  { name: 'Alena', note: 'Paid the full bill', amount: 'gets back $139.80', payer: true },
  { name: 'Sofia', note: 'Share of 4', amount: '$46.60' },
  { name: 'Azat', note: 'Share of 4', amount: '$46.60' },
  { name: 'Dilnaz', note: 'Share of 4', settled: true },
];

function ShareRow({ share, index }: { share: Share; index: number }) {
  const { c } = useTheme();
  // Avatars fade from the brand green to pale sage down the list, as on a printed tab.
  const fills = [c.accent, c.outline, c.primarySoft, c.primarySoft];
  const fill = fills[Math.min(index, fills.length - 1)];
  return (
    <View style={styles.row}>
      <View style={[styles.avatar, { backgroundColor: fill }]}>
        <Txt weight="600" color={share.payer ? c.primaryText : c.primaryStrong}>
          {share.name[0]}
        </Txt>
      </View>
      <View style={styles.flex1}>
        <Txt weight="600" style={styles.name}>
          {share.name}
        </Txt>
        <Txt variant="small">{share.note}</Txt>
      </View>
      {share.settled ? (
        <View style={[styles.settled, { backgroundColor: c.primarySoft }]}>
          <Icon name="checkmark" size={14} color={c.primaryStrong} />
          <Txt variant="small" weight="600" color={c.primaryStrong}>
            Settled
          </Txt>
        </View>
      ) : (
        <Txt weight="600" color={share.payer ? c.primaryStrong : c.text}>
          {share.amount}
        </Txt>
      )}
    </View>
  );
}

/** Illustration of a split bill for the landing and sign-in pages; not interactive. */
export function BillCard() {
  const { c } = useTheme();
  return (
    <View
      aria-hidden
      importantForAccessibility="no-hide-descendants"
      style={[styles.card, frosted(c, true)]}>
      <View style={styles.head}>
        <View style={styles.flex1}>
          <Txt variant="small" weight="500">
            Friday dinner
          </Txt>
          <Txt variant="h2" style={styles.title}>
            Dinner at Luna&apos;s
          </Txt>
        </View>
        <View style={styles.total}>
          <Txt variant="small" weight="500">
            Total
          </Txt>
          <Txt variant="big" color={c.primary} style={styles.totalValue}>
            $186.40
          </Txt>
        </View>
      </View>
      <View style={[styles.rule, { backgroundColor: c.border }]} />
      <View style={styles.rows}>
        {SHARES.map((share, i) => (
          <ShareRow key={share.name} share={share} index={i} />
        ))}
      </View>
      <View style={[styles.footer, { backgroundColor: c.accent }]}>
        <Txt weight="600" color={c.primaryText}>
          2 payments left to settle
        </Txt>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1, minWidth: 0 },
  card: {
    width: '100%',
    maxWidth: 460,
    borderRadius: radius.xl,
    borderWidth: 1,
    padding: 28,
    gap: 22,
    boxShadow: '0px 24px 60px rgba(31, 74, 55, 0.14)',
  },
  head: { flexDirection: 'row', alignItems: 'flex-start', gap: spacing.md },
  title: { fontSize: 22, lineHeight: 27, marginTop: 2 },
  total: { alignItems: 'flex-end' },
  totalValue: { fontFamily: fonts.display, fontSize: 26, lineHeight: 31 },
  rule: { height: 1 },
  rows: { gap: 14 },
  row: { flexDirection: 'row', alignItems: 'center', gap: spacing.md },
  avatar: { width: 40, height: 40, borderRadius: 20, alignItems: 'center', justifyContent: 'center' },
  name: { fontSize: 16 },
  settled: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: radius.pill,
  },
  footer: { alignItems: 'center', paddingVertical: 15, borderRadius: radius.md },
});
