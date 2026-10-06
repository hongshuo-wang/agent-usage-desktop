const SUMMARY_HEADINGS = {
  zh: new Set(["概览", "主要更新", "亮点"]),
  en: new Set(["summary", "highlights"]),
};

function heading(line: string): { level: number; title: string } | null {
  const match = line.match(/^(#{2,4})\s+(.+?)\s*#*\s*$/);
  return match ? { level: match[1].length, title: match[2].trim().toLowerCase() } : null;
}

function bullets(lines: string[]): string[] {
  return lines
    .map((line) => line.match(/^\s*[-*+]\s+(.+?)\s*$/)?.[1].trim())
    .filter((line): line is string => Boolean(line))
    .slice(0, 4);
}

export function releaseSummary(body: string | undefined, language: string): string[] {
  if (!body) return [];
  const preferred = language.startsWith("zh") ? SUMMARY_HEADINGS.zh : SUMMARY_HEADINGS.en;
  const fallback = language.startsWith("zh") ? SUMMARY_HEADINGS.en : SUMMARY_HEADINGS.zh;
  const lines = body.split(/\r?\n/);

  for (const headings of [preferred, fallback]) {
    for (let index = 0; index < lines.length; index += 1) {
      const current = heading(lines[index]);
      if (!current || !headings.has(current.title)) continue;
      const section: string[] = [];
      for (let next = index + 1; next < lines.length; next += 1) {
        const nextHeading = heading(lines[next]);
        if (nextHeading && nextHeading.level <= current.level) break;
        section.push(lines[next]);
      }
      const result = bullets(section);
      if (result.length > 0) return result;
    }
  }

  return [];
}
