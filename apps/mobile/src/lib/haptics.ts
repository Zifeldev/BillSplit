import * as Haptics from 'expo-haptics';
import { Platform } from 'react-native';

const native = Platform.OS === 'ios' || Platform.OS === 'android';

/** Light tick for taps / toggles. No-op on web. */
export function tapFeedback(): void {
  if (native) void Haptics.selectionAsync().catch(() => undefined);
}

/** Medium bump for primary actions. */
export function impactFeedback(): void {
  if (native) void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium).catch(() => undefined);
}

/** Success pattern for completed actions (finalize, settle up). */
export function successFeedback(): void {
  if (native) void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success).catch(() => undefined);
}

export function errorFeedback(): void {
  if (native) void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error).catch(() => undefined);
}
