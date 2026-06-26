import { useEffect, useState, useCallback } from 'react';
import { useCatalogStore } from '@/store/catalogStore';
import type { CatalogDomain, CatalogItem, CatalogListParams } from '@/api/catalog';

interface UseCatalogDomainResult {
  items: CatalogItem[];
  loading: boolean;
  error: string | null;
  refetch: () => Promise<CatalogItem[]>;
}

export function useCatalogDomain(
  domain: CatalogDomain,
  params?: CatalogListParams,
): UseCatalogDomainResult {
  const fetchDomain = useCatalogStore((s) => s.fetchDomain);
  const invalidateDomain = useCatalogStore((s) => s.invalidateDomain);
  const [items, setItems] = useState<CatalogItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    fetchDomain(domain, params)
      .then((result) => {
        if (!cancelled) setItems(result);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to fetch');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [domain, JSON.stringify(params), fetchDomain]);

  const refetch = useCallback(async () => {
    invalidateDomain(domain);
    setLoading(true);
    try {
      const result = await fetchDomain(domain, params, true);
      setItems(result);
      setError(null);
      return result;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch');
      throw err;
    } finally {
      setLoading(false);
    }
  }, [domain, params, fetchDomain, invalidateDomain]);

  return { items, loading, error, refetch };
}
