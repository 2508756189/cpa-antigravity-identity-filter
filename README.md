# cpa-antigravity-identity-filter

CLIProxyAPI v7 plugin (`antigravity-identity-filter`, version 1.0.0). For requests whose selected upstream format is Antigravity, it replaces only the third-party client identity paragraph at the start of the first system or developer prompt with the official Antigravity identity block. Every other byte is left as it arrived. Requests that are not targeting Antigravity are not modified.

The plugin id comes from the DLL file name. It is not `antigravity-coding-filter`.

## What is replaced

Only the first system/developer text, and only when `ToFormat` is `antigravity` (see below). The match is case-sensitive, anchored after any leading whitespace, and applied once. It consumes that first paragraph (through the next blank line) and leaves the blank line and everything after it.

Built-in prefixes, each with its own config toggle (default on):

- Codex: `You are Codex,` and `You are OpenAI Codex,`
- Claude Code: `You are Claude Code,` (including the Claude Agent SDK variant of that sentence)
- Claude Agent SDK: `You are a Claude agent,`
- ZCode: `You are ZCode,`
- Hermes: `You are Hermes Agent,`
- OpenCode: `You are OpenCode,` and `You are opencode`
- OpenClaw: `You are OpenClaw,`

Optional phrase, default on: the first `As Codex,` in that same first text becomes `As Antigravity,`. The rest of the Personality section is not rewritten.

If the text already starts with either official identity block, the paragraph is left alone.

Default identity (coding):

```text
You are Antigravity, a powerful agentic AI coding assistant designed by the Google Deepmind team working on Advanced Agentic Coding.
You are pair programming with a USER to solve their coding task. The task may require creating a new codebase, modifying or debugging an existing codebase, or simply answering a question.
```

`identity: generic` uses:

```text
You are Antigravity, a powerful agentic AI assistant designed by the Google Deepmind team.
You are working with a USER to solve their task.
```

Custom rules are anchored regular expressions (RE2) matched at that same start position. Omit `replacement` to insert the selected identity block. Set `paragraph: true` to consume the rest of the first paragraph. Custom rules run only when no enabled built-in matched, including when that built-in is turned off.

## What is not replaced

- Any request whose upstream format is not Antigravity, including the before-auth hook where `ToFormat` is still empty.
- Later system/developer messages, user/assistant/tool messages, tool schemas, and tool results.
- Paths (`.codex`, `$CODEX_HOME`), MCP and tool names (`mcp__codex_app…`), `codex://` links, `# Codex desktop context`, `AGENTS.md`, and the rest of a Personality section.
- A later `You are Codex,` inside the same first text, after the opening paragraph.
- Strings such as `you are codexify` or `You are Codexify` (the match is case-sensitive and requires the comma).

## Where the hook runs

Verified against CLIProxyAPI `v7.3.2`:

- `sdk/translator/formats.go` defines `FormatAntigravity` as `"antigravity"`.
- `sdk/cliproxy/auth/conductor_execution.go` sets that format in `requestToFormat` for the `antigravity` provider, then calls the after-auth interceptor with the inbound `req.Payload` before `executor.Execute`.
- `sdk/api/handlers/handlers_interceptors.go` copies `req.ToFormat.String()` into the plugin request.
- Translation into the Antigravity `request.systemInstruction` body happens inside the executor, after this plugin returns. For a Codex Responses client the identity therefore lives in the first `input[]` message with `role: developer` (or in `instructions` when that string is non-empty), not in a Gemini `systemInstruction` field yet. The plugin still accepts Chat, Anthropic, Gemini, and `{"request":{"systemInstruction":...}}` bodies in case that is what the hook receives.

`request.intercept_before` is a no-op. Comparison of `ToFormat` ignores surrounding space and ASCII case. The value CPA sends is the lowercase constant above.

## Evidence

Public reports (CLIProxyAPI issues 7049, 5037, and 4723, and jellyfish-p/cpa-plugin-antigravity-coding-filter issue 4) tie a fake HTTP 429 `RESOURCE_EXHAUSTED` to the third-party identity sentence. Replacing only that sentence has been observed to return 200. In the Antigravity `language_server` binary the same paragraph is wrapped in `LINT.IfChange(antigravity_identity)`, and the server-side abuse config tracks that text.

This does not prevent HTTP 403 or Terms of Service enforcement. It only rewrites the identity paragraph.

## Config

In `D:\Cliproxy\config.yaml`:

```yaml
plugins:
  enabled: true
  dir: "D:\\Cliproxy\\plugins"
  configs:
    antigravity-identity-filter:
      enabled: true
      priority: 1
      identity: coding          # coding (default) or generic
      replace_as_codex: true    # "As Codex," -> "As Antigravity,"
      clients:
        codex: true
        claude_code: true
        claude_agent_sdk: true
        zcode: true
        hermes: true
        opencode: true
        openclaw: true
      custom_rules:
        - pattern: '^You are ExampleAgent,'
          paragraph: true
          # replacement omitted: use the selected identity block
```

`enabled`, `priority`, and `store` are host fields and are ignored by the plugin. An unknown `clients` key is a config error, and the previous good config is kept.

## Install

CPA v7.3.2 looks in `plugins/windows/amd64` and then `plugins`.

