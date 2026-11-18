import Constants, { ExecutionEnvironment } from 'expo-constants';
import * as Device from 'expo-device';
import * as Notifications from 'expo-notifications';
import { Platform } from 'react-native';

/** Show pushes as banners while the app is in the foreground. */
try {
  Notifications.setNotificationHandler({
    handleNotification: async () => ({
      shouldShowBanner: true,
      shouldShowList: true,
      shouldPlaySound: false,
      shouldSetBadge: false,
    }),
  });
} catch {
  // Not available in this runtime (e.g. Expo Go on Android).
}

function projectId(): string | undefined {
  const extra = Constants.expoConfig?.extra as { eas?: { projectId?: string } } | undefined;
  return extra?.eas?.projectId ?? Constants.easConfig?.projectId ?? undefined;
}

/**
 * Asks for permission and returns the Expo push token (ExponentPushToken[...]),
 * or null when push is unavailable: simulator/emulator, permission denied,
 * no EAS projectId, Expo Go on Android (SDK 53+), etc. Never throws.
 */
export async function registerForPushAsync(): Promise<string | null> {
  try {
    if (!Device.isDevice) return null;
    if (Platform.OS === 'android' && Constants.executionEnvironment === ExecutionEnvironment.StoreClient) {
      console.warn('[push] Remote push is not supported in Expo Go on Android — use a development build.');
      return null;
    }
    if (Platform.OS === 'android') {
      await Notifications.setNotificationChannelAsync('default', {
        name: 'Default',
        // eslint-disable-next-line import/namespace -- re-exported via `export *`, the resolver misses it
        importance: Notifications.AndroidImportance.DEFAULT,
      });
    }
    let { status } = await Notifications.getPermissionsAsync();
    if (status !== 'granted') {
      ({ status } = await Notifications.requestPermissionsAsync());
    }
    if (status !== 'granted') return null;

    const id = projectId();
    if (!id) {
      console.warn(
        '[push] No EAS projectId (expo.extra.eas.projectId) — push notifications are disabled. ' +
          'Run `npx eas-cli@latest init` to create a project.',
      );
      return null;
    }
    const token = await Notifications.getExpoPushTokenAsync({ projectId: id });
    return token.data || null;
  } catch (err) {
    console.warn('[push] Could not get a push token:', err);
    return null;
  }
}

/** Calls `onData` with the notification's data when the user taps a push. */
export function addPushResponseListener(onData: (data: Record<string, unknown>) => void): () => void {
  try {
    const sub = Notifications.addNotificationResponseReceivedListener((response) => {
      onData((response.notification.request.content.data ?? {}) as Record<string, unknown>);
    });
    return () => sub.remove();
  } catch {
    return () => undefined;
  }
}

/** Data of the push that cold-started the app (once), if any. */
export async function consumeInitialPushData(): Promise<Record<string, unknown> | null> {
  try {
    const last = await Notifications.getLastNotificationResponseAsync();
    if (!last) return null;
    await Notifications.clearLastNotificationResponseAsync();
    return (last.notification.request.content.data ?? {}) as Record<string, unknown>;
  } catch {
    return null;
  }
}
