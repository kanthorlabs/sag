import test from "node:test";
import assert from "node:assert";
import guard from "./sag-guard.ts";

function toolCall(command: string) {
  return guard.tool_call(
    { tool: { name: "bash" }, input: { command } } as any,
    {} as any,
  );
}

function otherToolCall(toolName: string, command: string) {
  return guard.tool_call(
    { tool: { name: toolName }, input: { command } } as any,
    {} as any,
  );
}

test("blocks sag credential delete anywhere", async () => {
  const r = await toolCall("sag credential delete x");
  assert.strictEqual(r?.block, true);
  assert.strictEqual(r?.reason, "sag-guard: sag credential delete blocked");
});

test("blocks sag credential delete embedded in sh -c", async () => {
  const r = await toolCall('sh -c "sag credential delete y"');
  assert.strictEqual(r?.block, true);
  assert.strictEqual(r?.reason, "sag-guard: sag credential delete blocked");
});

test("blocks podman secret rm anywhere", async () => {
  const r = await toolCall('sh -c "podman secret rm x"');
  assert.strictEqual(r?.block, true);
  assert.strictEqual(r?.reason, "sag-guard: podman secret rm blocked");
});

test("blocks script -q at start", async () => {
  const r = await toolCall("script -q");
  assert.strictEqual(r?.block, true);
  assert.match(r?.reason ?? "", /sag-guard:.*blocked/);
});

test("blocks /usr/bin/script", async () => {
  const r = await toolCall("/usr/bin/script");
  assert.strictEqual(r?.block, true);
  assert.match(r?.reason ?? "", /sag-guard:.*blocked/);
});

test("blocks sh -c script ...", async () => {
  const r = await toolCall('sh -c "script ..."');
  assert.strictEqual(r?.block, true);
  assert.match(r?.reason ?? "", /sag-guard:.*blocked/);
});

test("blocks bash -c script ...", async () => {
  const r = await toolCall("bash -c 'script ...'");
  assert.strictEqual(r?.block, true);
  assert.match(r?.reason ?? "", /sag-guard:.*blocked/);
});

test("blocks sudo script", async () => {
  const r = await toolCall("sudo script");
  assert.strictEqual(r?.block, true);
  assert.match(r?.reason ?? "", /sag-guard:.*blocked/);
});

test("blocks cmd & script ...", async () => {
  const r = await toolCall("cmd & script ...");
  assert.strictEqual(r?.block, true);
  assert.match(r?.reason ?? "", /sag-guard:.*blocked/);
});

test("blocks ls then script -q on new line", async () => {
  const r = await toolCall("ls\nscript -q");
  assert.strictEqual(r?.block, true);
  assert.match(r?.reason ?? "", /sag-guard:.*blocked/s);
});

test("passes ls -la", async () => {
  const r = await toolCall("ls -la");
  assert.strictEqual(r, undefined);
});

test("passes ls scripts/", async () => {
  const r = await toolCall("ls scripts/");
  assert.strictEqual(r, undefined);
});

test("passes npx typescript", async () => {
  const r = await toolCall("npx typescript");
  assert.strictEqual(r, undefined);
});

test("passes go test ./e2e/...", async () => {
  const r = await toolCall("go test ./e2e/...");
  assert.strictEqual(r, undefined);
});

test("passes git commit -m script fix", async () => {
  const r = await toolCall('git commit -m "script fix"');
  assert.strictEqual(r, undefined);
});

test("passes grep script x", async () => {
  const r = await toolCall('"grep "script" x"');
  assert.strictEqual(r, undefined);
});

test("passes cat docs/script", async () => {
  const r = await toolCall("cat docs/script");
  assert.strictEqual(r, undefined);
});

test("passes other tool with sag credential delete in input", async () => {
  const r = await otherToolCall("write_file", "sag credential delete x");
  assert.strictEqual(r, undefined);
});

test("passes other tool call with script command", async () => {
  const r = await otherToolCall("read_file", "script -q");
  assert.strictEqual(r, undefined);
});
