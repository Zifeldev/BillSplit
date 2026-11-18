import * as Clipboard from 'expo-clipboard';
import { Platform, Share } from 'react-native';

import { notify } from './dialogs';

export async function copyText(text: string, label = 'Link copied'): Promise<void> {
  try {
    await Clipboard.setStringAsync(text);
    notify(label, text);
  } catch {
    notify('Could not copy', text);
  }
}

/** Uses the native share sheet; on web uses navigator.share when present, else copies. */
export async function shareLink(url: string, message: string): Promise<void> {
  try {
    if (Platform.OS === 'web') {
      const nav: Navigator | undefined = typeof navigator !== 'undefined' ? navigator : undefined;
      if (nav && typeof nav.share === 'function') {
        await nav.share({ title: 'Billsplit', text: message, url });
        return;
      }
      await copyText(url);
      return;
    }
    await Share.share(Platform.OS === 'ios' ? { message, url } : { message: `${message}\n${url}` });
  } catch {
    // user cancelled
  }
}

/** Shares plain text (no link): native share sheet; on web navigator.share, else copies. */
export async function shareText(message: string, title = 'Billsplit'): Promise<void> {
  try {
    if (Platform.OS === 'web') {
      const nav: Navigator | undefined = typeof navigator !== 'undefined' ? navigator : undefined;
      if (nav && typeof nav.share === 'function') {
        await nav.share({ title, text: message });
        return;
      }
      await copyText(message, 'Message copied');
      return;
    }
    await Share.share({ message });
  } catch {
    // user cancelled
  }
}
