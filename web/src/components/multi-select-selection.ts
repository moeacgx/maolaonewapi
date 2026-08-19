export function resolveSelectAllSelection(
  selected: string[],
  available: string[]
): string[] {
  const values = Array.from(new Set(available)).filter(Boolean)
  if (values.length === 0) return selected
  const selectedSet = new Set(selected)
  return values.every((value) => selectedSet.has(value)) ? [] : values
}
