// voiceComposer powers the web-inbox audio button: record from the microphone
// (OGG/Opus on Firefox, MP4/AAC on Chrome/Edge/Safari — the only formats
// WhatsApp accepts) or attach an audio file, preview, then POST multipart to
// /inbox/{conv}/reply-audio and swap in the re-rendered thread card.

// The text-send spinner lives in an Alpine store (not a component) so its state
// survives the thread-card outerHTML swap the reply triggers. It stays armed
// until the sent bubble reports delivered/read/failed on a fragment refresh.
document.addEventListener("alpine:init", () => {
	Alpine.store("send", {
		pending: false,
		pendingMsgID: "",
		timeout: null,

		markSending() {
			this.pending = true;
			this.pendingMsgID = "";
			clearTimeout(this.timeout);
			this.timeout = setTimeout(() => this.finish(), 90000); // never stuck forever
		},

		capture(msgID) {
			if (this.pending && !this.pendingMsgID && msgID) this.pendingMsgID = msgID;
		},

		seeStatus(status) {
			if (!this.pending || !status) return;
			if (status === "delivered" || status === "read" || status === "failed") this.finish();
		},

		finish() {
			clearTimeout(this.timeout);
			this.timeout = null;
			this.pending = false;
			this.pendingMsgID = "";
		},
	});
});

// The reply POST and the 15s bubble poll both swap DOM under these targets;
// listen once on document, they survive every swap.
document.addEventListener("htmx:beforeRequest", (e) => {
	const form = e.target.closest && e.target.closest("form[hx-post]");
	if (form && form.getAttribute("hx-post").endsWith("/reply")) {
		Alpine.store("send").markSending();
	}
});

