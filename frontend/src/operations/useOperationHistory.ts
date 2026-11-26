import { useCallback, useEffect, useRef, useState } from 'react';
import { getOperationPage, getOperationStatuses, type ActionPage } from '../bridge/operations';

/** Milliseconds between automatic history refreshes. */
const refreshInterval = 5000;

/**
 * Loads one page of operation history with the status codes, and keeps both current.
 * It starts unfiltered on page one, with twenty operations per page. It reloads when a
 * filter, the page, or the revision changes, when the window gains focus, and
 * periodically while the document is visible. It discards a response when a newer
 * request has started. Changing a filter or the page size returns to page one.
 */
export function useOperationHistory(revision: number) {
  const [history, setHistory] = useState<ActionPage>();
  const [statuses, setStatuses] = useState<Record<string, number | undefined>>({});
  const [statusFilter, setStatusFilter] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [pageNumber, setPageNumber] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const requestSequence = useRef(0);

  const refresh = useCallback(
    async function refreshHistory() {
      const currentSequence = ++requestSequence.current;
      try {
        const [operationPage, statusCodes] = await Promise.all([
          getOperationPage(typeFilter, statusFilter, pageNumber, pageSize),
          getOperationStatuses(),
        ]);
        if (currentSequence !== requestSequence.current) return;
        setHistory(operationPage);
        setStatuses(statusCodes ?? {});
        setError('');
      } catch (error) {
        if (currentSequence === requestSequence.current) setError(String(error));
      } finally {
        if (currentSequence === requestSequence.current) setLoading(false);
      }
    },
    [typeFilter, statusFilter, pageNumber, pageSize],
  );

  useEffect(
    function refreshOnChange() {
      setLoading(true);
      void refresh();
      function refreshWhenVisible() {
        if (!document.hidden) void refresh();
      }
      const intervalID = window.setInterval(refreshWhenVisible, refreshInterval);
      window.addEventListener('focus', refreshWhenVisible);
      return function stopRefreshing() {
        requestSequence.current++;
        window.clearInterval(intervalID);
        window.removeEventListener('focus', refreshWhenVisible);
      };
    },
    [refresh, revision],
  );

  function filterStatus(status: string) {
    setStatusFilter(status);
    setPageNumber(1);
  }
  function filterType(actionType: string) {
    setTypeFilter(actionType);
    setPageNumber(1);
  }
  function resizePage(size: number) {
    setPageSize(size);
    setPageNumber(1);
  }

  return {
    history,
    statuses,
    statusFilter,
    typeFilter,
    filterStatus,
    filterType,
    pageSize,
    resizePage,
    setPageNumber,
    error,
    loading,
    refresh,
  };
}
