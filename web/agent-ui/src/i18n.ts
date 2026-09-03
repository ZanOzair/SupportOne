/**
 * Message resolution in the browser mirrors the Go side: results carry keys,
 * the catalog fills them in, and a key with no translation renders as itself so
 * a gap is visible rather than blank.
 */

/** Formats a Go-style message, which is what the catalogs contain. */
export function format(template: string, args: unknown[] = []): string {
  let index = 0;
  return template.replace(/%(%|[a-zA-Z])/g, (match, verb: string) => {
    if (verb === '%') {
      return '%';
    }
    if (index >= args.length) {
      return match;
    }
    return String(args[index++]);
  });
}

export function translator(messages: Record<string, string>) {
  return (key: string, args: unknown[] = []): string => {
    const template = messages[key];
    if (template === undefined) {
      return key;
    }
    return format(template, args);
  };
}

export type Translate = ReturnType<typeof translator>;
