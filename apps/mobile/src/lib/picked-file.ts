import type { DocumentPickerAsset } from 'expo-document-picker';

import { readFileAsBase64 } from './file-base64';

export interface PickedFile {
  name: string;
  size: number;
  mimeType: string;
  base64: string;
  isPdf: boolean;
  /** Local / blob / data URI usable for an image preview. */
  uri: string;
}

const stripDataPrefix = (b64: string) => (b64.startsWith('data:') ? b64.slice(b64.indexOf(',') + 1) : b64);

/** Decoded byte length of a base64 string. */
export function base64Bytes(b64: string): number {
  const pad = b64.endsWith('==') ? 2 : b64.endsWith('=') ? 1 : 0;
  return Math.floor((b64.length * 3) / 4) - pad;
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

/**
 * Reads a DocumentPicker asset as base64 on every platform. On web the asset
 * carries a data: URI (or a File), on native a file:// URI in the cache dir.
 * Throws a friendly error when the file exceeds `maxBytes`.
 */
export async function readPickedFile(a: DocumentPickerAsset, maxBytes: number): Promise<PickedFile> {
  const name = a.name || 'file';
  const mimeType = a.mimeType || (name.toLowerCase().endsWith('.pdf') ? 'application/pdf' : '');
  const isPdf = mimeType === 'application/pdf' || name.toLowerCase().endsWith('.pdf');
  if (a.size && a.size > maxBytes) {
    throw new Error(`"${name}" is ${formatBytes(a.size)} — the limit is ${formatBytes(maxBytes)}. Try a smaller file.`);
  }
  let b64 = a.base64 ? stripDataPrefix(a.base64) : '';
  if (!b64 && a.file && typeof FileReader !== 'undefined') {
    b64 = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onerror = () => reject(reader.error ?? new Error('Failed to read file'));
      reader.onload = () => resolve(stripDataPrefix(typeof reader.result === 'string' ? reader.result : ''));
      reader.readAsDataURL(a.file as Blob);
    });
  }
  if (!b64) b64 = stripDataPrefix(await readFileAsBase64(a.uri));
  if (!b64) throw new Error(`Could not read "${name}".`);
  const size = base64Bytes(b64);
  if (size > maxBytes) {
    throw new Error(`"${name}" is ${formatBytes(size)} — the limit is ${formatBytes(maxBytes)}. Try a smaller file.`);
  }
  return { name, size, mimeType, base64: b64, isPdf, uri: a.uri };
}
