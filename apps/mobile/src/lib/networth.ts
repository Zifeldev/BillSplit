import type { AccountKind } from '@/api/types';
import type { IconName } from '@/components/Icon';
import { tone } from '@/theme';

export const ACCOUNT_KINDS: { value: AccountKind; label: string; icon: IconName; color: string; hint: string }[] = [
  { value: 'ACCOUNT_KIND_CARD', label: 'Card', icon: 'card', color: tone.slate, hint: 'Debit card or current account' },
  { value: 'ACCOUNT_KIND_CASH', label: 'Cash', icon: 'cash', color: tone.forest, hint: 'Money in your wallet' },
  { value: 'ACCOUNT_KIND_SAVINGS', label: 'Savings', icon: 'wallet', color: tone.pine, hint: 'Savings account without a fixed term' },
  { value: 'ACCOUNT_KIND_CREDIT_CARD', label: 'Credit card', icon: 'card-outline', color: tone.plum, hint: 'Enter how much you owe' },
  { value: 'ACCOUNT_KIND_LOAN', label: 'Loan', icon: 'document-text', color: tone.clay, hint: 'Remaining principal of a loan or mortgage' },
  { value: 'ACCOUNT_KIND_PROPERTY', label: 'Property', icon: 'home', color: tone.ochre, hint: 'Car, apartment — estimated value' },
];

export function accountMeta(k: AccountKind) {
  return ACCOUNT_KINDS.find((x) => x.value === k) ?? ACCOUNT_KINDS[0];
}
