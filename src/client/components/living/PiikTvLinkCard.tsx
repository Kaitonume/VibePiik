import { useEffect, useId, useRef, useState } from "react";
import { Glyph } from "../../ui/icons";
import { usePiikTvCopy } from "../../lib/piiktv-copy";

type SendState = "idle" | "busy" | "sent" | "failed";

interface SendResult {
  sent?: { address: string; name?: string }[];
}

type Message = { key: "card.searching" | "card.invalidExample" | "card.invalid" | "send.sentTo" | "send.noTv" | "send.failed" | "send.appDown"; vars?: Record<string, string> };

// The shape the App and the TV accept: scheme, host, optional port, /r/<code>, then anything.
const ROOM_LINK = /^https?:\/\/[A-Za-z0-9.-]+(?::\d{1,5})?\/r\/[1-9][0-9]{3}[^\s]*$/;

/** A pasted room link, with https:// added when the scheme is missing; null when it is no Piik room. */
export function piikTvLink(value: string): string | null {
  const text = value.trim();
  if (!text) return null;
  const link = /^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? text : `https://${text}`;
  return link.length <= 1024 && ROOM_LINK.test(link) ? link : null;
}

/**
 * PiikTV: launcher card that sends any Piik room link, also someone else's, to the
 * Piik TV apps open on this LAN, before or without starting a room on this computer.
 * The App finds the TVs and delivers the link server side.
 */
export function PiikTvLinkCard() {
  const { vis, t } = usePiikTvCopy();
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  const [state, setState] = useState<SendState>("idle");
  const [message, setMessage] = useState<Message | null>(null);
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(timer.current), []);

  function finish(next: SendState, text: Message): void {
    setState(next);
    setMessage(text);
    timer.current = window.setTimeout(() => { setState("idle"); setMessage(null); }, 8000);
  }

  async function send(): Promise<void> {
    if (state === "busy") return;
    window.clearTimeout(timer.current);
    const link = piikTvLink(input.current?.value ?? "");
    if (!link) {
      finish("failed", { key: "card.invalidExample" });
      input.current?.focus();
      return;
    }
    if (input.current) input.current.value = link;
    setState("busy");
    setMessage({ key: "card.searching" });
    try {
      const response = await fetch("/api/client-launcher/piik-tv", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ link }),
      });
      const result = await response.json().catch(() => null) as SendResult | null;
      if (response.ok && result?.sent?.length) {
        finish("sent", { key: "send.sentTo", vars: { names: result.sent.map(tv => tv.name || tv.address).join(", ") } });
      } else if (response.status === 404 && result) {
        finish("failed", { key: "send.noTv" });
      } else if (response.status === 400) {
        finish("failed", { key: "card.invalid" });
      } else {
        finish("failed", { key: "send.failed", vars: { detail: String(response.status) } });
      }
    } catch {
      finish("failed", { key: "send.appDown" });
    }
  }

  const label = t("card.button");
  const invalid = state === "failed" && (message?.key === "card.invalid" || message?.key === "card.invalidExample");
  return <>
    <p id={`${id}-title`} className={vis ? "visually-hidden" : "lr-client-launch-section lr-client-tv-section"}>
      {t("card.title")}
    </p>
    <section className="lr-client-tv" aria-labelledby={`${id}-title`}>
      <div className="lr-client-tv-row">
        <label className="lr-input">
          <Glyph name="link" size={18} />
          <input ref={input} type="text" inputMode="url" enterKeyHint="send" autoComplete="off"
            autoCapitalize="none" spellCheck={false} placeholder={t("card.placeholder")}
            aria-label={t("card.inputLabel")} aria-describedby={`${id}-hint ${id}-status`}
            aria-invalid={invalid || undefined}
            onInput={() => { if (state === "failed") { setState("idle"); setMessage(null); } }}
            onKeyDown={event => {
              // The card sits inside the launcher form: Enter sends the link, never starts a room.
              if (event.key === "Enter" && !event.nativeEvent.isComposing) {
                event.preventDefault();
                void send();
              }
            }} />
        </label>
        <button type="button" className="lr-btn is-primary" aria-label={label}
          aria-busy={state === "busy" || undefined} disabled={state === "busy"} onClick={() => void send()}>
          <Glyph name={state === "busy" ? "loader" : state === "sent" ? "check" : "tv"} size={19}
            className={state === "busy" ? "lr-spin" : undefined} />
          {vis ? null : <span className="lr-cap">{label}</span>}
        </button>
      </div>
      <p id={`${id}-status`} role="status"
        className={`lr-client-tv-status${state === "sent" ? " is-sent" : state === "failed" ? " is-failed" : ""}`}>
        {message ? t(message.key, message.vars) : ""}
      </p>
      <p id={`${id}-hint`} className={vis ? "visually-hidden" : undefined}>{t("card.hint")}</p>
    </section>
  </>;
}
