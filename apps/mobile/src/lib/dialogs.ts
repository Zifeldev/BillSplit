import { Alert, Platform } from 'react-native';

/** Cross-platform confirm (Alert buttons are not supported on web). */
export function confirm(title: string, message: string, confirmText = 'OK', destructive = false): Promise<boolean> {
  if (Platform.OS === 'web') {
    const ok = typeof window !== 'undefined' ? window.confirm(`${title}\n\n${message}`) : false;
    return Promise.resolve(ok);
  }
  return new Promise((resolve) => {
    Alert.alert(title, message, [
      { text: 'Cancel', style: 'cancel', onPress: () => resolve(false) },
      { text: confirmText, style: destructive ? 'destructive' : 'default', onPress: () => resolve(true) },
    ], { cancelable: true, onDismiss: () => resolve(false) });
  });
}

export function notify(title: string, message?: string): void {
  if (Platform.OS === 'web') {
    if (typeof window !== 'undefined') window.alert(message ? `${title}\n\n${message}` : title);
    return;
  }
  Alert.alert(title, message);
}
