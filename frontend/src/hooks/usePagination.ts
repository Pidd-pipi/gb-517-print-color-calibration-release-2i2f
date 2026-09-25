
import { useMemo, useState } from 'react';
export function usePagination(total: number, initialPageSize = 20) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(initialPageSize);
  const pages = useMemo(() => Math.max(1, Math.ceil(total / pageSize)), [total, pageSize]);
  const previous = () => setPage((value) => Math.max(1, value - 1));
  const next = () => setPage((value) => Math.min(pages, value + 1));
  return { page, pageSize, pages, setPage, setPageSize, previous, next };
}