document.addEventListener("htmx:afterSwap", (e) => {
	const t = e.detail && e.detail.target;
	if (!t || !t.id) return;
	if (t.id === "thread-card") {
		// The reply landed; the newest outbound bubble is our own message.
		const last = t.querySelector("#thread-bubbles .flex[data-direction='outbound']:last-of-type");
		if (!last) return;
		Alpine.store("send").capture(last.getAttribute("data-msg-id"));
		Alpine.store("send").seeStatus(last.getAttribute("data-status"));
	} else if (t.id === "thread-bubbles" && Alpine.store("send").pending && Alpine.store("send").pendingMsgID) {
		const want = Alpine.store("send").pendingMsgID;
		const node = t.querySelector(`[data-msg-id="${want}"]`);
		if (node) Alpine.store("send").seeStatus(node.getAttribute("data-status"));
	}
});
window.voiceComposer = (convID) => ({
	state: "idle", // idle | recording | ready
	mime: "",
	canRecord: false,
	recorder: null,
	stream: null,
	chunks: [],
	timer: null,
	timerStart: 0,
	durationMs: 0,
	blob: null,
	url: null,
	errors: "",
	sending: false,

	init() {
		const wanted = ["audio/ogg;codecs=opus", "audio/ogg", "audio/mp4", "audio/mpeg"];
		this.mime =
			(window.MediaRecorder &&
				wanted.find((m) => {
					try {
						return MediaRecorder.isTypeSupported(m);
					} catch {
						return false;
					}
				})) ||
			"";
		this.canRecord = this.mime !== "";
	},

	start() {
		if (!this.canRecord) {
			this.errors = "Recording is not supported in this browser — attach an audio file (MP3, M4A, OGG, AAC) instead.";
			return;
		}
		if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
			this.errors = "Microphone access is not available in this browser.";
			return;
		}
		clearInterval(this.timer);
		this.chunks = [];
		navigator.mediaDevices
			.getUserMedia({ audio: true })
			.then((stream) => {
				this.stream = stream;
				this.recorder = new MediaRecorder(stream, { mimeType: this.mime });
				this.recorder.ondataavailable = (e) => {
					if (e.data && e.data.size) this.chunks.push(e.data);
				};
				this.recorder.onstop = () => {
					this.stream.getTracks().forEach((t) => t.stop());
					this.blob = new Blob(this.chunks, { type: this.recorder.mimeType || this.mime });
					if (this.url) URL.revokeObjectURL(this.url);
					this.url = URL.createObjectURL(this.blob);
					this.state = "ready";
					clearInterval(this.timer);
				};
				this.recorder.start(250);
				this.timerStart = Date.now();
				this.timer = setInterval(() => {
					this.durationMs = Date.now() - this.timerStart;
				}, 333);
				this.state = "recording";
				this.errors = "";
			})
			.catch(() => {
				this.errors = "Microphone access was denied — allow it, or attach an audio file instead.";
			});
	},

	stop() {
		if (this.recorder && this.recorder.state === "recording") this.recorder.stop();
	},

	cancel() {
		if (this.recorder && this.recorder.state === "recording") {
			this.recorder.onstop = null;
			this.recorder.stop();
		}
		if (this.stream) this.stream.getTracks().forEach((t) => t.stop());
		this.reset();
	},

	reset() {
		clearInterval(this.timer);
		if (this.url) URL.revokeObjectURL(this.url);
		this.state = "idle";
		this.recorder = null;
		this.stream = null;
		this.chunks = [];
		this.blob = null;
		this.url = null;
		this.durationMs = 0;
		clearTimeout(this.durLoad);
	},

	loadFile(input) {
		const f = input && input.files && input.files[0];
		if (!f) return;
		const mime = this.canonical(f.type);
		if (!mime) {
			this.errors = "WhatsApp does not accept this format (WebM is not supported). Please attach an MP3, M4A, OGG, AAC or AMR file.";
			return;
		}
		this.mime = mime;
		this.blob = f;
		if (this.url) URL.revokeObjectURL(this.url);
		this.url = URL.createObjectURL(f);
		this.state = "ready";
		this.errors = "";
		const probe = new Audio();
		this.durLoad = setTimeout(() => {
			probe.src = this.url;
			probe.onloadedmetadata = () => {
				if (Number.isFinite(probe.duration) && probe.duration > 0) {
					this.durationMs = Math.round(probe.duration * 1000);
				}
			};
		}, 30);
	},

	canonical(mime) {
		const base = String(mime || "").split(";")[0].trim().toLowerCase();
		if (["audio/ogg", "audio/opus"].includes(base)) return "audio/ogg";
		if (["audio/mp4", "audio/m4a", "audio/x-m4a", "audio/m4b"].includes(base)) return "audio/mp4";
		if (["audio/mpeg", "audio/mp3", "audio/mpga"].includes(base)) return "audio/mpeg";
		if (base === "audio/aac") return "audio/aac";
		if (["audio/amr", "audio/amr-nb", "audio/x-amr"].includes(base)) return "audio/amr";
		return "";
	},

	ext() {
		const exts = { "audio/ogg": "ogg", "audio/mp4": "m4a", "audio/mpeg": "mp3", "audio/aac": "aac", "audio/amr": "amr" };
		return exts[this.mime] || "mp3";
	},

	fmt() {
		const s = Math.floor(this.durationMs / 1000);
		const m = Math.floor(s / 60);
		return String(m).padStart(2, "0") + ":" + String(s % 60).padStart(2, "0");
	},

	send() {
		if (!this.blob || this.sending) return;
		this.sending = true;
		this.errors = "";
		const fd = new FormData();
		fd.append("audio", this.blob, "voice." + this.ext());
		fd.append("duration_ms", String(this.durationMs));
		fetch("/inbox/" + convID + "/reply-audio", { method: "POST", body: fd })
			.then((res) => res.text())
			.then((html) => {
				const frag = document.createElement("template");
				frag.innerHTML = html;
				const node = frag.content.querySelector("#thread-card");
				const card = document.getElementById("thread-card");
				if (node && card) {
					card.replaceWith(node);
					const bubbles = node.querySelector("#thread-bubbles");
					if (bubbles) bubbles.scrollTop = bubbles.scrollHeight;
				}
				this.reset();
			})
			.catch(() => {
				this.errors = "Could not reach the server. Try again.";
			})
			.finally(() => {
				this.sending = false;
			});
	},
});