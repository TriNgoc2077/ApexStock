'use client';

import { useEffect, useRef, useState, useCallback } from 'react';

interface UsePollingOptions<T> {
  fetcher: () => Promise<T>;
  intervalMs?: number;
  isTerminal?: (data: T) => boolean;
  enabled?: boolean;
}

export function usePolling<T>({
  fetcher,
  intervalMs = 1500,
  isTerminal,
  enabled = true,
}: UsePollingOptions<T>) {
  const [data, setData] = useState<T | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isError, setIsError] = useState<boolean>(false);
  const [error, setError] = useState<Error | null>(null);
  const [isStale, setIsStale] = useState<boolean>(false);

  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;

  const isTerminalRef = useRef(isTerminal);
  isTerminalRef.current = isTerminal;

  const poll = useCallback(async () => {
    try {
      const result = await fetcherRef.current();
      setData(result);
      setIsError(false);
      setError(null);
      setIsStale(false);
      setIsLoading(false);

      if (isTerminalRef.current && isTerminalRef.current(result)) {
        return false; // Stop polling
      }
      return true; // Continue polling
    } catch (err: any) {
      setIsError(true);
      setError(err);
      setIsStale(true);
      setIsLoading(false);
      return true; // Continue polling on transient errors
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;

    let active = true;
    let timerId: NodeJS.Timeout;

    const execute = async () => {
      // Pause polling if tab is backgrounded
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
        timerId = setTimeout(execute, intervalMs);
        return;
      }

      const shouldContinue = await poll();
      if (active && shouldContinue) {
        timerId = setTimeout(execute, intervalMs);
      }
    };

    execute();

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        poll();
      }
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      active = false;
      clearTimeout(timerId);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
  }, [enabled, intervalMs, poll]);

  return { data, isLoading, isError, error, isStale, refetch: poll };
}
