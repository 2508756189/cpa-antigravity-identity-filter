# cpa-antigravity-identity-filter

CLIProxyAPI v7 plugin (`antigravity-identity-filter`, version 1.0.0). For requests whose selected upstream format is Antigravity, it replaces an explicit third-party client identity sentence with the official Antigravity identity block. The match is that sentence only. Guidance after the sentence stays, including guidance in the same paragraph. Bytes outside the matched span stay as they arrived. Requests that are not targeting Antigravity are not modified.

The plugin id comes from the DLL file name. It is not `antigravity-coding-filter`.

## What is replaced

By default only the first system or developer text that contains something other than whitespace, and only when `ToFormat` is `antigravity`. An empty or whitespace-only `instructions` value, text block, or message is skipped. The match is case-sensitive, anchored after any leading whitespace, and applied once per text.

A match is one sentence: it starts with a known prefix and ends at the first period that is followed by whitespace or the end of the text. It does not continue to the next blank line. A declaration with no sentence-ending period, a newline before that period, or a path-like token is left untouched.

Built-in prefixes, each with its own config toggle (default on):

- Codex: `You are Codex,` and `You are OpenAI Codex,`
- Claude Code: `You are Claude Code,` (including the Claude Agent SDK variant of that sentence)
- Claude Agent SDK: `You are a Claude agent,`
- ZCode: `You are ZCode,`
- Hermes: `You are Hermes Agent,`
- OpenCode: `You are OpenCode,` and `You are opencode`
- OpenClaw: `You are OpenClaw,`

Rule order is fixed: `openai-codex`, `codex`, `claude-code`, `claude-agent`, `zcode`, `hermes`, `opencode`, `opencode-lower`, `openclaw`, then custom rules. The first match wins. Custom rules run only when no enabled built-in matched, including when that built-in is turned off.

Optional phrase, default on, initial scope only: the first `As Codex,` in that same first text becomes `As Antigravity,`. It does not run in later messages or tool descriptions. Turning `scopes.initial` off skips this phrase as well.

If the text already starts with either official identity block, the identity sentence is left alone. If the selected block already appears anywhere in that text, a matched third-party sentence is removed and a second copy of the block is not inserted. Running the plugin again on its own output does not add another block.

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

Custom rules are regular expressions (RE2) matched at that same start position. Omit `replacement` to insert the selected identity block. `paragraph: true` extends the match to the end of the same sentence. It does not consume through the next blank line, and the rule does not apply when that sentence bound is missing.

## Scopes

`scopes.initial` (default true) is the first non-empty system or developer text. That is the historical position: Responses `instructions`, otherwise the first `input` or `messages` entry with role `system` or `developer`, Anthropic `system`, Gemini `systemInstruction` / `system_instruction`, or the same fields under `request`.

`scopes.subsequent_system` (default false) also rewrites the first non-empty text of each later message whose role is exactly `system` or `developer`. A user or assistant message is not a system message. A user message that merely contains `<system-reminder>` is not authority and is never rewritten. Sibling text blocks inside the first system value are not a separate reminder scope; they stay unchanged.

`scopes.tool_descriptions` (default false) rewrites only a tool's own `description` string: Chat Completions `tools[].function.description`, Responses and Anthropic `tools[].description`, and Gemini `tools[].functionDeclarations[].description`, including those fields under `request`. Tool names, parameter schemas, and descriptions nested inside parameters are not rewritten.

Optional neutralization runs only inside a scope that is already enabled:

- `neutralize.desktop_context` (default false) replaces a line that is exactly `# Codex desktop context` with `# Desktop context`. A longer line, and paths such as `.codex` or `.zcode`, are not matches.
- `neutralize.powered_by_model` (default false) replaces a sentence that is exactly `You are powered by the model named <token>.` with `You are powered by the model named Antigravity.` The token may contain internal dots (`gpt-5.3-codex`). A missing period leaves the text unchanged.

Neither rule scans user or assistant history, and neither replaces bare words such as Codex, GPT, or OpenAI.

## What is not replaced

- Any request whose upstream format is not Antigravity, including the before-auth hook where `ToFormat` is still empty.
- Every scope that is left at its default off, and the whole payload when `scopes.initial` is turned off and the other scopes stay off.
- User, assistant, and tool-result messages, including code or history that quotes an identity sentence or contains `<system-reminder>`.
- Tool names and parameter schemas.
- Paths (`.codex`, `.zcode`, `$CODEX_HOME`), MCP and tool names (`mcp__codex_app…`), `codex://` links, `AGENTS.md`, and a `# Codex desktop context` line unless that optional rule is enabled.
- A later identity sentence in the same text, after the opening sentence.
- Strings such as `you are codexify` or `You are Codexify` (the match is case-sensitive).
- A declaration that is not a single bounded sentence.

