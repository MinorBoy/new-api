import {
  Activity,
  Box,
  ChartNoAxesCombined,
  CreditCard,
  FileText,
  FileUp,
  FlaskConical,
  Images,
  Key,
  LayoutDashboard,
  ListTodo,
  MessageSquare,
  Radio,
  ServerCog,
  Settings,
  Ticket,
  User,
  Users,
  Video,
  Wallet,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import type { SidebarData } from "@/components/layout/types";
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
} from "@/lib/admin-permissions";
import { ROLE } from "@/lib/roles";
import { getEnabledApiKeys, getApiKeyValue } from "@/features/keys/api";
import { getFreshAuthHeaders } from "@/lib/api";
import type { ApiKey } from "@/features/keys/types";

const studioUrl =
  import.meta.env.VITE_FLYREQ_STUDIO_URL || "http://127.0.0.1:3001";

type StudioProvider = {
  type: "image" | "video" | "text";
  provider?: "openai";
  protocol?: "new-api" | "openai";
  preset?: "gpt-image-2";
  modelKey: string;
  name: string;
  modelId: string;
  baseUrl: string;
  apiKey: string;
  maxRefImages?: number;
  maxOutputSize?: "4K";
  supportsTemperature?: boolean;
  streamImages?: boolean;
};

const imageModelPattern = /image|dall-e|gpt-image|imagen|seedream|jimeng|flux|recraft|ideogram|midjourney|nano.?banana|qwen.*image/i;
const videoModelPattern = /video|seedance|sora|kling|wan.?video|hailuo|veo|jimeng-video|doubao.*video/i;

function normalizeModelId(model: string): string {
  return model.trim();
}

function isModelAllowedForKey(key: ApiKey, model: string): boolean {
  const modelLimits = key.model_limits ?? "";
  if (!key.model_limits_enabled || !modelLimits.trim()) return true;
  const limits = modelLimits.split(/[\s,]+/).map(normalizeModelId).filter(Boolean);
  return limits.includes(model) || limits.some((limit) => limit.endsWith("*") && model.startsWith(limit.slice(0, -1)));
}

async function getModelsForGroup(baseUrl: string, group: string): Promise<string[]> {
  const response = await fetch(`${baseUrl}/api/user/models?group=${encodeURIComponent(group || "default")}`, {
    headers: await getFreshAuthHeaders(),
  });
  if (!response.ok) return [];
  const payload = (await response.json()) as { data?: unknown };
  return Array.isArray(payload.data)
    ? payload.data.filter((model): model is string => typeof model === "string").map(normalizeModelId).filter(Boolean)
    : [];
}

function buildStudioProviders(baseUrl: string, credentials: Array<{ key: ApiKey; value: string }>, modelsByGroup: Map<string, string[]>): StudioProvider[] {
  const providers: StudioProvider[] = [];
  for (const { key, value } of credentials) {
    const models = (modelsByGroup.get(key.group || "default") || []).filter((model) => isModelAllowedForKey(key, model));
    const imageModels = models.filter((model) => imageModelPattern.test(model));
    const videoModels = models.filter((model) => videoModelPattern.test(model));
    const textModels = models.filter((model) => !imageModelPattern.test(model) && !videoModelPattern.test(model));
    const keyPrefix = `new-api-${key.id}`;
    imageModels.forEach((model, index) => providers.push({
      type: "image",
      provider: "openai",
      preset: "gpt-image-2",
      modelKey: `${keyPrefix}-image-${index}`,
      name: `${key.name || `API Key ${key.id}`} / ${model}`,
      modelId: model,
      baseUrl,
      apiKey: value,
      maxRefImages: 16,
      maxOutputSize: "4K",
      supportsTemperature: false,
      streamImages: true,
    }));
    videoModels.forEach((model, index) => providers.push({
      type: "video",
      protocol: "new-api",
      modelKey: `${keyPrefix}-video-${index}`,
      name: `${key.name || `API Key ${key.id}`} / ${model}`,
      modelId: model,
      baseUrl,
      apiKey: value,
    }));
    textModels.forEach((model, index) => providers.push({
      type: "text",
      protocol: "openai",
      modelKey: `${keyPrefix}-text-${index}`,
      name: `${key.name || `API Key ${key.id}`} / ${model}`,
      modelId: model,
      baseUrl,
      apiKey: value,
    }));
  }
  return providers;
}

export async function prepareFlyreqStudioUrl(): Promise<string> {
  const baseUrl = window.location.origin;
  const keys = await getEnabledApiKeys();
  if (!keys.length) throw new Error("No enabled API key");
  const credentials = await Promise.all(keys.map(async (key) => ({ key, value: await getApiKeyValue(key.id) })));
  const groups = [...new Set(credentials.map(({ key }) => key.group || "default"))];
  const modelsByGroup = new Map(await Promise.all(groups.map(async (group) => [group, await getModelsForGroup(baseUrl, group)] as const)));
  const providers = buildStudioProviders(baseUrl, credentials, modelsByGroup);
  if (!providers.length) throw new Error("No enabled models available for the API keys");
  const url = new URL(`${studioUrl.replace(/\/$/, "")}/zh/`);
  for (const provider of providers) url.searchParams.append("provider", JSON.stringify(provider));
  return url.toString();
}

