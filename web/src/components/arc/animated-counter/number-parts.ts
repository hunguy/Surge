export type Part = { key: string; digit: number; order: number } | { key: string; text: string; separator: boolean };

/** Split a formatted number into columns keyed by place value, so 999 → 1,000 keeps the ones column the ones column. */
export function partsFor(value: number, decimals: number, locale: string): Part[] {
  const parts = new Intl.NumberFormat(locale, { minimumFractionDigits: decimals, maximumFractionDigits: decimals, numberingSystem: "latn" }).formatToParts(value);
  let place = parts.reduce((count, part) => count + (part.type === "integer" ? part.value.length : 0), 0);
  let fraction = 0;
  let order = 0;
  return parts.flatMap((part, index): Part[] => {
    if (part.type === "integer") return [...part.value].map(char => ({ key: `i${--place}`, digit: Number(char), order: order++ }));
    if (part.type === "fraction") return [...part.value].map(char => ({ key: `f${fraction++}`, digit: Number(char), order: order++ }));
    if (part.type === "group" || part.type === "decimal") return [{ key: part.type === "group" ? `g${place}` : "d", text: part.value, separator: true }];
    return [{ key: `${part.type}${index}`, text: part.value, separator: false }];
  });
}
