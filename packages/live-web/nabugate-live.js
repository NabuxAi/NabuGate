// @nabugate/live — realtime voice in the browser, through NabuGate.
//
// One file, no dependencies, ES module. The product's server owns the key
// and the actions; this handles the call:
//
//   1. microphone → RTCPeerConnection → SDP offer
//   2. signal(offer) — YOUR endpoint forwards it to NabuGate
//      POST /v1/live/sessions and returns the vendor's answer
//   3. audio runs browser ↔ vendor; JSON events on the "oai-events" channel
//   4. function calls → onToolCall(call) — YOUR server runs the action with
//      the signed-in user's rights and returns a result the model speaks from
//   5. usage seconds → onUsage — YOUR server relays them to
//      POST /v1/live/sessions/{id}/usage so the minutes are billed (or
//      reports its own clock instead; the gateway bills the highest snapshot)
//
// The gateway decides which vendor a live alias lands on, so the events of
// two vendors are understood and a call's first event says which it speaks:
// OpenAI's Realtime API (`session.created`), which is what the gateway opens
// through /v1/realtime/calls, and GPT-Live (`session.started`). Both sets of
// names are exported so a product can pin or override them.

export const DATA_CHANNEL = "oai-events";

// GPT-Live.
export const EVENTS = {
    started: "session.started",
    closed: "session.closed",
    usage: "session.usage.updated",
    inputTranscript: "session.input_transcript.delta",
    outputTranscript: "session.output_transcript.delta",
    responseEnvelope: "response.event",
    itemDone: "response.output_item.done",
    delegationCreated: "session.delegation.created",
    close: "session.close",
    itemCreate: "response.item.create",
    responseCreate: "response.create",
    thinkingAppend: "session.thinking.append",
    commentaryAppend: "session.commentary.append",
    instructionsAppend: "session.instructions.append",
};

// OpenAI's Realtime API, as its GA interface names them.
export const REALTIME_EVENTS = {
    created: "session.created",
    speechStarted: "input_audio_buffer.speech_started",
    inputTranscript: "conversation.item.input_audio_transcription.delta",
    inputTranscriptDone: "conversation.item.input_audio_transcription.completed",
    inputTranscriptFailed: "conversation.item.input_audio_transcription.failed",
    outputTranscript: "response.output_audio_transcript.delta",
    outputTranscriptDone: "response.output_audio_transcript.done",
    audioStarted: "output_audio_buffer.started",
    audioStopped: "output_audio_buffer.stopped",
    audioCleared: "output_audio_buffer.cleared",
    responseDone: "response.done",
    itemCreate: "conversation.item.create",
    responseCreate: "response.create",
    error: "error",
};

export function transcriptRoleFor(type) {
    if (type === EVENTS.inputTranscript) return "user";
    if (type === EVENTS.outputTranscript) return "agent";
    return null;
}

export function appendTranscriptDelta(turns, role, delta) {
    const text = String(delta ?? "");
    if (!text) return turns;
    const list = Array.isArray(turns) ? turns : [];
    const last = list[list.length - 1];
    if (last && last.role === role) return [...list.slice(0, -1), { ...last, text: last.text + text }];
    return [...list, { role, text, at: new Date().toISOString() }];
}

// The vendor reports a call's duration inside `event.usage` — on
// session.closed, and on session.usage.updated if it sends one. Its docs show
// that object without naming the field, so every plausible spelling is read,
// seconds first and then milliseconds, and an unknown shape is ignored rather
// than thrown on. Billing does not hang on this: a product that times the call
// itself (NabuCRM does) can report its own clock instead.
const USAGE_SECOND_FIELDS = ["seconds", "duration_seconds", "session_seconds", "total_seconds", "audio_seconds", "duration"];
const USAGE_MILLISECOND_FIELDS = ["duration_ms", "milliseconds", "total_ms"];

function finiteNumber(value) {
    if (typeof value === "number" && Number.isFinite(value)) return value;
    if (typeof value === "string" && value.trim() !== "" && Number.isFinite(Number(value))) return Number(value);
    return null;
}

