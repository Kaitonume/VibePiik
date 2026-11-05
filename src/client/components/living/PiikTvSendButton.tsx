import { useEffect, useRef, useState } from "react";
import { Glyph } from "../../ui/icons";
import { usePiikTvCopy } from "../../lib/piiktv-copy";
import { Tooltip } from "./Tooltip";

type SendState = "idle" | "busy" | "sent" | "failed";

interface SendResult {
  sent?: { address: string; name?: string }[];
  error?: string;
}

/**
 * PiikTV: sends the room link to the Piik TV apps open on this LAN. The local App
 * finds them (TCP 8790) and delivers the link server side, so the page needs no
 * TV address and no cross-origin request.
 */
export function PiikTvSendButton({ link }: { link: string | null | undefined }) {
  const { t } = usePiikTvCopy();
  const [state, setState] = useState<SendState>("idle");
  const [detail, setDetail] = useState("");
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(timer.current), []);

  async function send(): Promise<void> {
    if (!link || state === "busy") return;
    window.clearTimeout(timer.current);
    setState("busy");
    try {
      const response = await fetch("/api/piik-tv/send", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ link }),
      });
      const result = await response.json().catch(() => null) as SendResult | null;
      if (response.ok && result?.sent?.length) {
        setDetail(t("send.sentTo", { names: result.sent.map(tv => tv.name || tv.address).join(", ") }));
        setState("sent");
      } else {
        setDetail(response.status === 404
          ? t("send.noTv")
          : t("send.failed", { detail: result?.error ?? String(response.status) }));
        setState("failed");
      }
    } catch {
      setDetail(t("send.appDown"));
      setState("failed");
    }
    timer.current = window.setTimeout(() => setState("idle"), 5000);
  }

  const title = state === "sent" || state === "failed" ? detail
    : link ? t("send.hint") : t("send.noRoom");
  return (
    <Tooltip kind={state === "failed" ? "warning" : "hint-copy-invite"} text={title} place="below" align="end">
      <button type="button" className="lr-btn is-compact lr-tv-send" aria-label={title}
        aria-busy={state === "busy" || undefined} disabled={!link || state === "busy"}
        onClick={() => void send()}>
        <Glyph name={state === "busy" ? "loader" : state === "sent" ? "check" : state === "failed" ? "alert" : "window"}
          size={16} className={state === "busy" ? "lr-spin" : undefined} />
        <span className="lr-cap">{state === "sent" ? t("send.sentLabel") : t("send.label")}</span>
      </button>
    </Tooltip>
  );
}
