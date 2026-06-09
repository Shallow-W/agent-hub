interface SegmenterSegment {
  segment: string;
}

interface SegmenterInstance {
  segment(input: string): Iterable<SegmenterSegment>;
}

interface SegmenterConstructor {
  new (
    locales?: string | string[],
    options?: { granularity?: 'grapheme' | 'word' | 'sentence' },
  ): SegmenterInstance;
}

function getSegmenter(): SegmenterConstructor | null {
  const intlWithSegmenter = Intl as typeof Intl & { Segmenter?: SegmenterConstructor };
  return typeof intlWithSegmenter.Segmenter === 'function' ? intlWithSegmenter.Segmenter : null;
}

export function truncateGraphemes(text: string, maxLength: number): string {
  if (!text) return '';
  const Segmenter = getSegmenter();
  if (Segmenter) {
    const segmenter = new Segmenter(undefined, { granularity: 'grapheme' });
    const segments = Array.from(segmenter.segment(text), (segment) => segment.segment);
    return segments.length > maxLength ? `${segments.slice(0, maxLength).join('')}...` : text;
  }
  const chars = Array.from(text);
  return chars.length > maxLength ? `${chars.slice(0, maxLength).join('')}...` : text;
}
