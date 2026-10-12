import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  buildPortScanCandidates,
  buildDownloadRequestBody,
  buildEventStreamHeaders,
  coerceStoredBoolean,
  extractFilename,
  filterPendingDuplicates,
  findReachableCandidate,
  openEventStream,
  queueDuplicateDownload,
  resolveInterceptEnabled,
} from '../lib/background-logic';

describe('background logic', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('coerces stored booleans from extension storage values', () => {
    expect(coerceStoredBoolean(true)).toBe(true);
    expect(coerceStoredBoolean(false)).toBe(false);
    expect(coerceStoredBoolean('true')).toBe(true);
    expect(coerceStoredBoolean('false')).toBe(false);
    expect(coerceStoredBoolean(undefined)).toBeUndefined();
    expect(resolveInterceptEnabled(undefined)).toBe(true);
    expect(resolveInterceptEnabled(false)).toBe(false);
  });

  it('includes auth when building SSE headers', () => {
    expect(buildEventStreamHeaders('secret-token')).toEqual({
      Accept: 'text/event-stream',
      'Cache-Control': 'no-cache',
      Authorization: 'Bearer secret-token',
    });

    expect(buildEventStreamHeaders(null)).toEqual({
      Accept: 'text/event-stream',
      'Cache-Control': 'no-cache',
    });
  });

  it('builds download request bodies with optional skip approval', () => {
    expect(buildDownloadRequestBody({
      url: 'https://example.com/file.zip',
      filename: 'file.zip',
      headers: { Cookie: 'a=b' },
      skipApproval: true,
    })).toEqual({
      url: 'https://example.com/file.zip',
      filename: 'file.zip',
      headers: { Cookie: 'a=b' },
      skip_approval: true,
      skip_duplicate_warning: undefined,
    });

    expect(buildDownloadRequestBody({
      url: 'https://example.com/file.zip',
      filename: 'file.zip',
      headers: {},
    })).toEqual({
      url: 'https://example.com/file.zip',
      filename: 'file.zip',
      headers: undefined,
      skip_approval: undefined,
      skip_duplicate_warning: undefined,
    });

    expect(buildDownloadRequestBody({
      url: 'https://example.com/file.zip',
      filename: '',
      headers: {},
      skipDuplicateWarning: true,
    })).toMatchObject({ skip_duplicate_warning: true, skip_approval: undefined });
  });

  it('never sends a browser-derived destination path', () => {
    // The browser's download directory must not override the default download
    // directory configured in Surge's settings.
    const body = buildDownloadRequestBody({
      url: 'https://example.com/file.zip',
      filename: 'file.zip',
      headers: {},
      skipApproval: true,
    });

    expect(body).not.toHaveProperty('path');
    expect(body).not.toHaveProperty('relative_to_default_dir');
  });

  it('prioritizes preferred URLs when building port scan candidates', () => {
    expect(buildPortScanCandidates(1700, 3, ['http://127.0.0.1:1702', 'http://127.0.0.1:1701']))
      .toEqual([
        'http://127.0.0.1:1702',
        'http://127.0.0.1:1701',
        'http://127.0.0.1:1700',
      ]);
  });

  it('finds a reachable candidate without waiting for the full list', async () => {
    const probe = vi.fn(async (candidate: string) => {
      if (candidate === 'http://127.0.0.1:1703') {
        await new Promise((resolve) => setTimeout(resolve, 10));
        return true;
      }

      await new Promise((resolve) => setTimeout(resolve, 50));
      return false;
    });

    const match = await findReachableCandidate([
      'http://127.0.0.1:1700',
      'http://127.0.0.1:1701',
      'http://127.0.0.1:1702',
      'http://127.0.0.1:1703',
    ], probe, 4);

    expect(match).toBe('http://127.0.0.1:1703');
  });

  it('updates the badge when queueing a duplicate download', async () => {
    const pendingDuplicates = new Map();
    const persistPendingDuplicates = vi.fn().mockResolvedValue(undefined);
    const updateBadge = vi.fn();
    const openPopup = vi.fn().mockResolvedValue(undefined);
    const sendPrompt = vi.fn().mockResolvedValue(undefined);

    const nextCounter = await queueDuplicateDownload({
      pendingDuplicates,
      pendingDuplicateCounter: 0,
      url: 'https://example.com/file.zip',
      filename: 'file.zip',
      persistPendingDuplicates,
      updateBadge,
      openPopup,
      sendPrompt,
      now: () => 1234,
    });

    expect(nextCounter).toBe(1);
    expect([...pendingDuplicates]).toEqual([[
      'dup_1',
      {
        url: 'https://example.com/file.zip',
        filename: 'file.zip',
        timestamp: 1234,
      },
    ]]);
    expect(persistPendingDuplicates).toHaveBeenCalledOnce();
    expect(updateBadge).toHaveBeenCalledOnce();
    expect(openPopup).toHaveBeenCalledOnce();
    expect(sendPrompt).toHaveBeenCalledWith({
      type: 'promptDuplicate',
      id: 'dup_1',
      filename: 'file.zip',
    });
  });

  it('opens the SSE stream with the expected headers', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(new Response('ok', { status: 200 }));
    const controller = new AbortController();

    await openEventStream('http://127.0.0.1:1700', 'token-123', controller.signal, fetchImpl);

    expect(fetchImpl).toHaveBeenCalledWith('http://127.0.0.1:1700/events', {
      headers: {
        Accept: 'text/event-stream',
        'Cache-Control': 'no-cache',
        Authorization: 'Bearer token-123',
      },
      signal: controller.signal,
    });
  });

  it('filters expired pending duplicates during rehydration', () => {
    expect(filterPendingDuplicates([
      ['dup_1', { url: 'a', filename: 'a', timestamp: 50 }],
      ['dup_2', { url: 'b', filename: 'b', timestamp: 150 }],
    ], 200, 100)).toEqual([
      ['dup_2', { url: 'b', filename: 'b', timestamp: 150 }],
    ]);
  });

  it('extracts the filename hint across path styles', () => {
    expect(extractFilename({ filename: 'C:\\Users\\me\\file.zip' })).toBe('file.zip');
    expect(extractFilename({ filename: '/home/me/file.zip' })).toBe('file.zip');
    expect(extractFilename({ filename: 'downloads/file with spaces.zip' })).toBe('file with spaces.zip');
    expect(extractFilename({ filename: 'plain.txt' })).toBe('plain.txt');
    expect(extractFilename({})).toBe('');
  });
});
