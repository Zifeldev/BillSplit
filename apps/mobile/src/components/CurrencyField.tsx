import React, { useMemo } from 'react';

import { useCurrencies } from '@/api/queries';
import { COMMON_CURRENCIES } from '@/lib/format';

import { PickerOption, SelectField } from './ui';

/**
 * Currency picker. Uses FinanceService/ListCurrencies (public — works before
 * login) when `remote` is true, otherwise / on failure a short hardcoded list.
 */
export function CurrencyField({
  label = 'Currency',
  value,
  onChange,
  remote = true,
  allowEmpty,
  emptyLabel = 'Default',
}: {
  label?: string;
  value: string;
  onChange: (code: string) => void;
  remote?: boolean;
  allowEmpty?: boolean;
  emptyLabel?: string;
}) {
  const q = useCurrencies(remote);
  const options = useMemo<PickerOption[]>(() => {
    const list: PickerOption[] =
      remote && q.data && q.data.currencies.length > 0
        ? q.data.currencies.map((cur) => ({ value: cur.code, label: cur.code, hint: cur.name }))
        : COMMON_CURRENCIES.map((code) => ({ value: code, label: code }));
    if (value && !list.some((o) => o.value === value)) list.unshift({ value, label: value });
    return allowEmpty ? [{ value: '', label: emptyLabel }, ...list] : list;
  }, [q.data, remote, value, allowEmpty, emptyLabel]);
  return (
    <SelectField
      label={label}
      value={value}
      placeholder={allowEmpty ? emptyLabel : 'Select currency'}
      options={options}
      onChange={onChange}
      searchable
    />
  );
}
