import { useEffect, useState } from "react";
import styles from "./header-bar.module.css";
import { Settings } from "lucide-react";
import Badge from "@/components/arc/badge/badge";
import Button from "@/components/arc/button/button";
import type { BadgeTone } from "@/components/arc/badge/badge";
import { useApp } from "@/state/app-state";
import { pillLabel } from "@/regions/server-label";
import type { LogEntry } from "@/state/app-reducer";

/** A new line holds the pill this long before the server label returns. */
const FLASH_MS = 10_000;
/** The pill shares the header with the title and the settings button, so a long line is clipped to fit. */
const FLASH_CHARS = 36;
/** Tighter than a log line: the label carries a name and an address, and the pill must still
 *  fit a phone. 30 code points lands just past "Office iMac (203.0.113.9:1700)", which measures
 *  201px against the 204px the header leaves it at 375px wide. */
const LABEL_CHARS = 30;
/** A log level recolours the pill while its line is showing. */
const FLASH_TONES: Record<LogEntry["level"], BadgeTone> = { info: "neutral", success: "success", warning: "warning" };

/** Clipped on code points, so a split surrogate cannot paint a replacement glyph. */
const clip = (text: string, max: number): string => {
  const points = [...text];
  return points.length > max ? `${points.slice(0, max).join("")}…` : text;
};

export default function HeaderBar() {
  const { connection, savedConnections, log, setConnectionOpen, setLogOpen } = useApp();
  const latest = log[log.length - 1];
  const latestId = latest?.id;

  // The id whose line has had its turn. Held rather than a boolean so the newest line
  // shows even if an older one is still inside its window.
  const [doneId, setDoneId] = useState<string | undefined>(undefined);
  useEffect(() => {
    if (latestId === undefined) return;
    const timer = setTimeout(() => setDoneId(latestId), FLASH_MS);
    return () => clearTimeout(timer);
  }, [latestId]);

  // Derived, so a new line needs no state of its own: it is the latest entry until its timer lands.
  const flash = latest !== undefined && latest.id !== doneId ? latest : null;

  const dot = <span className={styles.dot} aria-hidden="true" />;
  // The tone carries the connection state, so the text is free to name the server.
  const status = clip(pillLabel(connection, savedConnections), LABEL_CHARS);
  // While a line is showing it carries that line's own tone; a warning must not wear the green of a healthy connection.
  const tone: BadgeTone = flash ? FLASH_TONES[flash.level] : connection.connected ? "success" : "danger";
  const text = flash === null ? status : clip(flash.message, FLASH_CHARS);

  return (
    <header className={styles.header}>
      <h1 className={styles.title}>
        <img className={styles.logo} src="/logo.png" alt="" width={21} height={24} />
        Surge
      </h1>
      <div className={styles.actions}>
        {/* The status pill opens the activity log and announces the newest line in passing. */}
        <button type="button" className={styles.status} onClick={() => setLogOpen(true)}>
          <Badge tone={tone} icon={dot}>
            {text}
          </Badge>
        </button>
        <Button variant="ghost" size="sm" aria-label="Server settings" onClick={() => setConnectionOpen(true)}>
          <Settings size={18} strokeWidth={1.75} aria-hidden="true" />
        </Button>
      </div>
    </header>
  );
}
