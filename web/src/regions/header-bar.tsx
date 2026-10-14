import { useEffect, useState } from "react";
import styles from "./header-bar.module.css";
import { Settings } from "lucide-react";
import Badge from "@/components/arc/badge/badge";
import Button from "@/components/arc/button/button";
import type { BadgeTone } from "@/components/arc/badge/badge";
import { useApp } from "@/state/app-state";
import type { LogEntry } from "@/state/app-reducer";

/** A new line holds the pill this long before the connection label returns. */
const FLASH_MS = 10_000;
/** The pill shares the header with the title and the settings button, so a long line is clipped to fit. */
const FLASH_CHARS = 36;
/** A log level recolours the pill while its line is showing. */
const FLASH_TONES: Record<LogEntry["level"], BadgeTone> = { info: "neutral", success: "success", warning: "warning" };

export default function HeaderBar() {
  const { connection, log, setConnectionOpen, setLogOpen } = useApp();
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
  const status = connection.connected ? "Connected" : "Disconnected";
  // While a line is showing it carries that line's own tone; a warning must not wear the green of a healthy connection.
  const tone: BadgeTone = flash ? FLASH_TONES[flash.level] : connection.connected ? "success" : "danger";
  // Clipped on code points, so a split surrogate cannot paint a replacement glyph.
  const message = flash ? [...flash.message] : [];
  const text = flash === null ? status : message.length > FLASH_CHARS ? `${message.slice(0, FLASH_CHARS).join("")}…` : flash.message;

  return (
    <header className={styles.header}>
      <h1 className={styles.title}>Surge</h1>
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
