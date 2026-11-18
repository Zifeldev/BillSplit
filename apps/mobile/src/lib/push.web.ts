/** Web: no push notifications (the in-app inbox still works). */
export async function registerForPushAsync(): Promise<string | null> {
  return null;
}

export function addPushResponseListener(_onData: (data: Record<string, unknown>) => void): () => void {
  return () => undefined;
}

export async function consumeInitialPushData(): Promise<Record<string, unknown> | null> {
  return null;
}
