// managed by sag agent setup
import type { Extension, ToolCallEvent } from "@earendil-works/pi-coding-agent";

const scriptPattern =
  /(?:^|[;&|\n(`]|-c\s+["'])\s*(?:(?:sudo|env)\s+)*(?:[^\s;&|()`'"]*\/)?script(?=$|[\s"'`;&|)])/;

const guard: Extension = {
  async tool_call(event: ToolCallEvent) {
    if (event.tool.name !== "bash") {
      return;
    }
    const cmd: string = (event.input as Record<string, string>)?.command ?? "";
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
  },
};

export default guard;
