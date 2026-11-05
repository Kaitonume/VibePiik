import {
  qualitySettingsSchema,
  type QualitySettings,
} from "../../shared/protocol";
import { QUALITY_PROFILES, supportsCaptureProfile } from "../media/quality";

// PiikTV: new shares start on the "High FPS" preset (1080p, 60 fps, 12 Mbps, balanced).
export const DEFAULT_HOST_QUALITY: QualitySettings = QUALITY_PROFILES["1080p60"];

const STORAGE_KEY = "piik:quality-preference:v1";

export function readPreferredQuality(): QualitySettings {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    const result = qualitySettingsSchema.safeParse(stored ? JSON.parse(stored) : null);
    if (result.success && supportsCaptureProfile(result.data)) return result.data;
  } catch {
    // Storage may be unavailable; sharing still works with session settings.
  }
  return { ...DEFAULT_HOST_QUALITY, resolution: { ...DEFAULT_HOST_QUALITY.resolution } };
}

export function savePreferredQuality(settings: QualitySettings): void {
  const result = qualitySettingsSchema.safeParse(settings);
  if (!result.success) return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(result.data));
  } catch {
    // Saving a preference must not turn an applied capture change into a failure.
  }
}