export function usageSecondsFrom(event) {
    try {
        const usage = event && typeof event === "object" ? event.usage : null;
        if (!usage || typeof usage !== "object") return null;
        for (const key of USAGE_SECOND_FIELDS) {
            const nested = usage[key] && typeof usage[key] === "object" ? usage[key].seconds : usage[key];
            const n = finiteNumber(nested);
            if (n !== null) return Math.max(0, Math.floor(n));
        }
        for (const key of USAGE_MILLISECOND_FIELDS) {
            const n = finiteNumber(usage[key]);
            if (n !== null) return Math.max(0, Math.floor(n / 1000));
        }
    } catch {
        // An unrecognised usage shape is not an error: report nothing.
    }
    return null;
}

function argumentsOf(item) {
    if (typeof item.arguments === "string" && item.arguments.trim()) {
        try {
            return JSON.parse(item.arguments);
        } catch {
            return { _raw: item.arguments };
        }
    }
    return item.arguments && typeof item.arguments === "object" ? item.arguments : {};
}

export function functionCallFrom(event) {
    if (event?.type !== EVENTS.responseEnvelope) return null;
    const inner = event.event;
    if (inner?.type !== EVENTS.itemDone) return null;
    const item = inner.item;
    if (item?.type !== "function_call" || !item.call_id || !item.name) return null;
    return { callId: item.call_id, name: item.name, arguments: argumentsOf(item), delegationId: event.delegation_id || null };
}

// A Realtime response hands over its function calls whole in response.done.
// Answering them there, together, lets one response.create follow all of
// them; one per call would race the response that is still open.
export function realtimeFunctionCallsFrom(event) {
    if (event?.type !== REALTIME_EVENTS.responseDone) return [];
    const output = Array.isArray(event.response?.output) ? event.response.output : [];
    return output
        .filter((item) => item?.type === "function_call" && item.call_id && item.name && (item.status ?? "completed") === "completed")
        .map((item) => ({ callId: item.call_id, name: item.name, arguments: argumentsOf(item), delegationId: null }));
}

function outputText(output) {
    return typeof output === "string" ? output : JSON.stringify(output ?? {});
}

export function functionOutputEvents(callId, output) {
    return [
        { type: EVENTS.itemCreate, item: { type: "function_call_output", call_id: callId, output: outputText(output) } },
        { type: EVENTS.responseCreate },
    ];
}

export function realtimeOutputEvents(results) {
    return [
        ...results.map(({ callId, output }) => ({
            type: REALTIME_EVENTS.itemCreate,
            item: { type: "function_call_output", call_id: callId, output: outputText(output) },
        })),
        { type: REALTIME_EVENTS.responseCreate },
    ];
}

function waitForIceGathering(pc, timeoutMs = 1500) {
    if (pc.iceGatheringState === "complete") return Promise.resolve();
    return new Promise((resolve) => {
        const done = () => {
            pc.removeEventListener("icegatheringstatechange", check);
            resolve();
        };
        const check = () => pc.iceGatheringState === "complete" && done();
        pc.addEventListener("icegatheringstatechange", check);
        setTimeout(done, timeoutMs);
    });
}

