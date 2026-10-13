import type { FilterValue, MockDownload } from "./types";

export const seedDownloads: MockDownload[] = [
  {
    id: "d1",
    filename: "File_name_1.exfat",
    savePath: "/Volumes/SSD_Ext/Payloads",
    host: "Viking.pm",
    status: "active",
    progress: 84,
    speedBps: 2_000_000,
    baseSpeedBps: 2_000_000,
    elapsedSec: 20 * 60 + 8,
    etaSec: 11 * 60 + 15,
    totalBytes: 8_400_000_000,
  },
  {
    id: "d2",
    filename: "File_name_2.mkv",
    savePath: "/Volumes/SSD_Ext/Video",
    host: "1file.com",
    status: "active",
    progress: 31,
    speedBps: 1_400_000,
    baseSpeedBps: 1_400_000,
    elapsedSec: 4 * 60 + 42,
    etaSec: 35 * 60 + 19,
    totalBytes: 4_300_000_000,
  },
  {
    id: "d3",
    filename: "File_name_on_queue.txt",
    savePath: "/mnt/Saved/folder",
    host: "Viking.pm",
    status: "queued",
    progress: 0,
    speedBps: 0,
    baseSpeedBps: 0,
    elapsedSec: 0,
    etaSec: null,
    queuePosition: 1,
    totalBytes: 24_000_000,
  },
  {
    id: "d4",
    filename: "Dataset_bundle_2026_10.tar.zst",
    savePath: "/mnt/Saved/folder",
    host: "archive.org",
    status: "queued",
    progress: 0,
    speedBps: 0,
    baseSpeedBps: 0,
    elapsedSec: 0,
    etaSec: null,
    queuePosition: 2,
    totalBytes: 8_400_000_000,
  },
  {
    id: "d5",
    filename: "File_downloaded.png",
    savePath: "/path/to/loc",
    host: "1file.com",
    status: "done",
    progress: 100,
    speedBps: 0,
    baseSpeedBps: 0,
    elapsedSec: 1 * 60 + 12,
    etaSec: 0,
    totalBytes: 4_800_000,
  },
  {
    id: "d6",
    filename: "Firmware_image_signed.img",
    savePath: "/Volumes/SSD_Ext/Firmware",
    host: "Viking.pm",
    status: "done",
    progress: 100,
    speedBps: 0,
    baseSpeedBps: 0,
    elapsedSec: 3 * 60 + 5,
    etaSec: 0,
    totalBytes: 190_000_000,
  },
];

export const filterMatches = (download: MockDownload, filter: FilterValue): boolean => {
  if (filter === "queue") return download.status === "queued";
  if (filter === "done") return download.status === "done";
  return download.status === "active" || download.status === "paused";
};

export const countFor = (downloads: MockDownload[], filter: FilterValue): number =>
  downloads.filter(download => filterMatches(download, filter)).length;
