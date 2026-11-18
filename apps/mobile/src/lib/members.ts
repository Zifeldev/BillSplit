import type { Group } from '@/api/types';

/**
 * Name of a group member by id. Only for places where the API returns bare ids
 * (item claims); everything else carries names from the server.
 */
export function memberName(group: Group | undefined, userId: string, myId?: string): string {
  if (myId && userId === myId) return 'You';
  const m = group?.members?.find((x) => x.userId === userId);
  return m?.name || m?.email || 'Unknown';
}