/**
 * Open a call.
 *
 * @param {object} o
 * @param {(offerSdp: string) => Promise<{sdp_answer?: string, sdp?: string}>} o.signal
 * @param {(call: {callId:string,name:string,arguments:object}) => Promise<any>} [o.onToolCall]
 * @param {(delegation: {id:string}, reply: {think(t):void, say(t):void, instruct(t):void}) => void} [o.onDelegation]
 *   GPT-Live client-delegation mode: the product's own backend answers; reply with the three appends.
 * @param {(role:'user'|'agent', delta:string, turn:{item:string|null, final:boolean}) => void} [o.onTranscript]
 *   `turn.item` is the vendor's id for the utterance, when it gives one. The caller's words are
 *   transcribed after they stop talking, so they can arrive while the answer is already being
 *   spoken: on Realtime an empty delta opens their turn the moment they start, and keying turns
 *   by `item` keeps the transcript in the order things were said. `final` closes a turn; one of
 *   the caller's that closes empty was noise.
 * @param {(seconds:number) => void} [o.onUsage]
 * @param {(speaking:boolean) => void} [o.onSpeaking]
 * @param {() => void} [o.onConnect]
 * @param {(reason:string) => void} [o.onClose]
 * @param {(message:string) => void} [o.onError]
 * @param {(event:object) => void} [o.onEvent]
 * @param {boolean} [o.speakFirst] Realtime: the assistant opens the call rather than waiting to be spoken to.
 */
