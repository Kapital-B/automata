/** Up to two initials from a name, or the first letter of an address. */
export function initials(name: string, email?: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  if (words.length >= 2) return (words[0][0] + words[words.length - 1][0]).toUpperCase();
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase();
  return (email?.trim()[0] ?? "?").toUpperCase();
}

/** A person's display name, falling back to their address. */
export function contactName(name: string | undefined, email?: string): string {
  return name?.trim() || email?.trim() || "Unnamed contact";
}
