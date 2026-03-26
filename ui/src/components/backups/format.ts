/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/backups/format.ts.
 */
/**
 * TODO: Document normalizeLocalDateTimeInput.
 */
export function normalizeLocalDateTimeInput(value: string): string | null {
  if (!value) {
    return null;
  }

  const [datePart, timePart] = value.split("T");
  if (!datePart || !timePart) {
    return null;
  }

  const [year, month, day] = datePart.split("-").map((part) => Number.parseInt(part, 10));
  const [hourPart = "0", minutePart = "0", secondPart = "0"] = timePart.split(":");
  const [secondValue = "0", millisecondValue = "0"] = secondPart.split(".");
  const hour = Number.parseInt(hourPart, 10);
  const minute = Number.parseInt(minutePart, 10);
  const second = Number.parseInt(secondValue, 10);
  const millisecond = Number.parseInt(millisecondValue.padEnd(3, "0").slice(0, 3), 10);

  if ([year, month, day, hour, minute, second, millisecond].some(Number.isNaN)) {
    return null;
  }

  return new Date(year, month - 1, day, hour, minute, second, millisecond).toISOString();
}
