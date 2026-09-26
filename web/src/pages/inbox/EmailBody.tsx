import { useMemo, useState, type SyntheticEvent } from "react";
import { buildEmailSrcDoc, isProbablyHtml } from "./format";

/**
 * The message itself. HTML mail renders in a sandboxed, script-free iframe
 * sized to its content; plain text wraps, long links included, so neither can
 * push the page wider than the screen.
 */
export function EmailBody({ body }: { body: string }) {
  const [height, setHeight] = useState(320);
  const html = useMemo(() => isProbablyHtml(body), [body]);
  const srcDoc = useMemo(() => buildEmailSrcDoc(body), [body]);

  if (!body.trim()) {
    return <p className="px-4 py-5 text-sm text-muted-foreground sm:px-6">This message has no body.</p>;
  }

  if (!html) {
    return (
      <div className="whitespace-pre-wrap px-4 py-5 text-sm leading-relaxed text-foreground/90 [overflow-wrap:anywhere] sm:px-6">
        {body}
      </div>
    );
  }

  const resizeFrame = (event: SyntheticEvent<HTMLIFrameElement>) => {
    const documentHeight = event.currentTarget.contentDocument?.documentElement.scrollHeight;
    if (documentHeight) {
      setHeight(Math.min(Math.max(documentHeight, 320), 6000));
    }
  };

  return (
    <div className="px-2 py-4 sm:px-6 sm:py-5">
      <iframe
        title="Email body"
        srcDoc={srcDoc}
        sandbox="allow-popups allow-popups-to-escape-sandbox allow-same-origin"
        onLoad={resizeFrame}
        className="block w-full max-w-full rounded-md border border-border bg-white"
        style={{ height }}
      />
    </div>
  );
}
