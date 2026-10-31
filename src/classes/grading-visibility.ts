// Revise these visibility rules when the CCA syllabus is available.
export const HIDDEN_ITEMS_BY_PROGRAM: Record<string, RegExp[]> = {
  'AI Development': [
    /\bui\s*\/?\s*ux\b/i,
    /\bui\b.*design/i,
    /\bux\b.*design/i,
  ],
};

export function isHiddenItem(
  programName: string | undefined | null,
  itemName: string,
): boolean {
  if (!programName) return false;
  const patterns = Object.entries(HIDDEN_ITEMS_BY_PROGRAM).find(
    ([name]) => name.toLowerCase() === programName.toLowerCase(),
  )?.[1];
  return patterns?.some((pattern) => pattern.test(itemName)) ?? false;
}
