import React, { useState } from 'react';
import { Modal, Pressable, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { tapFeedback } from '@/lib/haptics';
import {
  heroGradient,
  MAX_WIDTH,
  PALETTES,
  paletteTokens,
  radius,
  setPalette,
  setThemePreference,
  spacing,
  useIsWide,
  usePalette,
  useTheme,
  useThemePreference,
  type PaletteInfo,
  type PaletteName,
  type ThemePreference,
} from '@/theme';

import { auroraImage, frosted } from './Glass';
import { Icon, type IconName } from './Icon';
import { IconButton, Row, Segmented, Txt } from './ui';

const THEME_OPTIONS: { value: ThemePreference; label: string; icon: IconName }[] = [
  { value: 'system', label: 'System', icon: 'contrast' },
  { value: 'light', label: 'Light', icon: 'sun' },
  { value: 'dark', label: 'Dark', icon: 'moon' },
];

const ORB_SIZE = 30;
const ORB_RING = 3;
const DIALOG_MAX_WIDTH = 480;
const PREVIEW_HEIGHT = 112;

/** A drop of the palette: its hero gradient under a glass highlight. */
export function PaletteOrb({ name, size = ORB_SIZE }: { name: PaletteName; size?: number }) {
  const { dark } = useTheme();
  const tokens = paletteTokens(name, dark);
  return (
    <View
      style={[
        styles.orb,
        { width: size, height: size, borderRadius: size / 2, boxShadow: `0px 4px 12px ${tokens.accentGlow}` },
        heroGradient(tokens.hero),
      ]}>
      <View style={[styles.orbShine, { width: size * 0.5, height: size * 0.3, borderRadius: size * 0.2 }]} />
    </View>
  );
}

/** Miniature of a screen in the given palette: aurora, a hero panel and a frosted card. */
function PalettePreview({ name }: { name: PaletteName }) {
  const { dark } = useTheme();
  const tokens = paletteTokens(name, dark);
  return (
    <View style={[styles.preview, { backgroundColor: tokens.bg, backgroundImage: auroraImage(tokens.aurora) }]}>
      <View style={[styles.previewHero, heroGradient(tokens.hero)]}>
        <View style={[styles.line, styles.lineShort, styles.onHero]} />
        <View style={[styles.line, styles.lineBold, styles.onHero]} />
      </View>
      <View style={[styles.previewCard, frosted(tokens)]}>
        <View style={[styles.dot, { backgroundColor: tokens.accent }]} />
        <View style={[styles.line, styles.lineFill, { backgroundColor: tokens.border }]} />
        <View style={[styles.pill, { backgroundColor: tokens.primarySoft }]} />
      </View>
    </View>
  );
}

function PaletteTile({ info, selected }: { info: PaletteInfo; selected: boolean }) {
  const { c } = useTheme();
  return (
    <Pressable
      accessibilityRole="radio"
      accessibilityState={{ selected }}
      accessibilityLabel={`${info.label} palette`}
      onPress={() => {
        tapFeedback();
        setPalette(info.name);
      }}
      style={({ pressed }) => [
        styles.tile,
        { borderColor: selected ? c.primary : c.border },
        selected && { boxShadow: `0px 8px 22px ${c.accentGlow}` },
        pressed && styles.pressed,
      ]}>
      <PalettePreview name={info.name} />
      <Row gap={spacing.sm} style={styles.tileLabel}>
        <PaletteOrb name={info.name} size={20} />
        <Txt weight="600" style={styles.flex1} numberOfLines={1}>
          {info.label}
        </Txt>
        {selected ? <Icon name="checkmark-circle" size={20} color={c.primary} /> : null}
      </Row>
    </Pressable>
  );
}

/** The palette choices as a 2×2 grid of live previews. */
export function PaletteGrid() {
  const current = usePalette();
  return (
    <View accessibilityRole="radiogroup" style={styles.grid}>
      {PALETTES.map((info) => (
        <PaletteTile key={info.name} info={info} selected={info.name === current} />
      ))}
    </View>
  );
}

/** Theme and palette controls; changes apply to the whole app at once, behind the sheet. */
export function AppearanceSheet({ visible, onClose }: { visible: boolean; onClose: () => void }) {
  const { c } = useTheme();
  const wide = useIsWide();
  const insets = useSafeAreaInsets();
  const preference = useThemePreference();
  return (
    <Modal visible={visible} transparent animationType={wide ? 'fade' : 'slide'} onRequestClose={onClose}>
      <Pressable style={[styles.backdrop, wide && styles.backdropWide]} onPress={onClose}>
        <Pressable
          onPress={() => undefined}
          style={[
            styles.sheet,
            wide && styles.dialog,
            frosted(c, true),
            { paddingBottom: spacing.xl + (wide ? 0 : insets.bottom) },
          ]}>
          {wide ? null : <View style={[styles.grabber, { backgroundColor: c.border }]} />}
          <Row>
            <Txt variant="h2" style={styles.flex1}>
              Appearance
            </Txt>
            <IconButton icon="close" onPress={onClose} label="Close" filled />
          </Row>
          <Txt variant="label">Theme</Txt>
          <Segmented options={THEME_OPTIONS} value={preference} onChange={setThemePreference} />
          <Txt variant="label" style={styles.sectionLabel}>
            Color palette
          </Txt>
          <PaletteGrid />
        </Pressable>
      </Pressable>
    </Modal>
  );
}

/** Header button: an orb in the current palette that opens the appearance sheet. */
export function AppearanceButton() {
  const palette = usePalette();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel="Change theme and color palette"
        hitSlop={8}
        onPress={() => {
          tapFeedback();
          setOpen(true);
        }}
        style={({ pressed }) => [styles.orbButton, pressed && styles.orbPressed]}>
        <PaletteOrb name={palette} />
      </Pressable>
      <AppearanceSheet visible={open} onClose={() => setOpen(false)} />
    </>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  orb: { borderWidth: ORB_RING / 2, borderColor: 'rgba(255,255,255,0.72)', alignItems: 'center', overflow: 'hidden' },
  orbShine: { marginTop: 3, backgroundColor: 'rgba(255,255,255,0.42)' },
  orbButton: { minWidth: 44, minHeight: 44, alignItems: 'center', justifyContent: 'center' },
  orbPressed: { transform: [{ scale: 0.9 }] },
  pressed: { opacity: 0.85, transform: [{ scale: 0.98 }] },
  backdrop: { flex: 1, backgroundColor: 'rgba(13,20,16,0.45)', justifyContent: 'flex-end', alignItems: 'center' },
  backdropWide: { justifyContent: 'center' },
  sheet: {
    width: '100%',
    maxWidth: MAX_WIDTH,
    borderTopLeftRadius: radius.xl,
    borderTopRightRadius: radius.xl,
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.sm,
    gap: spacing.sm,
  },
  dialog: { maxWidth: DIALOG_MAX_WIDTH, borderRadius: radius.xl, paddingTop: spacing.lg },
  grabber: { alignSelf: 'center', width: 40, height: 5, borderRadius: 3, marginBottom: spacing.xs },
  sectionLabel: { marginTop: spacing.sm },
  grid: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing.md },
  tile: { flexGrow: 1, flexBasis: '45%', borderRadius: radius.lg, borderWidth: 2, overflow: 'hidden' },
  tileLabel: { paddingHorizontal: spacing.md, paddingVertical: spacing.md },
  preview: { height: PREVIEW_HEIGHT, padding: spacing.md, gap: spacing.sm },
  previewHero: { flex: 1, borderRadius: radius.sm, padding: spacing.sm, justifyContent: 'center', gap: 5 },
  previewCard: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    borderRadius: radius.sm,
    paddingHorizontal: spacing.sm,
    height: 30,
  },
  line: { height: 5, borderRadius: 3 },
  lineShort: { width: '34%', opacity: 0.7 },
  lineBold: { width: '58%', height: 9, borderRadius: 5 },
  lineFill: { flex: 1 },
  onHero: { backgroundColor: '#FFFFFF' },
  dot: { width: 14, height: 14, borderRadius: 7 },
  pill: { width: 30, height: 12, borderRadius: 6 },
});
