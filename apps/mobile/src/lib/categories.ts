import type { IconName } from '@/components/Icon';

import type { Category } from '@/api/types';
import type { PickerOption } from '@/components/ui';


export function categoryOptions(cats: Category[] | undefined): PickerOption[] {
  const kindLabel: Record<string, string> = {
    CATEGORY_KIND_NEEDS: 'Needs',
    CATEGORY_KIND_WANTS: 'Wants',
    CATEGORY_KIND_SAVINGS: 'Savings',
    CATEGORY_KIND_INCOME: 'Income',
  };
  return (cats ?? []).map((cat) => ({
    value: cat.id,
    label: cat.name,
    hint: kindLabel[cat.kind],
  }));
}

export interface CategoryMeta {
  /** Identity color, light and dark steps. */
  light: string;
  dark: string;
  icon: IconName;
}

// Slots 1–8 follow a CVD-validated categorical order (blue, orange, aqua, yellow,
// magenta, green, violet, red); color follows the category, never its rank.
const META: Record<string, CategoryMeta> = {
  transport: { light: '#2A78D6', dark: '#3987E5', icon: 'car-sport' },
  dining: { light: '#EB6834', dark: '#D95926', icon: 'restaurant' },
  groceries: { light: '#1BAF7A', dark: '#199E70', icon: 'cart' },
  housing: { light: '#EDA100', dark: '#C98500', icon: 'home' },
  shopping: { light: '#E87BA4', dark: '#D55181', icon: 'bag-handle' },
  entertainment: { light: '#008300', dark: '#0A9A0A', icon: 'film' },
  subscriptions: { light: '#4A3AA7', dark: '#9085E9', icon: 'tv' },
  health: { light: '#E34948', dark: '#E66767', icon: 'medkit' },
  utilities: { light: '#0891B2', dark: '#22A6C4', icon: 'flash' },
  insurance: { light: '#64748B', dark: '#8A97AD', icon: 'shield-checkmark' },
  travel: { light: '#0EA5E9', dark: '#38BDF8', icon: 'airplane' },
  savings: { light: '#16A34A', dark: '#22C55E', icon: 'wallet' },
  investments: { light: '#7C3AED', dark: '#A78BFA', icon: 'trending-up' },
  income: { light: '#047857', dark: '#34D399', icon: 'cash' },
  transfer: { light: '#8B8FA3', dark: '#7D8199', icon: 'swap-horizontal' },
  other: { light: '#8B8FA3', dark: '#7D8199', icon: 'ellipsis-horizontal' },
};

const FALLBACK: CategoryMeta = { light: '#8B8FA3', dark: '#7D8199', icon: 'pricetag' };

/** Color + icon for a finance category id (server ids are stable slugs). */
export function categoryMeta(id: string | undefined): CategoryMeta {
  return (id && META[id]) || FALLBACK;
}

export function categoryColor(id: string | undefined, dark: boolean): string {
  const m = categoryMeta(id);
  return dark ? m.dark : m.light;
}
