/** Web: picker URIs are blob: or data: URLs — fetch them and encode. */
export async function readFileAsBase64(uri: string): Promise<string> {
  if (uri.startsWith('data:')) {
    const comma = uri.indexOf(',');
    return comma >= 0 ? uri.slice(comma + 1) : '';
  }
  const res = await fetch(uri);
  const blob = await res.blob();
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error ?? new Error('Failed to read file'));
    reader.onload = () => {
      const result = typeof reader.result === 'string' ? reader.result : '';
      const comma = result.indexOf(',');
      resolve(comma >= 0 ? result.slice(comma + 1) : result);
    };
    reader.readAsDataURL(blob);
  });
}