## Limits

- The identity sentence must be at most 320 bytes, must stay on one line, and must end in a period.
- The sentence is not matched when it contains `/`, `\`, `<`, `>`, `.codex`, `.zcode`, or `://`.
- The sentence must start at the beginning of the eligible text, after leading whitespace. Leading words make it unrecognized.
- One identity replacement per eligible text. Repeated application is stable.
- `subsequent_system` sees the first non-empty text of a later system or developer message, not every content block in that message.
- Unsupported shapes are left unchanged: non-object JSON, invalid JSON, a non-array `tools` value, and the before-auth hook. If an enabled tool or later-message region is malformed, the whole body is left unchanged.
- This plugin does not walk every string in the payload and does not replace product names globally.

## Where the hook runs

Verified against CLIProxyAPI `v7.3.2`:

- `sdk/translator/formats.go` defines `FormatAntigravity` as `"antigravity"`.
- `sdk/cliproxy/auth/conductor_execution.go` sets that format in `requestToFormat` for the `antigravity` provider, then calls the after-auth interceptor with the inbound `req.Payload` before `executor.Execute`.
- `sdk/api/handlers/handlers_interceptors.go` copies `req.ToFormat.String()` into the plugin request.
- Translation into the Antigravity `request.systemInstruction` body happens inside the executor, after this plugin returns. For a Codex Responses client the identity therefore lives in the first `input[]` message with `role: developer` (or in `instructions` when that string is non-empty), not in a Gemini `systemInstruction` field yet. The plugin still accepts Chat, Anthropic, Gemini, and `{"request":{"systemInstruction":...}}` bodies in case that is what the hook receives.

`request.intercept_before` is a no-op. Comparison of `ToFormat` ignores surrounding space and ASCII case. The value CPA sends is the lowercase constant above.

`enabled`, `priority`, and `store` are host fields. The host skips a disabled plugin. This plugin ignores those keys if it is still invoked; disabling a scope above is what makes an invoked plugin pass the payload through.

## CPA sensitive words

This plugin replaces exact identity sentences. It does not obfuscate arbitrary substrings and it does not edit the host config.

CLIProxyAPI v7.3.2 can separately obfuscate configured substrings in the translated Antigravity system instruction (`antigravity.sensitive-words` in the host config, applied by the executor after this plugin returns). That host list can cover phrases this plugin does not treat as an identity sentence. Leave it unset if you do not want that extra pass. The two settings do not call each other.

## Diagnostics

`diagnostics` defaults to false. When true, each applied rule is reported through the existing `host.log` callback as rule id, scope, and change count. The log fields are only `rule_id`, `scope`, and `count`. The plugin does not log the prompt, the matched text, the replacement, headers, credentials, paths, usernames, or the request body. With no host API (unit tests, or before `cliproxy_plugin_init`), the callback is skipped.

## Evidence

Public reports (CLIProxyAPI issues 7049, 5037, and 4723, and jellyfish-p/cpa-plugin-antigravity-coding-filter issue 4) discuss a fake HTTP 429 `RESOURCE_EXHAUSTED` alongside a third-party identity sentence. This plugin does not claim to fix HTTP 429. A 429 can have other causes, and the sentence bound in this version is a reliability fix so a match cannot delete the following guidance. It also does not prevent HTTP 403 or Terms of Service enforcement.

## Config

In `D:\Cliproxy\config.yaml`:

```yaml
plugins:
  enabled: true
  dir: "D:\\Cliproxy\\plugins"
  configs:
    antigravity-identity-filter:
      enabled: true                 # host field, ignored by the plugin
      priority: 1                   # host field
      identity: coding              # coding (default) or generic
      replace_as_codex: true        # initial text only: "As Codex," -> "As Antigravity,"
      diagnostics: false            # rule id, scope, and count only
      clients:
        codex: true
        claude_code: true
        claude_agent_sdk: true
        zcode: true
        hermes: true
        opencode: true
        openclaw: true
      scopes:
        initial: true               # default
        subsequent_system: false    # later role=system|developer messages
        tool_descriptions: false    # tool description fields only
      neutralize:
        desktop_context: false      # exact "# Codex desktop context" line
        powered_by_model: false     # exact "You are powered by the model named …." sentence
      custom_rules:
        - pattern: '^You are ExampleAgent,'
          paragraph: true           # sentence bound, not the next blank line
          # replacement omitted: use the selected identity block
```

