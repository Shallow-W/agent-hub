interface FilterableItem {
  name?: string;
  key?: string;
  category?: string;
  description?: string;
}

export function filterByCategory<T extends FilterableItem>(items: T[], category: string): T[] {
  if (category === 'all') return items;
  return items.filter((item) => (item.category?.trim() || '未分类') === category);
}

export function searchItems<T extends FilterableItem>(items: T[], query: string): T[] {
  const q = query.trim().toLowerCase();
  if (!q) return items;
  return items.filter((item) => {
    const name = (item.name || item.key || '').toLowerCase();
    const desc = (item.description || '').toLowerCase();
    const cat = (item.category || '').toLowerCase();
    return name.includes(q) || desc.includes(q) || cat.includes(q);
  });
}

export function filterAndSearch<T extends FilterableItem>(items: T[], category: string, query: string): T[] {
  return searchItems(filterByCategory(items, category), query);
}

export function extractCategories<T extends FilterableItem>(items: T[]): string[] {
  const cats = new Set<string>();
  items.forEach((item) => cats.add(item.category?.trim() || '未分类'));
  return Array.from(cats);
}
