import styles from "./download-filter.module.css";
import SegmentedControl from "@/components/arc/segmented-control/segmented-control";
import { useMockApp } from "@/state/mock-app-state";
import type { FilterValue } from "@/mock/types";

const TABS: { value: FilterValue; label: string }[] = [
  { value: "queue", label: "Queue" },
  { value: "active", label: "Active" },
  { value: "done", label: "Done" },
];

export default function DownloadFilter() {
  const { filter, counts, setFilter } = useMockApp();

  return (
    <div className={styles.filter}>
      <SegmentedControl
        label="Download status"
        value={filter}
        onValueChange={value => setFilter(value as FilterValue)}
        options={TABS.map(tab => ({ value: tab.value, label: `${tab.label} (${counts[tab.value]})` }))}
      />
    </div>
  );
}
