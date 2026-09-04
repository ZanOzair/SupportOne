# Explanations and the optional assistant

A verdict is not an answer. "disk.smart — urgent — the drive reports 142 reallocated sectors" tells someone that something is wrong and nothing about what to do, which is the state most diagnostic tools leave people in. This document covers the two tiers that close that gap, and why the second one is off.

## Tier 1: the offline explainer

Every verdict any compiled-in check can report is explained in plain language, with a short ordered list of things to try, from a table built into the binary.

It works with the network unplugged. It needs no API key, no account and no configuration. It costs nothing to run and gives the same answer every time for the same finding. **This is the tier that has to make the product useful on its own** — it is what the user gets when there is no model, no key and no connection, which is most of the time and, for many people, all of the time.

The phase gate is stated as a test rather than as a claim: a guard walks the shipped message catalog and fails if any check verdict has no rule, and fails the other way too if a rule explains a verdict no catalog carries.

### Three rules the design holds to

**Results that are fine are explained too.** A report that speaks up only about problems teaches people that silence means it did not look. `os.info` gets "this is what the computer is running; it is here so a technician does not have to ask", and its one step is "nothing to do".

**The explanation key is derived, not written twice.** `check.disk.smart.failing` is explained by `explain.disk.smart.failing`, computed rather than typed, so the two cannot drift apart.

**Nothing is offered that this build cannot do.** Every fix and walkthrough an explanation points at is resolved through the same registries the rest of the agent uses. An ID naming something not compiled in is dropped before the user sees it, and a Windows-only repair never appears on Linux.

### The step order is the advice

For a failing drive the steps are, in this order: back up anything you would hate to lose; stop using the machine for anything you cannot afford to lose; arrange for the drive to be replaced. Reversed, that is different and worse advice. Order is carried, not incidental.

A finding past what someone should work through alone — a drive reporting imminent failure, a machine that has gone a very long time without a security update, a log recording unexplained stops — carries a closing line saying so, and saying to hand the saved report to whoever supports the computer.

### Shared steps

Steps that say the same thing in different places are one string in one place. "Copy anything you would hate to lose to another drive or a cloud folder" is the same sentence whether a drive is failing or a backup has gone stale, and it reads identically in both.

## Tier 2: the optional assistant

**It is off.** Not disabled-by-default in the sense of a switch someone might flip without noticing: the agent contacts nothing unless a person enabled it, supplied an endpoint, saw the exact bytes that would leave the machine, and confirmed that specific send.

This is the only outbound connection SupportOne makes.

### Turning it on

```sh
# A local model: nothing leaves the computer at all.
./supportone-agent --text --assist \
  --assist-endpoint http://127.0.0.1:11434/v1/chat/completions \
  --assist-model llama3.2

# A hosted endpoint, with your own key in the environment.
export SUPPORTONE_ASSIST_KEY=...
./supportone-agent --text --assist \
  --assist-endpoint https://api.example.com/v1/chat/completions \
  --assist-model some-model
```

There is no default endpoint. `--assist` without `--assist-endpoint` is an error, not a fallback.

The endpoint speaks the OpenAI chat-completions shape, which is what Ollama, llama.cpp, LM Studio and most gateways already speak — so a local model needs no adapter and no key.

### The egress gate

| Rule | Why |
|---|---|
| HTTPS everywhere except this machine | Plain HTTP is allowed only on loopback, where a local model server lives and the traffic never leaves the computer. Refusing that case would push people towards a hosted service, which is the opposite of the point. |
| Prepare, then ask | Prepare builds the payload from the *redacted* snapshot and contacts nobody. Ask will not send without the token Prepare issued, and a token is good for one send. |
| The payload is shown in full | Not a summary of what would be sent — the bytes. A summary is not what the user is agreeing to. |
| Requests capped before sending, responses capped while reading | A snapshot larger than the cap is refused rather than truncated; a model that answers with a gigabyte does not get to exhaust memory. |
| Errors name the host, never the URL | A query string can carry a credential, and an HTTP error quotes the URL. |
| Every send reaches the audit log | With its byte count, whether it was redacted, and whether it was answered — including the sends that failed. |

The terminal path always redacts fully. Someone piping a report into a model endpoint is not sitting there weighing each field, and the protective choice is the one to make on their behalf when they are not asked. The browser lets them choose, starting fully redacted.

### What the model can and cannot do

**It cannot execute anything.** The only actionable thing it can return is a list of fix IDs, and those are resolved against the compiled-in registry before the user is offered them. A test hands it `rm -rf /`, an invented ID, and a Windows-only repair on Linux; all three are discarded at the boundary. The worst a manipulated model — or a machine's own data crafted to manipulate one, a malicious filename or a crafted log entry — can do is name an ID that does not resolve.

**Its prose stays its prose.** It is stripped of control characters (which is what would otherwise let a reply repaint a terminal), capped on a rune boundary, and shown as the model's words. It never becomes SupportOne's verdict, and it never replaces the Tier-1 explanation, which stands on its own beside it.

**Suggestions it names that do not exist are counted and shown.** A model that keeps naming repairs this build does not carry is worth knowing about, so the count is surfaced rather than swallowed.

The system prompt asks for plain language, forbids suggesting downloads or commands, and says not to manufacture concerns. That is not a security control and is not treated as one — the controls above hold whether or not the model pays it any attention.

### No credential is stored

The key is read from `SUPPORTONE_ASSIST_KEY` for the life of the process and is never written down: not to a config file, not to the OS keychain, not to the audit log.

This is a deliberate strengthening of the project's own "if keys are ever needed, use the OS keychain" rule. A tool that keeps your API key becomes a place your API key can leak from, and this one does not need to be that. Local model servers, which most people running this will use, need no key at all. If a future version wants a remembered key, the keychain is where it goes — but not storing one beats storing one well.

## Adding an explanation

1. A rule in `internal/explain/rules.go`, keyed by the verdict it answers.
2. Its cause in every catalog under `internal/i18n/locales`, at the key `CauseKey` derives.
3. Steps drawn from the shared set where one already says the right thing, or new shared step keys where none does.

The guard tests fail if any of those is missing, which is how "every check result explained offline" stays true rather than becoming true once.
