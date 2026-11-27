// managed by sag agent setup
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const scriptPattern =
  /(?:^|[;&|\n(`]|-c\s+["'])\s*(?:(?:sudo|env)\s+)*(?:[^\s;&|()`'"]*\/)?script(?=$|[\s"'`;&|)])/;

export function check(toolName: string, input: unknown): { block: true; reason: string } | undefined {
  if (toolName !== "bash") {
    return undefined;
  }
  const cmd = (input as { command?: unknown } | undefined)?.command;
  if (typeof cmd !== "string") {
    return undefined;
  }
  if (cmd.includes("sag credential delete")) {
    return { block: true, reason: "sag-guard: sag credential delete blocked" };
  }
  if (cmd.includes("podman secret rm")) {
    return { block: true, reason: "sag-guard: podman secret rm blocked" };
  }
  const m = scriptPattern.exec(cmd);
  if (m) {
    return { block: true, reason: `sag-guard: ${m[0]} blocked` };
  }
  return undefined;
}

export default function (pi: ExtensionAPI) {
  pi.on("tool_call", async (event) => check(event.toolName, event.input));
}
