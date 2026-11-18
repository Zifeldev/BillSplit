import React from 'react';

import { setThemePreference, useTheme } from '@/theme';

import { IconButton } from './ui';

/** One-tap switch between the light and dark theme; the three-way choice lives in Settings. */
export function ThemeToggle() {
  const { dark } = useTheme();
  return (
    <IconButton
      icon={dark ? 'sun' : 'moon'}
      label={dark ? 'Switch to light theme' : 'Switch to dark theme'}
      onPress={() => setThemePreference(dark ? 'light' : 'dark')}
    />
  );
}