export async function openFlyreqStudioWithPreparation(
  prepareUrl: () => Promise<string> = prepareFlyreqStudioUrl,
  openWindow: (url: string, target?: string) => Window | null = (url, target) => window.open(url, target),
  targetStudioUrl: string = studioUrl,
): Promise<void> {
  // Reserve the tab on about:blank while the configuration loads. The blank
  // tab stays same-origin with this page, so the later location assignment is
  // always honored; assigning a provider URL to a tab that already runs the
  // studio makes the studio silently cancel that navigation, dropping the
  // configuration payload.
  const popup = openWindow("about:blank", "_blank");
  if (!popup) {
    console.error("Failed to open Lynwu Studio tab");
    return;
  }
  try {
    popup.location.href = await prepareUrl();
  } catch (error) {
    // Load the studio shell so the reserved tab stays usable for recovery
    // instead of sitting on about:blank after a transient API or
    // configuration failure.
    console.error("Failed to prepare Lynwu Studio configuration", error);
    try {
      popup.location.href = `${targetStudioUrl.replace(/\/$/, "")}/zh/`;
    } catch (navigateError) {
      console.error("Failed to open Lynwu Studio shell", navigateError);
    }
  }
}

function openFlyreqStudio(): void {
  void openFlyreqStudioWithPreparation();
}

/**
 * Root navigation groups for the application sidebar.
 *
 * These are shown when the URL does not match any nested sidebar view
 * registered in `layout/lib/sidebar-view-registry.ts`.
 */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation();

  return {
    navGroups: [
      {
        id: "chat",
        title: t("Chat"),
        items: [
          {
            title: t("Playground"),
            url: "/playground",
            icon: FlaskConical,
          },
          {
            title: t("Creative Studio"),
            url: "/video-generation",
            icon: Images,
            onClick: () => {
              void openFlyreqStudio();
            },
          },
          {
            title: t("Video generation"),
            url: "/video-generation",
            icon: Video,
          },
          {
            title: t("Asset library"),
            url: "/assets",
            icon: Images,
          },
          {
            title: t("Chat"),
            icon: MessageSquare,
            type: "chat-presets",
          },
        ],
      },
      {
        id: "general",
        title: t("General"),
        items: [
          {
            title: t("Overview"),
            url: "/dashboard/overview",
            icon: Activity,
          },
          {
            title: t("Dashboard"),
            url: "/dashboard/models",
            icon: LayoutDashboard,
          },
          {
            title: t("API Keys"),
            url: "/keys",
            icon: Key,
          },
          {
            title: t("Usage Logs"),
            url: "/usage-logs/common",
            icon: FileText,
          },
          {
            title: t("Task Logs"),
            url: "/usage-logs/task",
            activeUrls: ["/usage-logs/drawing"],
            configUrls: ["/usage-logs/drawing", "/usage-logs/task"],
            icon: ListTodo,
          },
        ],
      },
      {
        id: "personal",
        title: t("Personal"),
        items: [
          {
            title: t("Wallet"),
            url: "/wallet",
            icon: Wallet,
          },
          {
            title: t("Profile"),
            url: "/profile",
            icon: User,
          },
        ],
      },
      {
        id: "admin",
        title: t("Admin"),
        items: [
          {
            title: t("Channels"),
            url: "/channels",
            icon: Radio,
          },
          {
            title: t("Cost accounting"),
            url: "/cost-accounting",
            icon: ChartNoAxesCombined,
            requiredPermission: {
              resource: ADMIN_PERMISSION_RESOURCES.COST_ACCOUNTING,
              action: ADMIN_PERMISSION_ACTIONS.READ,
            },
          },
          {
            title: t("Config import"),
            url: "/config-import",
            icon: FileUp,
            requiredPermission: {
              resource: ADMIN_PERMISSION_RESOURCES.CONFIG_IMPORT,
              action: ADMIN_PERMISSION_ACTIONS.READ,
            },
          },
          {
            title: t("Models"),
            url: "/models/metadata",
            icon: Box,
          },
          {
            title: t("Users"),
            url: "/users",
            icon: Users,
          },
          {
            title: t("Redemption Codes"),
            url: "/redemption-codes",
            icon: Ticket,
          },
          {
            title: t("Subscriptions"),
            url: "/subscriptions",
            icon: CreditCard,
          },
          {
            title: t("System Info"),
            url: "/system-info",
            icon: ServerCog,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t("System Settings"),
            url: "/system-settings/site",
            activeUrls: ["/system-settings"],
            icon: Settings,
          },
        ],
      },
    ],
  };
}
