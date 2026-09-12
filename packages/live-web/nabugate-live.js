// @nabugate/live — realtime voice (GPT-Live) in the browser, through NabuGate.
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
// Event names follow the vendor's GPT-Live contract at the time of writing;
// they are exported so a product can pin or override them.

export const DATA_CHANNEL = "oai-events";
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

export function functionCallFrom(event) {
    if (event?.type !== EVENTS.responseEnvelope) return null;
    const inner = event.event;
    if (inner?.type !== EVENTS.itemDone) return null;
    const item = inner.item;
    if (item?.type !== "function_call" || !item.call_id || !item.name) return null;
    let args = {};
    if (typeof item.arguments === "string" && item.arguments.trim()) {
        try {
            args = JSON.parse(item.arguments);
        } catch {
            args = { _raw: item.arguments };
        }
    } else if (item.arguments && typeof item.arguments === "object") {
        args = item.arguments;
    }
    return { callId: item.call_id, name: item.name, arguments: args, delegationId: event.delegation_id || null };
}

export function functionOutputEvents(callId, output) {
    const text = typeof output === "string" ? output : JSON.stringify(output ?? {});
    return [
        { type: EVENTS.itemCreate, item: { type: "function_call_output", call_id: callId, output: text } },
        { type: EVENTS.responseCreate },
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
 *   client-delegation mode: the product's own backend answers; reply with the three appends.
 * @param {(role:'user'|'agent', delta:string) => void} [o.onTranscript]
 * @param {(seconds:number) => void} [o.onUsage]
 * @param {(speaking:boolean) => void} [o.onSpeaking]
 * @param {() => void} [o.onConnect]
 * @param {(reason:string) => void} [o.onClose]
 * @param {(message:string) => void} [o.onError]
 * @param {(event:object) => void} [o.onEvent]
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

    channel.onmessage = (message) => {
        let event;
        try {
            event = JSON.parse(message.data);
        } catch {
            return;
        }
        onEvent?.(event);
        const type = event?.type || "";
        if (type === EVENTS.started) return void onConnect?.();
        const role = transcriptRoleFor(type);
        if (role) {
            onTranscript?.(role, event.delta);
            if (role === "agent") {
                onSpeaking?.(true);
                clearTimeout(speakingTimer);
                speakingTimer = setTimeout(() => onSpeaking?.(false), 900);
            } else onSpeaking?.(false);
            return;
        }
        const call = functionCallFrom(event);
        if (call) {
            Promise.resolve()
                .then(() => (onToolCall ? onToolCall(call) : { error: "no tool handler" }))
                .catch((error) => ({ error: error?.message || "tool failed" }))
                .then((result) => {
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
            send({ type: EVENTS.close });
            await Promise.race([closedPromise, new Promise((r) => setTimeout(r, 2500))]);
            teardown("close_requested");
        },
    };
}
