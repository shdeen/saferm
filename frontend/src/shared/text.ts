import appstrings from './appstrings.json';

/** Labels, messages, and help text of the interface. */
export { appstrings };

/** Replaces each `{name}` placeholder that has a supplied value and leaves the others intact. */
export function formatText(
  template: string,
  replacements: Record<string, string | number>,
): string {
  return template.replace(
    /\{(\w+)\}/g,
    function replaceToken(token: string, placeholderName: string) {
      return String(replacements[placeholderName] ?? token);
    },
  );
}

/** Returns the final nonempty path segment for either path separator. */
export function fileName(filePath: string): string {
  return filePath.split(/[\\/]/).filter(Boolean).at(-1) ?? filePath;
}

/** Formats a timestamp in the local date and time format, or returns the unset label. */
export function formatDate(timestamp?: string): string {
  if (!timestamp) return appstrings.unset;
  return new Date(timestamp).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}
