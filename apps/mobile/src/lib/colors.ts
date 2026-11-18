/** Stable, deterministic colors and icons for people and groups. */

import type { IconName } from '@/components/Icon';
import { tone } from '@/theme';

function hash(seed: string): number {
  let h = 2166136261;
  for (let i = 0; i < seed.length; i++) {
    h ^= seed.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return Math.abs(h);
}

// Mid-tones from the theme: white initials keep at least 4.5:1 on each.
const AVATAR_COLORS: string[] = [
  tone.forest,
  tone.slate,
  tone.clay,
  tone.plum,
  tone.pine,
  tone.ochre,
  tone.brick,
  tone.moss,
  tone.stone,
];

/** Avatar color for a person, stable per user id. */
export function personColor(seed: string): string {
  return AVATAR_COLORS[hash(seed || '?') % AVATAR_COLORS.length];
}

const COVERS: string[] = [tone.forest, tone.pine, tone.slate, tone.clay, tone.plum, tone.ochre, tone.moss, tone.brick];

const ICON_RULES: [RegExp, IconName][] = [
  [/trip|travel|vacation|отпуск|поезд|путеш|tour|flight|plane|самол/i, 'airplane'],
  [/beach|sea|море|пляж/i, 'island'],
  [/ski|snow|горы|mountain/i, 'mountain'],
  [/flat|home|house|apartment|кварт|дом|roommate|rent/i, 'home'],
  [/dinner|lunch|food|eat|ужин|обед|еда|restaurant|pizza/i, 'restaurant'],
  [/coffee|кофе|cafe/i, 'coffee'],
  [/bar|beer|drinks|пиво|party|вечерин|birthday|др\b/i, 'confetti'],
  [/car|road|авто|машин/i, 'car-sport'],
  [/camp|поход|hike/i, 'tent'],
  [/office|work|работ|team|команд/i, 'briefcase'],
  [/game|игр/i, 'game'],
  [/gym|sport|спорт|football|футбол/i, 'sport'],
  [/family|семь/i, 'family'],
  [/grocer|продукт|shop/i, 'cart'],
];
const FALLBACK_ICONS: IconName[] = ['receipt', 'wallet', 'star', 'people', 'sparkles', 'flag', 'rocket', 'target', 'leaf', 'pricetag'];

export interface GroupCover {
  color: string;
  icon: IconName;
}

/** Cover color + icon for a group, from its name (keywords) and id. */
export function groupCover(id: string, name: string): GroupCover {
  const h = hash(name.trim().toLowerCase() || id);
  const rule = ICON_RULES.find(([re]) => re.test(name));
  return {
    color: COVERS[h % COVERS.length],
    icon: rule ? rule[1] : FALLBACK_ICONS[h % FALLBACK_ICONS.length],
  };
}
