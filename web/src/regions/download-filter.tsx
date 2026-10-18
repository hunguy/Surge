import styles from "./download-filter.module.css";
import SegmentedControl from "@/components/arc/segmented-control/segmented-control";
import Badge from "@/components/arc/badge/badge";
import type { BadgeTone } from "@/components/arc/badge/badge";
import { useApp } from "@/state/app-state";
import type { FilterValue } from "@/types";

/** One hue per tab: queued waits, active runs, done finished. */
const TONES: Record<FilterValue, BadgeTone> = { queue: "warning", active: "info", done: "success" };

const TABS: { value: FilterValue; label: string }[] = [
  { value: "queue", label: "Queue" },
  { value: "active", label: "Active" },
  { value: "done", label: "Done" },
];

export default function DownloadFilter() {
  const { filter, counts, setFilter } = useApp();

  return (
    <div className={styles.filter}>
      <SegmentedControl
        label="Download status"
        value={filter}
        onValueChange={value => setFilter(value as FilterValue)}
        options={TABS.map(tab => ({
          value: tab.value,
          label: tab.label,
          // An empty tab shows no chip; a zero is noise in a label that already names the bucket.
          accessory:
            counts[tab.value] > 0 ? (
              <Badge tone={TONES[tab.value]} size="sm">
                {counts[tab.value]}
              </Badge>
            ) : null,
        }))}
      />
    </div>
  );
}