1. Download `antigravity-identity-filter_*_windows_amd64.zip` from the GitHub Actions artifact on a PR or `main` run, or from a tagged Release (`v*`).
2. Copy `antigravity-identity-filter.dll` to `D:\Cliproxy\plugins\windows\amd64\`.
3. Add the config above and restart CLIProxyAPI.
4. `GET /v0/management/plugins` should show this plugin `registered: true` and `effective_enabled: true`, with version `1.0.0`.

The DLL is a Go `-buildmode=c-shared` library for `windows/amd64`, built with mingw-w64 (the same toolchain Go uses for c-shared Windows builds). It targets the CLIProxyAPI v7.3.2 plugin ABI (`github.com/router-for-me/CLIProxyAPI/v7 v7.3.2`).

Build on Windows, from this repo:

```powershell
$env:CGO_ENABLED = "1"
go build -trimpath -buildmode=c-shared -ldflags "-s -w" -o antigravity-identity-filter.dll .
Remove-Item antigravity-identity-filter.h
```

Cross-compile from Linux:

```bash
sudo apt-get install gcc-mingw-w64-x86-64
CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
  CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
  go build -trimpath -buildmode=c-shared -ldflags "-s -w" \
  -o antigravity-identity-filter.dll .
```

## Tests

```bash
go test ./...
go vet ./...
```

---

# cpa-antigravity-identity-filter（中文）

CLIProxyAPI v7 插件（名称 `antigravity-identity-filter`，版本 1.0.0）。仅当请求被路由到 Antigravity 凭证（`ToFormat` 为 `antigravity`）时，把第一条 system/developer 提示词开头的第三方客户端身份段落换成官方 Antigravity 身份块。其余字节原样保留。非 Antigravity 请求不会被修改。

请与旧插件 `antigravity-coding-filter` 区分开。DLL 文件名决定插件 id。

## 会替换什么

只处理第一条 system/developer 文本。规则区分大小写，锚定在可选空白之后，只应用一次，并吃掉第一段（直到下一个空行）。空行以及后面的内容不动。

内置前缀都可以单独关闭，默认开启：

- Codex：`You are Codex,` 与 `You are OpenAI Codex,`
- Claude Code：`You are Claude Code,`（含 “running within the Claude Agent SDK” 这句）
- Claude Agent SDK：`You are a Claude agent,`
- ZCode：`You are ZCode,`
- Hermes：`You are Hermes Agent,`
- OpenCode：`You are OpenCode,` 与 `You are opencode`
- OpenClaw：`You are OpenClaw,`

可选短语（默认开）：同一条文本里第一次出现的 `As Codex,` 换成 `As Antigravity,`。Personality 其余句子不动。

若文本已经以任一官方身份块开头，则不再改这段身份。

默认 `identity: coding` 写入上面的编码助手两句话。`identity: generic` 写入较短的通用助手两句话。

`custom_rules` 是锚定在同一起点的正则（RE2）。不写 `replacement` 时插入当前选定的身份块。`paragraph: true` 时连同第一段一起替换。只有没有内置规则命中时才会用自定义规则；把对应客户端关掉后，自定义规则仍然生效。

## 不会替换什么

- 上游不是 Antigravity 的请求。鉴权前的 `request.intercept_before` 也不会改，此时 `ToFormat` 仍为空。
- 第二条及之后的 system/developer、user/assistant/tool 消息、工具定义和工具结果。
- 路径（`.codex`、`$CODEX_HOME`）、工具名（`mcp__codex_app…`）、`codex://`、`# Codex desktop context`、`AGENTS.md`，以及 Personality 里除那一个短语以外的文字。
- 同一条文本里、第一段之后再次出现的 `You are Codex,`。
- `you are codexify`、`You are Codexify` 这类大小写不同或没有逗号的字符串。

## 依据

CLIProxyAPI issues 7049、5037、4723，以及 jellyfish-p/cpa-plugin-antigravity-coding-filter issue 4 的公开记录表明：第三方身份句会触发伪造的 HTTP 429 `RESOURCE_EXHAUSTED`；只改这一句时，响应可以变回 200。Antigravity `language_server` 二进制里这段文字包在 `LINT.IfChange(antigravity_identity)` 中，服务端 abuse 配置跟踪的就是这段文本。

这不能阻止 HTTP 403，也不能避开服务条款封禁。它只改身份段落。

## 配置

写在 `D:\Cliproxy\config.yaml`：

```yaml
plugins:
  enabled: true
  dir: "D:\\Cliproxy\\plugins"
  configs:
    antigravity-identity-filter:
      enabled: true
      priority: 1
      identity: coding
      replace_as_codex: true
      clients:
        codex: true
        claude_code: true
        claude_agent_sdk: true
        zcode: true
        hermes: true
        opencode: true
        openclaw: true
      custom_rules: []
```

CPA v7.3.2 会依次查找 `plugins\windows\amd64` 和 `plugins`。把 Release 或 Actions 产物 `antigravity-identity-filter_*_windows_amd64.zip` 里的 `antigravity-identity-filter.dll` 放到 `D:\Cliproxy\plugins\windows\amd64\`，重启后访问 `GET /v0/management/plugins`，确认 `registered: true`、`effective_enabled: true`、版本 `1.0.0`。

针对 v7.3.2 源码核对过的行为：`ToFormat` 在选定 Antigravity 凭证之后、请求体被翻译成 Gemini/`systemInstruction` 之前就已经是 `"antigravity"`。Codex Desktop 的开发者提示在入站的 Responses `input[].content[].text`（`instructions` 为空时），不在翻译后的字段里。插件对 Chat、Anthropic、Gemini 以及 `{"request":{"systemInstruction":...}}` 也做了同样的“只改第一处”处理。
