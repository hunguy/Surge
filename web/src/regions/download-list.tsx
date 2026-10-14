import styles from "./download-list.module.css";
import { Pause, Play, Trash2 } from "lucide-react";
import SwipeActions, { SwipeActionsRow } from "@/components/arc/swipe-actions/swipe-actions";
import type { SwipeAction } from "@/components/arc/swipe-actions/swipe-actions";
import EmptyState from "@/components/arc/empty-state/empty-state";
import Button from "@/components/arc/button/button";
import DownloadRow from "@/regions/download-row";
import { useApp } from "@/state/app-state";
import type { FilterValue, Download } from "@/types";

const EMPTY: Record<FilterValue, { title: string; description: string }> = {
  queue: { title: "Nothing queued", description: "Downloads wait here until an active slot frees up." },
  active: { title: "Nothing downloading", description: "Add a link and it starts right away." },
  done: { title: "Nothing finished yet", description: "Completed downloads collect here." },
};

const actionsFor = (
  download: Download,
  pause: (id: string) => void,
  resume: (id: string) => void,
  remove: (id: string) => void,
): SwipeAction[] => {
  const actions: SwipeAction[] = [];
  if (download.status === "active") {
    actions.push({
      label: "Pause",
      icon: <Pause size={18} strokeWidth={1.75} aria-hidden="true" />,
      onSelect: () => pause(download.id),
      keepRow: true,
    });
  }
  if (download.status === "paused") {
    actions.push({
      label: "Resume",
      icon: <Play size={18} strokeWidth={1.75} aria-hidden="true" />,
      onSelect: () => resume(download.id),
      keepRow: true,
    });
  }
  actions.push({
    label: "Remove",
    icon: <Trash2 size={18} strokeWidth={1.75} aria-hidden="true" />,
    tone: "danger",
    onSelect: () => remove(download.id),
  });
  return actions;
};

export default function DownloadList() {
  const { visibleDownloads, filter, pause, resume, remove, setAddUrlOpen } = useApp();

  if (visibleDownloads.length === 0) {
    return (
      <div className={styles.region}>
        <EmptyState
          title={EMPTY[filter].title}
          description={EMPTY[filter].description}
          action={
            <Button variant="secondary" onClick={() => setAddUrlOpen(true)}>
              Add download
            </Button>
          }
        />
      </div>
    );
  }

  return (
    <div className={styles.region}>
      {/* The empty state carries its own action, so this toolbar appears only once there are rows to sit above. */}
      <div className={styles.toolbar}>
        <Button variant="secondary" size="sm" onClick={() => setAddUrlOpen(true)}>
          Add download
        </Button>
      </div>
      <SwipeActions label="Downloads">
        {visibleDownloads.map(download => (
          <SwipeActionsRow
            key={download.id}
            label={download.filename}
            trailing={actionsFor(download, pause, resume, remove)}
          >
            <DownloadRow download={download} />
          </SwipeActionsRow>
        ))}
      </SwipeActions>
    </div>
  );
}
