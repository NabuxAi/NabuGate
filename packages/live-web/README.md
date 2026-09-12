# @nabugate/live — live voice with actions, in any product

A phone-call-like conversation with an assistant that can *do things* in
your product — search, create, book, answer from your data — with the key
on your server and the minutes billed through NabuGate. Works the same in
NabuCRM (Laravel), NabuDesk (Laravel), NabuChat (Next.js), NabuPilot
(Node) and NabuHub (Go): three server endpoints and one browser call.

```
browser ──offer──▶ your server ──▶ NabuGate POST /v1/live/sessions ──▶ vendor
        ◀─answer──             ◀──                                  ◀──
browser ◀═══════════ audio + events, WebRTC, no gateway in the path ═══▶ vendor
browser ──tool call──▶ your server (runs it as the signed-in user) ──result──▶ browser ──▶ vendor
browser ──seconds───▶ your server ──▶ NabuGate POST /v1/live/sessions/{id}/usage
```

## 1. Browser

```html
<script type="module">
import { connectLive, appendTranscriptDelta } from "/vendor/nabugate-live.js";

let turns = [];
const call = await connectLive({
  signal: (sdp) => fetch("/api/voice/session", { method: "POST", headers: { "Content-Type": "application/json" },
                         body: JSON.stringify({ sdp }) }).then((r) => r.json()),
  onToolCall: (c) => fetch(`/api/voice/tool`, { method: "POST", headers: { "Content-Type": "application/json" },
                         body: JSON.stringify({ session_id: call.session.session_id, ...c }) })
                      .then((r) => r.json()).then((d) => d.output),
  onTranscript: (role, delta) => { turns = appendTranscriptDelta(turns, role, delta); },
  onUsage: (seconds) => navigator.sendBeacon("/api/voice/usage", JSON.stringify({ session_id: call.session.session_id, seconds })),
  onClose: (reason) => console.log("ended", reason),
});
// … later
await call.close();
</script>
```

`onToolCall` receives `{ callId, name, arguments }` and returns whatever the
model should read (a string or JSON). Throw, and the model is told the
action failed instead of stalling. `onDelegation` is the alternative when
your own backend does the thinking (client delegation): reply with
`think()`, `say()` and `instruct()`.

## 2. Your server — three endpoints

**Session** — build the session for *this* user and forward the offer:

```php
// Laravel (NabuCRM / NabuDesk)
Route::post('/api/voice/session', function (Request $r) {
    $user = $r->user();
    $res = Http::withToken(config('services.nabugate.key'))
        ->post(config('services.nabugate.url').'/live/sessions', [
            'model' => 'nabu-live',
            'session' => [
                'instructions' => "You are {$user->workspace->name}'s assistant. Speak the caller's language. Act through the tools; never claim an action happened unless the tool returned ok.",
                'audio' => ['output' => ['voice' => 'quartz']],
                'delegation' => ['type' => 'responses', 'responses' => [
                    'model' => 'gpt-5.6-luna',
                    'tools' => VoiceTools::definitions($user),   // OpenAI function-tool JSON
                    'tool_choice' => 'auto',
                ]],
            ],
            'transport' => ['type' => 'webrtc', 'sdp' => $r->input('sdp')],
        ])->throw()->json();
    $session = VoiceSession::create(['user_id' => $user->id, 'external_id' => $res['session']['id']]);
    return ['session_id' => $session->id, 'sdp_answer' => $res['transport']['sdp']];
});
```

**Tool** — run the action with the caller's own permissions and return text:

```php
Route::post('/api/voice/tool', function (Request $r) {
    $session = VoiceSession::where('id', $r->input('session_id'))->where('user_id', $r->user()->id)->firstOrFail();
    return ['output' => VoiceTools::run($r->user(), $r->input('name'), $r->input('arguments', []))];
});
```

**Usage** — relay the seconds the browser saw so the gateway bills them:

```php
Route::post('/api/voice/usage', function (Request $r) {
    $session = VoiceSession::findOrFail($r->input('session_id'));
    Http::withToken(config('services.nabugate.key'))
        ->post(config('services.nabugate.url')."/live/sessions/{$session->external_id}/usage",
               ['seconds' => (int) $r->input('seconds'), 'final' => (bool) $r->input('final', false)]);
    return ['ok' => true];
});
```

`onUsage` fires when the vendor's `session.*` events carry a duration. The
vendor documents a `usage` object on `session.closed` without naming its field,
so the SDK reads several spellings and stays silent on one it does not know.
If you already time the call yourself — NabuCRM does — report your own clock
instead; the gateway bills the highest snapshot it has seen, never twice.

Same three in Next.js route handlers, an Express router or a Go handler —
the bodies are identical. NabuGate meters the minutes against the key's
owner (see `docs/live.md`); what you charge *your* customer is your call.

## 3. Tools that act

A tool is an OpenAI function definition plus a handler that runs inside
your product. The model only ever sees the definition and the result:

```json
{ "type": "function", "name": "create_ticket",
  "description": "Open a support ticket for the caller. Confirm the ticket number to them.",
  "parameters": { "type": "object", "properties": { "subject": {"type":"string"}, "priority": {"type":"string","enum":["low","normal","high"]} },
                  "required": ["subject"], "additionalProperties": false } }
```

Rules that keep this safe:

- Run the handler **as the signed-in user** with their normal authorisation;
  the model never gets a key, a token or a row it could not see in the UI.
- Treat tool results as data; tell the model so in the instructions.
- Return `{"error": "…"}` on failure so the model reports it rather than
  inventing an outcome.
- Bound the number of calls per session (Zooey uses 40).

Zooey's reference implementation: `src/domains/voice_tools.rs` (search
listings, save lead, request callback) and `frontend/src/lib/voice/gptLive.js`.

## Costs

See `docs/live-costs.md` for per-minute numbers per engine.
