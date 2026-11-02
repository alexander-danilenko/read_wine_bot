// End times are Kyiv wall-clock time, "YYYY-MM-DDTHH:MM", the format of <input type="datetime-local">.
// The phone's own time zone never enters: the bot runs giveaways on Kyiv time.

const kyivParts = new Intl.DateTimeFormat("en-CA", {
  timeZone: "Europe/Kyiv",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});

// defaultEnd is a week from today in Kyiv at 19:00.
export function defaultEnd(now = new Date()): string {
  const inAWeek = new Date(now.getTime() + 7 * 24 * 60 * 60 * 1000);
  return `${kyivParts.format(inAWeek)}T19:00`;
}

// formatEnd shows "2026-12-01T20:00" as "01.12.2026 20:00".
export function formatEnd(value: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}:\d{2})$/.exec(value);
  return m ? `${m[3]}.${m[2]}.${m[1]} ${m[4]}` : value;
}
