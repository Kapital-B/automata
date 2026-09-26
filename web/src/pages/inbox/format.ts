import type { MessageItem } from "@/lib/auth";

const HTML_TAG_RE = /<\/?[a-z][\s\S]*>/i;
const HTML_DOCUMENT_RE = /<(?:!doctype|html|head|body)\b/i;
const TEXT_BODY_URL_RE = /\[https?:\/\/[^\]]+\]/i;
const EMAIL_CSP =
  "default-src 'none'; img-src http: https: data: cid: blob:; style-src 'unsafe-inline' http: https:; font-src http: https: data:; connect-src 'none'; script-src 'none'; form-action 'none'; frame-ancestors 'none';";

/** Primary line is display name or address; secondary is the other part when both exist. */
export function senderLines(from: MessageItem["from_json"]): { primary: string; secondary?: string } {
  const name = from?.name?.trim() ?? "";
  const addr = from?.address?.trim() ?? "";
  if (name && addr) {
    const same = name.toLowerCase().replace(/^mailto:/i, "") === addr.toLowerCase().replace(/^mailto:/i, "");
    if (same) return { primary: name };
    return { primary: name, secondary: addr };
  }
  if (addr) return { primary: addr };
  if (name) return { primary: name };
  return { primary: "Unknown sender" };
}

export function isProbablyHtml(body: string) {
  return HTML_TAG_RE.test(body);
}

/** A plain-text body carrying bracketed links is a flattened HTML email worth re-fetching. */
export function looksLikeTextConvertedHtml(body: string) {
  return !isProbablyHtml(body) && TEXT_BODY_URL_RE.test(body);
}

/** Wraps email HTML in a document with a locked-down CSP, for a sandboxed iframe. */
export function buildEmailSrcDoc(html: string) {
  const securityHead = `<base target="_blank"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta http-equiv="Content-Security-Policy" content="${EMAIL_CSP}"><style>html,body{margin:0;padding:0;max-width:100%;overflow-wrap:anywhere}img{max-width:100%;height:auto}table{max-width:100%}</style>`;

  if (HTML_DOCUMENT_RE.test(html)) {
    if (/<head\b[^>]*>/i.test(html)) {
      return html.replace(/<head\b([^>]*)>/i, `<head$1>${securityHead}`);
    }
    if (/<html\b[^>]*>/i.test(html)) {
      return html.replace(/<html\b([^>]*)>/i, `<html$1><head>${securityHead}</head>`);
    }
    return `<!doctype html><html><head>${securityHead}</head>${html}</html>`;
  }

  return `<!doctype html>
<html>
  <head>
    ${securityHead}
  </head>
  <body>${html}</body>
</html>`;
}
