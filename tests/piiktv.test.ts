import { describe, expect, it } from "vitest";
import { isPiikTvLanguage, piikTvText } from "../src/client/lib/piiktv-copy";
import { piikTvLink } from "../src/client/components/living/PiikTvLinkCard";

// VibePiik: the Piik TV features (header button and launcher card).
describe("Piik TV link entry", () => {
  it("accepts Piik room links and completes a missing scheme", () => {
    expect(piikTvLink(" https://a.trycloudflare.com/r/1234#v=AbCdEfGhIjKlMnOpQrStUA "))
      .toBe("https://a.trycloudflare.com/r/1234#v=AbCdEfGhIjKlMnOpQrStUA");
    expect(piikTvLink("http://192.168.0.20:8787/r/9999")).toBe("http://192.168.0.20:8787/r/9999");
    expect(piikTvLink("192.168.0.20:8787/r/1000")).toBe("https://192.168.0.20:8787/r/1000");
  });

  it("refuses anything that is not a Piik room", () => {
    for (const value of ["", "   ", "https://example.com/", "https://example.com/r/0123", "https://example.com/r/123",
      "ftp://example.com/r/1234", "javascript:alert(1)", "https://exa mple.com/r/1234",
      `https://example.com/r/1234${"a".repeat(1100)}`]) {
      expect(piikTvLink(value), value).toBeNull();
    }
  });
});

describe("Piik TV texts", () => {
  it("has English and Chinese with the same placeholders", () => {
    expect(piikTvText("en", "send.label")).toBe("Send URL to PIIK TV app");
    expect(piikTvText("zh", "card.button")).toBe("发送到电视");
    expect(piikTvText("en", "send.sentTo", { names: "TV, 192.168.0.9" })).toBe("Sent to TV, 192.168.0.9");
    expect(piikTvText("zh", "send.failed", { detail: "500" })).toBe("发送失败（500）");
  });

  it("only knows Piik's two languages", () => {
    expect(isPiikTvLanguage("en")).toBe(true);
    expect(isPiikTvLanguage("zh")).toBe(true);
    expect(isPiikTvLanguage("it")).toBe(false);
    expect(isPiikTvLanguage(undefined)).toBe(false);
  });
});