export async function connectLive({
    signal,
    onToolCall,
    onDelegation,
    onTranscript,
    onUsage,
    onSpeaking,
    onConnect,
    onClose,
    onError,
    onEvent,
    speakFirst = false,
    iceServers = [{ urls: "stun:stun.l.google.com:19302" }],
    audioElement,
} = {}) {
    if (typeof RTCPeerConnection === "undefined") throw new Error("This browser cannot make WebRTC calls.");
    const pc = new RTCPeerConnection({ iceServers });
    const audio = audioElement || document.createElement("audio");
    audio.autoplay = true;
    audio.setAttribute("playsinline", "");
    const mic = await navigator.mediaDevices.getUserMedia({ audio: true });
    for (const track of mic.getTracks()) pc.addTrack(track, mic);
    pc.ontrack = (event) => {
        audio.srcObject = event.streams[0];
        audio.play?.().catch(() => {});
    };
    const channel = pc.createDataChannel(DATA_CHANNEL);
    let closed = false;
    let resolveClosed;
    const closedPromise = new Promise((r) => (resolveClosed = r));
    let speakingTimer;
    // "realtime" or "gpt-live", read off the call's first event.
    let dialect = null;
    // The caller's Realtime turns whose words have streamed in, so the full
    // transcript that closes them is not written a second time.
    const heard = new Set();

    const send = (event) => {
        if (channel.readyState === "open") channel.send(JSON.stringify(event));
    };
    const teardown = (reason) => {
        if (closed) return;
        closed = true;
        clearTimeout(speakingTimer);
        try {
            channel.close();
        } catch {}
        for (const track of mic.getTracks()) track.stop();
        try {
            pc.close();
        } catch {}
        audio.srcObject = null;
        resolveClosed?.();
        onClose?.(reason);
    };

    const replyFor = (delegationId) => ({
        think: (content) => send({ type: EVENTS.thinkingAppend, delegation_id: delegationId, content: String(content) }),
        say: (content) => send({ type: EVENTS.commentaryAppend, delegation_id: delegationId, content: String(content) }),
        instruct: (content) =>
            send({ type: EVENTS.instructionsAppend, delegation_id: delegationId, content: String(content) }),
    });

    const run = (call) =>
        Promise.resolve()
            .then(() => (onToolCall ? onToolCall(call) : { error: "no tool handler" }))
            .catch((error) => ({ error: error?.message || "tool failed" }));

    const onRealtimeEvent = (event, type) => {
        const turn = (final) => ({ item: event.item_id || null, final });
        switch (type) {
            case REALTIME_EVENTS.speechStarted:
                return void onTranscript?.("user", "", turn(false));
            case REALTIME_EVENTS.inputTranscript:
                heard.add(event.item_id);
                return void onTranscript?.("user", event.delta || "", turn(false));
            case REALTIME_EVENTS.inputTranscriptDone:
                // A transcription model that does not stream sends the words only here.
                if (!heard.delete(event.item_id) && event.transcript) onTranscript?.("user", event.transcript, turn(false));
                return void onTranscript?.("user", "", turn(true));
            case REALTIME_EVENTS.inputTranscriptFailed:
                heard.delete(event.item_id);
                return void onTranscript?.("user", "", turn(true));
            case REALTIME_EVENTS.outputTranscript:
                return void onTranscript?.("agent", event.delta || "", turn(false));
            case REALTIME_EVENTS.outputTranscriptDone:
                return void onTranscript?.("agent", "", turn(true));
            case REALTIME_EVENTS.audioStarted:
                return void onSpeaking?.(true);
            case REALTIME_EVENTS.audioStopped:
            case REALTIME_EVENTS.audioCleared:
                return void onSpeaking?.(false);
            case REALTIME_EVENTS.responseDone: {
                const calls = realtimeFunctionCallsFrom(event);
                if (!calls.length) return;
                return void Promise.all(calls.map(run)).then((outputs) => {
                    const results = calls.map((call, i) => ({ callId: call.callId, output: outputs[i] }));
                    for (const out of realtimeOutputEvents(results)) send(out);
                });
            }
            case REALTIME_EVENTS.error:
                return void onError?.(event.error?.message || event.message || "Voice session error");
        }
    };

    channel.onmessage = (message) => {
        let event;
        try {
            event = JSON.parse(message.data);
        } catch {
            return;
        }
        onEvent?.(event);
        const type = event?.type || "";
        if (type === REALTIME_EVENTS.created || type === EVENTS.started) {
            dialect = type === EVENTS.started ? "gpt-live" : "realtime";
            onConnect?.();
            if (speakFirst && dialect === "realtime") send({ type: REALTIME_EVENTS.responseCreate });
            return;
        }
        if (dialect === "realtime") return void onRealtimeEvent(event, type);
        const role = transcriptRoleFor(type);
        if (role) {
            onTranscript?.(role, event.delta, { item: null, final: false });
            if (role === "agent") {
                onSpeaking?.(true);
                clearTimeout(speakingTimer);
                speakingTimer = setTimeout(() => onSpeaking?.(false), 900);
            } else onSpeaking?.(false);
            return;
        }
        const call = functionCallFrom(event);
        if (call) {
            run(call).then((result) => {
                for (const out of functionOutputEvents(call.callId, result)) send(out);
            });
            return;
        }
        if (type === EVENTS.delegationCreated && event.delegation?.id) {
            onDelegation?.(event.delegation, replyFor(event.delegation.id));
            return;
        }
        // Only the session's own events: a backend response's token usage
        // rides in response.* envelopes and is not a duration.
        if (type.startsWith("session.")) {
            const seconds = usageSecondsFrom(event);
            if (seconds !== null) onUsage?.(seconds);
        }
        if (type === EVENTS.closed) return void teardown(event.reason || "closed");
        if (type === "error" || type === "session.error") {
            onError?.(event?.error?.message || event?.message || "Voice session error");
        }
    };
    pc.onconnectionstatechange = () => {
        if (["failed", "closed", "disconnected"].includes(pc.connectionState)) {
            teardown(pc.connectionState === "failed" ? "connection_lost" : "disconnected");
        }
    };

    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    await waitForIceGathering(pc);
    let answer;
    try {
        answer = await signal(pc.localDescription?.sdp || offer.sdp);
    } catch (error) {
        teardown("signal_failed");
        throw error;
    }
    const answerSdp = answer?.sdp_answer || answer?.sdpAnswer || answer?.sdp;
    if (!answerSdp) {
        teardown("no_answer");
        throw new Error("The voice session returned no SDP answer.");
    }
    await pc.setRemoteDescription({ type: "answer", sdp: answerSdp });

    return {
        session: answer,
        send,
        instruct: (content) => replyFor(null).instruct(content),
        async close() {
            if (closed) return;
            // GPT-Live is hung up with an event and says goodbye first; a Realtime
            // call has no such event and ends when the connection does.
            if (dialect !== "realtime") {
                send({ type: EVENTS.close });
                await Promise.race([closedPromise, new Promise((r) => setTimeout(r, 2500))]);
            }
            teardown("close_requested");
        },
    };
}