`enabled`, `priority`, and `store` are host fields and are ignored by the plugin. An unknown `clients`, `scopes`, or `neutralize` key is a config error, and the previous good config is kept.

## Install

CPA v7.3.2 looks in `plugins/windows/amd64` and then `plugins`.

1. Download `antigravity-identity-filter_*_windows_amd64.zip` from the GitHub Actions artifact on a PR or `main` run, or from a tagged Release (`v*`).
2. Copy `antigravity-identity-filter.dll` to `D:\Cliproxy\plugins\windows\amd64\`.
3. Add the config above and restart CLIProxyAPI.
4. `GET /v0/management/plugins` should show this plugin `registered: true` and `effective_enabled: true`, with version `1.0.0`.

The DLL is a Go `-buildmode=c-shared` library for `windows/amd64`, built with mingw-w64 (the same toolchain Go uses for c-shared Windows builds). It targets the CLIProxyAPI v7.3.2 plugin ABI (`github.com/router-for-me/CLIProxyAPI/v7 v7.3.2`).

These steps are documentation. Building or merging this branch does not copy the DLL onto a machine or enable the plugin.

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

## Rollback

To undo a copy you installed yourself, remove `antigravity-identity-filter.dll` from the CPA plugins directory, or set `plugins.configs.antigravity-identity-filter.enabled` to `false`, then restart CLIProxyAPI. Confirm with `GET /v0/management/plugins` that the plugin is no longer effective. The release workflow in this repo is unchanged. Do not retag or publish a release from a feature branch unless you mean to.

## Tests

```bash
go test ./...
go test -race ./...
go vet ./...
go test ./.github/scripts
go vet ./.github/scripts
```

`go test` runs the fuzz seed corpus. `go test -fuzz` is optional and is not part of CI.

---

# cpa-antigravity-identity-filter（中文）

CLIProxyAPI v7 插件（名称 `antigravity-identity-filter`，版本 1.0.0）。仅当请求被路由到 Antigravity（`ToFormat` 为 `antigravity`）时，把明确的第三方客户端身份句换成官方 Antigravity 身份块。只替换这一句。句子后面的执行说明会保留，即使它和身份句在同一段、中间没有空行。匹配范围以外的字节原样保留。非 Antigravity 请求不会被修改。

请与旧插件 `antigravity-coding-filter` 区分开。DLL 文件名决定插件 id。

## 会替换什么

默认只处理第一条含有非空白内容的 system/developer 文本。全空或只有空白的 `instructions`、文本块或消息会跳过。规则区分大小写，锚定在可选空白之后，每段文本只替换一次。

匹配范围是一句：以已知前缀开头，结束于后跟空白或文本末尾的第一个句号。不再吃到下一个空行。没有句号、句号前出现换行、或句子里带有路径样式的声明会原样保留。

内置前缀都可以单独关闭，默认开启：

- Codex：`You are Codex,` 与 `You are OpenAI Codex,`
- Claude Code：`You are Claude Code,`（含 “running within the Claude Agent SDK” 这句）
- Claude Agent SDK：`You are a Claude agent,`
- ZCode：`You are ZCode,`
- Hermes：`You are Hermes Agent,`
- OpenCode：`You are OpenCode,` 与 `You are opencode`
- OpenClaw：`You are OpenClaw,`

顺序固定，先命中的规则生效。自定义规则只在没有内置规则命中时运行；把对应客户端关掉后，自定义规则仍然可以生效。

`replace_as_codex` 默认开，且只作用于初始文本里第一次出现的 `As Codex,`，把它换成 `As Antigravity,`。后面的消息和工具描述不会做这个替换。`scopes.initial` 关掉时，这个短语也不会替换。

文本若已经以任一官方身份块开头，则不再改身份句。这段文本里若已经有选定的官方块，则只删掉命中的第三方句子，不会再插入第二份。对插件自己的输出再跑一次是稳定的。

默认 `identity: coding` 写入上面的编码助手两句话。`identity: generic` 写入较短的通用助手两句话。

`custom_rules` 是锚定在同一起点的正则（RE2）。不写 `replacement` 时插入当前选定的身份块。`paragraph: true` 只把匹配延长到同一句的句号，不再吃到空行；找不到这个句界时，该规则不生效。

## 作用范围

`scopes.initial` 默认 true，即第一条 system/developer 文本。

`scopes.subsequent_system` 默认 false。打开后，还会改写后面每条角色恰好是 `system` 或 `developer` 的消息里的第一段非空文本。user/assistant 不是系统消息。用户消息里出现 `<system-reminder>` 并不构成权限，不会被改写。第一条 system 内容里的后续文本块也不是这个范围。

`scopes.tool_descriptions` 默认 false。只改写工具自己的 `description`：Chat Completions 的 `tools[].function.description`、Responses/Anthropic 的 `tools[].description`、Gemini 的 `tools[].functionDeclarations[].description`。不改工具名、参数 schema，也不改参数内部的 description。

可选中和规则只在已经打开的范围里运行，默认都关闭：

- `neutralize.desktop_context`：整行恰好是 `# Codex desktop context` 时换成 `# Desktop context`。更长的行以及 `.codex`、`.zcode` 路径都不匹配。
- `neutralize.powered_by_model`：恰好是 `You are powered by the model named <token>.` 这一句时，换成 `You are powered by the model named Antigravity.`。token 可以含内部句点。没有结尾句号则不改。

