/**
 * Zustand store for catalog domain data.
 *
 * Provides a single cache layer for all 4 catalog domains so components don't
 * need to independently fetch the same data. Uses domain-scoped loading states
 * and a simple freshness mechanism to avoid duplicate requests.
 */
import { create } from 'zustand';
import {
  type CatalogDomain,
  type CatalogItem,
  type CatalogListParams,
  listCatalog,
} from '@/api/catalog';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface DomainCache {
  items: CatalogItem[];
  loaded: boolean;
  loading: boolean;
  fetchedAt: number; // Date.now()
}

interface CatalogState {
  domains: Record<string, DomainCache>;
  fetchDomain: (
    domain: CatalogDomain,
    params?: CatalogListParams,
    force?: boolean,
  ) => Promise<CatalogItem[]>;
  invalidateDomain: (domain: CatalogDomain) => void;
  clearAll: () => void;
}

// ---------------------------------------------------------------------------
// Freshness threshold: 30 seconds
// ---------------------------------------------------------------------------

const FRESH_MS = 30_000;

function emptyCache(): DomainCache {
  return { items: [], loaded: false, loading: false, fetchedAt: 0 };
}

// In-flight dedup: prevents concurrent fetches for the same domain + params.
const inflightMap = new Map<string, Promise<CatalogItem[]>>();

function cacheKey(domain: string, params?: CatalogListParams): string {
  if (!params) return domain;
  return `${domain}:${params.subtype ?? ''}:${params.category ?? ''}`;
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

export const useCatalogStore = create<CatalogState>((set, get) => ({
  domains: {},

  fetchDomain: async (domain, params, force = false) => {
    const key = cacheKey(domain, params);
    const state = get();
    const cache = state.domains[key] ?? emptyCache();

    // Return cached data if fresh and not forced.
    if (!force && cache.loaded && Date.now() - cache.fetchedAt < FRESH_MS) {
      return cache.items;
    }

    // Dedup in-flight requests.
    const existing = inflightMap.get(key);
    if (existing) return existing;

    const promise = (async () => {
      set((s) => ({
        domains: {
          ...s.domains,
          [key]: {
            ...(s.domains[key] ?? emptyCache()),
            loading: true,
          },
        },
      }));

      try {
        const items = await listCatalog(domain, params);
        set((s) => ({
          domains: {
            ...s.domains,
            [key]: {
              items,
              loaded: true,
              loading: false,
              fetchedAt: Date.now(),
            },
          },
        }));
        return items;
      } catch {
        set((s) => ({
          domains: {
            ...s.domains,
            [key]: {
              ...(s.domains[key] ?? emptyCache()),
              loading: false,
            },
          },
        }));
        throw new Error(`Failed to fetch catalog domain: ${domain}`);
      } finally {
        inflightMap.delete(key);
      }
    })();

    inflightMap.set(key, promise);
    return promise;
  },

  invalidateDomain: (domain) => {
    set((s) => {
      const next = { ...s.domains };
      for (const key of Object.keys(next)) {
        if (key === domain || key.startsWith(`${domain}:`)) {
          next[key] = { ...next[key]!, fetchedAt: 0 };
        }
      }
      return { domains: next };
    });
  },

  clearAll: () => {
    set({ domains: {} });
    inflightMap.clear();
  },
}));

/**
 * Reset the catalog store (e.g. on logout).
 */
export function resetCatalogStore(): void {
  useCatalogStore.setState({ domains: {} });
  inflightMap.clear();
}
