import React from 'react';
import { StyleSheet, View } from 'react-native';

import { spacing, useIsWide } from '@/theme';

export interface MasonryBlock {
  key: string;
  /** Rough height in points; only used to decide which column a block goes to. */
  weight: number;
  node: React.ReactNode;
}

const COLUMN_COUNT = 2;

/** Puts each block, in reading order, into whichever column is currently shorter. */
function balance(blocks: MasonryBlock[]): MasonryBlock[][] {
  const columns: MasonryBlock[][] = Array.from({ length: COLUMN_COUNT }, () => []);
  const heights = columns.map(() => 0);
  for (const block of blocks) {
    const shortest = heights.indexOf(Math.min(...heights));
    columns[shortest] = [...columns[shortest], block];
    heights[shortest] += block.weight;
  }
  return columns;
}

/**
 * Dashboard layout: one flow on phones, two balanced columns on desktop-sized windows.
 * Blocks are distributed by weight, so a missing section never leaves one column half empty.
 */
export function Masonry({ blocks }: { blocks: MasonryBlock[] }) {
  const wide = useIsWide();
  if (!wide) {
    return (
      <View style={styles.column}>
        {blocks.map((block) => (
          <View key={block.key} style={styles.block}>
            {block.node}
          </View>
        ))}
      </View>
    );
  }
  return (
    <View style={styles.columns}>
      {balance(blocks).map((column, i) => (
        <View key={i} style={[styles.column, styles.columnWide]}>
          {column.map((block) => (
            <View key={block.key} style={styles.block}>
              {block.node}
            </View>
          ))}
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  columns: { flexDirection: 'row', alignItems: 'flex-start', gap: spacing.xl },
  column: { gap: spacing.md },
  columnWide: { flex: 1, minWidth: 0 },
  block: { gap: spacing.md },
});