这两条规则不扫描 user/assistant 历史，也不全局替换 Codex、GPT、OpenAI 这几个词。

## 不会替换什么

- 上游不是 Antigravity 的请求。鉴权前的 `request.intercept_before` 也不会改。
- 保持默认关闭的范围。`scopes.initial` 关掉且其他范围仍关闭时，整个请求原样通过。
- user、assistant、工具结果，包括其中引用的身份句、代码，以及 `<system-reminder>`。
- 工具名和参数 schema。
- 路径（`.codex`、`.zcode`、`$CODEX_HOME`）、工具名、`codex://`、`AGENTS.md`。未打开对应开关时，`# Codex desktop context` 也不改。
- 同一段文本里、首句之后再次出现的身份句。
- `you are codexify`、`You are Codexify` 这类大小写不同的字符串。
- 不能确定为单句的声明。

## 限制

身份句最长 320 字节，必须单行并以句号结束。句子中出现 `/`、`\`、`<`、`>`、`.codex`、`.zcode` 或 `://` 时不匹配。句首不能有别的文字。每段文本只做一次身份替换。`subsequent_system` 只看后续 system/developer 消息的第一段非空文本。非对象 JSON、非法 JSON、非数组的 `tools`，以及鉴权前的钩子都保持不变。启用的工具区或后续消息区如果结构损坏，整个 body 保持不变。

## 与 CPA 敏感词的关系

本插件只替换精确的身份句，不做任意子串混淆，也不修改宿主机配置。

CLIProxyAPI v7.3.2 的 `antigravity.sensitive-words` 由执行器在本插件返回之后、翻译好的 system instruction 上单独混淆。宿主名单可以补充本插件不视为身份句的短语。不需要这一步时不要配置它。两处设置互不调用。

## 诊断

`diagnostics` 默认 false。打开后通过已有的 `host.log` 只记录规则 id、范围和变更次数，字段只有 `rule_id`、`scope`、`count`。不会记录提示词、命中文本、替换文本、header、凭据、路径、用户名或请求体。

## 关于 429

CLIProxyAPI issues 7049、5037、4723，以及 jellyfish-p/cpa-plugin-antigravity-coding-filter issue 4 的公开讨论把伪造的 HTTP 429 `RESOURCE_EXHAUSTED` 和第三方身份句放在一起。本插件不声称能修复 HTTP 429。429 还可以有别的原因。这里的句界是可靠性修复，避免一次匹配删掉后面的执行说明。它也不能阻止 HTTP 403 或服务条款限制。

## 配置

写在 `D:\Cliproxy\config.yaml`。字段与英文示例相同：`identity`、`replace_as_codex`、`diagnostics`、`clients`、`scopes`（`initial` 默认 true，`subsequent_system` 与 `tool_descriptions` 默认 false）、`neutralize`（`desktop_context` 与 `powered_by_model` 默认 false）、`custom_rules`。`enabled`、`priority`、`store` 是宿主字段，插件忽略。`clients`、`scopes`、`neutralize` 里的未知键是配置错误，会保留上一份有效配置。

安装、构建和回滚见上面的英文小节。把 DLL 拷进插件目录或设置 `enabled: false` 都是你自己的操作；合并本分支不会在任何机器上启用插件。回滚时删掉 DLL 或关掉宿主配置里的 `enabled`，重启 CLIProxyAPI，再用 `GET /v0/management/plugins` 确认。本仓库的发布流程没有改动。
