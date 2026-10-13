import styles from "./throughput-chart.module.css";
import LineChart from "@/components/arc/line-chart/line-chart";
import { useApp } from "@/state/app-state";
import { averageOf, minOf, peakOf } from "@/throughput";
import { useMediaQuery } from "@/regions/use-media-query";

const SERIES = [
  { key: "download", label: "Download", area: true },
  { key: "upload", label: "Upload", dashed: true },
];

const round = (value: number) => Math.round(value).toLocaleString("en-US");

export default function ThroughputChart() {
  const { throughput } = useApp();
  const wide = useMediaQuery("(min-width: 768px)");

  const stats = [
    { label: "Min", value: minOf(throughput) },
    { label: "Average", value: averageOf(throughput) },
    { label: "Peak", value: peakOf(throughput) },
  ];

  return (
    <section className={styles.region}>
      <LineChart
        data={throughput.map(point => ({ key: point.key, label: point.label, axisLabel: point.axisLabel, values: point.values }))}
        series={SERIES}
        label="Throughput"
        unit="MB/s"
        height={wide ? 220 : 160}
        categoryLabel="Time"
      />
      <dl className={styles.stats}>
        {stats.map(stat => (
          <div className={styles.stat} key={stat.label}>
            <dt className={styles.statLabel}>{stat.label}</dt>
            <dd className={styles.statValue}>{round(stat.value)} MB/s</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
